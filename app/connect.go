// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/prehook"

	"github.com/harenber/ptc-go/v2/pactor"
	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/ax25/agwpe"
	"github.com/n8jja/Pat-Vara/vara"

	// Register stateless dialers
	_ "github.com/la5nta/wl2k-go/transport/ax25"
	_ "github.com/la5nta/wl2k-go/transport/telnet"
)

func hasSSID(str string) bool { return strings.Contains(str, "-") }

func (a *App) Connect(connectStr string) (success bool) {
	if connectStr == "" {
		return false
	} else if aliased, ok := a.config.ConnectAliases[connectStr]; ok {
		return a.Connect(aliased)
	}

	// Replace placeholders
	connectStr = strings.ReplaceAll(connectStr, cfg.PlaceholderMycall, a.options.MyCall)

	// Prompt if Winlink account is unconfirmed
	if confirmed := a.promptUnconfirmedAccount(); !confirmed {
		return false
	}

	// Hack around bug in frontend which may occur if the status updates too quickly.
	if a.websocketHub.NumClients() > 0 {
		defer func() { time.Sleep(time.Second); a.websocketHub.UpdateStatus() }()
	}

	debug.Printf("connectStr: %s", connectStr)
	url, err := transport.ParseURL(connectStr)
	if err != nil {
		log.Println(err)
		return false
	}

	// TODO: Remove after some release cycles (2023-05-21)
	// Rewrite legacy serial-tnc scheme.
	if url.Scheme == MethodSerialTNCDeprecated {
		log.Printf("Transport scheme %s:// is deprecated, use %s:// instead.", MethodSerialTNCDeprecated, MethodAX25SerialTNC)
		url.Scheme = MethodAX25SerialTNC
	}

	// Rewrite the generic ax25:// scheme to use a specified AX.25 engine.
	if url.Scheme == MethodAX25 {
		url.Scheme = a.defaultAX25Method()
	}

	// Init TNCs
	switch url.Scheme {
	case MethodAX25AGWPE:
		if err := a.initAGWPEForConnect(); err != nil {
			log.Println(err)
			return
		}
	case MethodArdop:
		if err := a.initARDOPForConnect(); err != nil {
			log.Println(err)
			return
		}
	case MethodPactor:
		ptCmdInit := ""
		if val, ok := url.Params["init"]; ok {
			ptCmdInit = strings.Join(val, "\n")
		}
		if err := a.initPACTOR(ptCmdInit); err != nil {
			log.Println(err)
			return
		}
	case MethodVaraHF:
		if err := a.initVARAHFForConnect(); err != nil {
			log.Println(err)
			return
		}
	case MethodVaraFM:
		if err := a.initVARAFMForConnect(); err != nil {
			log.Println(err)
			return
		}
	}

	// Set default userinfo (mycall)
	if url.User == nil {
		url.SetUser(a.options.MyCall)
	}

	// Set default host interface address
	if url.Host == "" {
		switch url.Scheme {
		case MethodAX25Linux:
			url.Host = a.config.AX25Linux.Port
		case MethodAX25SerialTNC:
			url.Host = a.config.SerialTNC.Path
			if hbaud := a.config.SerialTNC.HBaud; hbaud > 0 {
				url.Params.Set("hbaud", fmt.Sprint(hbaud))
			}
			if sbaud := a.config.SerialTNC.SerialBaud; sbaud > 0 {
				url.Params.Set("serial_baud", fmt.Sprint(sbaud))
			}
		}
	}

	// Radio Only?
	radioOnly := a.options.RadioOnly
	if v := url.Params.Get("radio_only"); v != "" {
		radioOnly, _ = strconv.ParseBool(v)
	}
	if radioOnly {
		if hasSSID(a.options.MyCall) {
			log.Println("Radio Only does not support callsign with SSID")
			return
		}

		if strings.HasPrefix(url.Scheme, MethodAX25) {
			log.Printf("Radio-Only is not available for %s", url.Scheme)
			return
		}
		url.SetUser(url.User.Username() + "-T")
	}

	// QSY
	var revertFreq func()
	if freq := url.Params.Get("freq"); freq != "" {
		revertFreq, err = a.qsy(url.Scheme, freq)
		if err != nil {
			log.Printf("Unable to QSY: %s", err)
			return
		}
		defer revertFreq()
	}
	var currFreq Frequency
	if vfo, _, ok, _ := a.VFOForTransport(url.Scheme); ok {
		f, _ := vfo.GetFreq()
		currFreq = Frequency(f)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.dialCancelFunc = func() { a.dialing = nil; cancel() }
	defer a.dialCancelFunc()

	// Signal web gui that we are dialing a connection
	a.dialing = url
	a.websocketHub.UpdateStatus()

	prehookScript := prehook.Script{
		Dir:  a.options.PrehooksPath,
		File: url.Params.Get("prehook"),
		Args: url.Params["prehook-arg"],
	}
	if prehookScript.File != "" {
		if err := prehookScript.VerifyFile(); err != nil {
			log.Printf("invalid prehook: %s", err)
			return
		}
	}

	log.Printf("Connecting to %s (%s)...", url.Target, url.Scheme)
	conn, err := transport.DialURLContext(ctx, url)

	// Signal web gui that we are no longer dialing
	a.dialing = nil
	a.websocketHub.UpdateStatus()

	a.eventLog.LogConn("connect "+connectStr, currFreq, conn, err)

	switch {
	case errors.Is(err, context.Canceled):
		log.Printf("Connect cancelled")
		return
	case err != nil:
		log.Printf("Unable to establish connection to remote: %s", err)
		return
	}

	if prehookScript.File != "" {
		log.Println("Running prehook...")
		prehookScript.Env = append([]string{
			buildinfo.AppName + "_DIAL_URL=" + connectStr,
			buildinfo.AppName + "_REMOTE_ADDR=" + conn.RemoteAddr().String(),
			buildinfo.AppName + "_LOCAL_ADDR=" + conn.LocalAddr().String(),
		}, append(os.Environ(), a.Env()...)...)
		conn = prehook.Wrap(conn)
		if err := prehookScript.Execute(ctx, conn); err != nil {
			conn.Close()
			log.Printf("Prehook script failed: %s", err)
			return
		}
		log.Println("Prehook succeeded")
	}

	err = a.exchange(conn, url.Target, false)
	if err != nil {
		log.Printf("Exchange failed: %s", err)
	} else {
		log.Println("Disconnected.")
		success = true
	}

	return
}

func (a *App) qsy(method, addr string) (revert func(), err error) {
	noop := func() {}
	rig, rigName, ok, err := a.VFOForTransport(method)
	if err != nil {
		return noop, err
	} else if !ok {
		return noop, fmt.Errorf("hamlib rig '%s' not loaded", rigName)
	}

	log.Printf("QSY %s: %s", method, addr)
	_, oldFreq, err := SetFreq(rig, addr)
	if err != nil {
		return noop, err
	}

	time.Sleep(3 * time.Second)
	return func() {
		time.Sleep(time.Second)
		log.Printf("QSX %s: %.3f", method, float64(oldFreq)/1e3)
		rig.SetFreq(oldFreq)
	}, nil
}

func (a *App) onBusyChannel(ctx context.Context) (abort bool) {
	if a.options.IgnoreBusy {
		log.Println("Ignoring busy channel!")
		return false
	}

	log.Println("Waiting for clear channel...")
	select {
	case <-ctx.Done():
		// The channel is no longer busy.
		log.Println("Channel clear")
		return false
	case resp := <-a.promptHub.Prompt(ctx, 5*time.Minute, PromptKindBusyChannel, "Waiting for clear channel..."):
		return resp.Value == "abort" || resp.Err == context.DeadlineExceeded
	}
}

// ARDOP returns the initialized ARDOP modem, initializing it if necessary.
func (a *App) ARDOP() (*ardop.TNC, error) {
	if err := a.initARDOP(); err != nil {
		return nil, err
	}
	return a.ardop, nil
}

// dialARDOP is the actual network dial used by openARDOP. Overridden in
// tests to avoid needing to simulate ARDOP's real two-port control protocol.
var dialARDOP = ardop.OpenTCP

// launchPollInterval and launchPollBudget control how long openARDOP waits
// for a freshly launched TNC to become reachable before giving up. Var, not
// const, so tests can shrink them.
var (
	launchPollInterval = 500 * time.Millisecond
	launchPollBudget   = 5 * time.Second
)

func (a *App) initARDOP() error {
	if a.ardop != nil && a.ardop.Ping() == nil {
		return nil
	}
	if a.ardop != nil {
		a.ardop.Close()
		a.ardop = nil
	}

	tnc, err := a.openARDOP()
	if err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %w", err)
	}
	return a.configureARDOP(tnc)
}

// initARDOPForConnect is like initARDOP, but for the outgoing Connect path:
// if a launch_cmd is configured, it waits (see waitForARDOP) for the daemon
// to come up before giving up, since Connect has no other retry loop.
func (a *App) initARDOPForConnect() error {
	if a.ardop != nil && a.ardop.Ping() == nil {
		return nil
	}
	if a.ardop != nil {
		a.ardop.Close()
		a.ardop = nil
	}

	tnc, err := a.waitForARDOP()
	if err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %w", err)
	}
	return a.configureARDOP(tnc)
}

// configureARDOP finishes initializing a freshly dialed Ardop TNC and
// installs it as a.ardop.
func (a *App) configureARDOP(tnc *ardop.TNC) error {
	a.ardop = tnc
	a.ardop.SetBusyFunc(a.onBusyChannel)

	if !a.config.Ardop.ARQBandwidth.IsZero() {
		if err := a.ardop.SetARQBandwidth(a.config.Ardop.ARQBandwidth); err != nil {
			return fmt.Errorf("unable to set ARQ bandwidth for ardop TNC: %w", err)
		}
	}

	if err := a.ardop.SetCWID(a.config.Ardop.CWID); err != nil {
		return fmt.Errorf("unable to configure CWID for ardop TNC: %w", err)
	}

	if v, err := a.ardop.Version(); err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %s", err)
	} else {
		log.Printf("ARDOP TNC (%s) initialized", v)
	}

	transport.RegisterDialer(MethodArdop, a.ardop)

	if !a.config.Ardop.PTTControl {
		return nil
	}

	rig, ok := a.rigs[a.config.Ardop.Rig]
	if !ok {
		return fmt.Errorf("unable to set PTT rig '%s': Not defined or not loaded", a.config.Ardop.Rig)
	}

	a.ardop.SetPTT(rig)
	return nil
}

// openARDOP makes a single dial attempt against the configured Ardop TNC
// address, launching the configured Ardop.LaunchCmd if it isn't reachable
// and no launch is already in flight for it. It does not itself wait for a
// freshly launched daemon to come up: this is the primitive shared by both
// initARDOP() callers (outgoing Connect and, via ARDOPListener.Init(), the
// incoming Listen path), and only Connect lacks a retry loop of its own —
// see initARDOPForConnect. Waiting here too would turn ListenerHub's 1s
// retry cadence into launchPollBudget-per-tick.
func (a *App) openARDOP() (*ardop.TNC, error) {
	tnc, err := dialARDOP(a.config.Ardop.Addr, a.options.MyCall, a.config.Locator)
	if err == nil {
		return tnc, nil
	}
	if a.config.Ardop.LaunchCmd.Path != "" {
		if startErr := a.startLaunch(MethodArdop, a.config.Ardop.LaunchCmd); startErr != nil {
			debug.Printf("ardop: launch_cmd failed to start: %s", startErr)
			log.Printf("Pat failed to launch ardop")
		}
	}
	return nil, err
}

// waitForARDOP dials the configured Ardop TNC, launching Ardop.LaunchCmd if
// it isn't reachable, and — unlike openARDOP — retries with a bounded poll
// budget while the daemon starts up. Only initARDOPForConnect uses this:
// unlike Listen, outgoing Connect has no other retry loop of its own to
// fall back on for the wait.
//
// It does NOT give up early just because the process it spawned has
// exited: some launch_cmd scripts daemonize (a short-lived parent forks the
// real daemon and exits), so "our child exited" is not reliable evidence
// the daemon itself isn't still coming up. The one case that IS reliable
// enough to skip polling is cmd.Start() itself failing — that means nothing
// was ever created, so retrying the dial cannot help.
func (a *App) waitForARDOP() (*ardop.TNC, error) {
	tnc, err := dialARDOP(a.config.Ardop.Addr, a.options.MyCall, a.config.Locator)
	if err == nil || a.config.Ardop.LaunchCmd.Path == "" {
		return tnc, err
	}

	if startErr := a.startLaunch(MethodArdop, a.config.Ardop.LaunchCmd); startErr != nil {
		debug.Printf("ardop: launch_cmd failed to start: %s", startErr)
		log.Printf("Pat failed to launch ardop")
		return nil, err
	}

	deadline := time.Now().Add(launchPollBudget)
	for time.Now().Before(deadline) {
		time.Sleep(launchPollInterval)
		if tnc, err = dialARDOP(a.config.Ardop.Addr, a.options.MyCall, a.config.Locator); err == nil {
			return tnc, nil
		}
	}

	log.Printf("Pat failed to launch ardop")
	return nil, err
}

// startLaunch atomically checks whether a process is already tracked as
// launched for name and, if not, spawns cmdCfg and registers it — closing
// the check-then-spawn race that would otherwise let two concurrent callers
// (e.g. a Listen retry tick and a manual Connect) both spawn a duplicate.
// The registered entry is removed once the process exits, allowing a later
// call to relaunch it (e.g. after the daemon crashes).
func (a *App) startLaunch(name string, cmdCfg cfg.LaunchCmd) error {
	a.launchedMu.Lock()
	if _, inFlight := a.launched[name]; inFlight {
		a.launchedMu.Unlock()
		return nil
	}

	cmd := exec.Command(cmdCfg.Path, cmdCfg.Args...)
	if err := cmd.Start(); err != nil {
		a.launchedMu.Unlock()
		return err
	}
	if a.launched == nil {
		a.launched = make(map[string]*exec.Cmd)
	}
	a.launched[name] = cmd
	a.launchedMu.Unlock()

	go func() {
		cmd.Wait()
		a.launchedMu.Lock()
		if a.launched[name] == cmd {
			delete(a.launched, name)
		}
		a.launchedMu.Unlock()
	}()
	return nil
}

// isLaunchInFlight reports whether a process is currently tracked as
// spawned, and not yet seen to exit, for the given transport.
func (a *App) isLaunchInFlight(name string) bool {
	a.launchedMu.Lock()
	defer a.launchedMu.Unlock()
	_, ok := a.launched[name]
	return ok
}

// terminateLaunchedProcesses kills any process Pat has spawned via a
// transport's LaunchCmd and not yet seen exit.
func (a *App) terminateLaunchedProcesses() {
	a.launchedMu.Lock()
	defer a.launchedMu.Unlock()
	for name, cmd := range a.launched {
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("Failed to terminate launched %s process: %s", name, err)
		}
	}
}

func (a *App) initPACTOR(cmdlineinit string) error {
	if a.pactor != nil {
		a.pactor.Close()
	}
	var err error
	a.pactor, err = pactor.OpenModem(a.config.Pactor.Path, a.config.Pactor.Baudrate, a.options.MyCall, a.config.Pactor.InitScript, cmdlineinit)
	if err != nil || a.pactor == nil {
		return fmt.Errorf("pactor initialization failed: %w", err)
	}

	transport.RegisterDialer(MethodPactor, a.pactor)

	return nil
}

// VARAHF returns the initialized VARA HF modem, initializing it if necessary.
func (a *App) VARAHF() (*vara.Modem, error) {
	if err := a.initVARAHF(); err != nil {
		return nil, err
	}
	return a.varaHF, nil
}

func (a *App) initVARAHF() error {
	if a.varaHF != nil && a.varaHF.Ping() {
		return nil
	}
	if a.varaHF != nil {
		a.varaHF.Close()
		a.varaHF = nil
	}
	m, err := a.initVARA(MethodVaraHF, a.config.VaraHF)
	if err != nil {
		return err
	}
	return a.finishVARAHF(m)
}

// initVARAHFForConnect is like initVARAHF, but for the outgoing Connect
// path: if a launch_cmd is configured, it waits (see waitForVaraModem) for
// the modem to come up before giving up, since Connect has no other retry
// loop of its own, unlike Listen.
func (a *App) initVARAHFForConnect() error {
	if a.varaHF != nil && a.varaHF.Ping() {
		return nil
	}
	if a.varaHF != nil {
		a.varaHF.Close()
		a.varaHF = nil
	}
	dialed, err := a.waitForVaraModem(MethodVaraHF, a.config.VaraHF)
	if err != nil {
		return fmt.Errorf("vara initialization failed: %w", err)
	}
	m, err := a.configureVaraModem(dialed, MethodVaraHF, a.config.VaraHF)
	if err != nil {
		return err
	}
	return a.finishVARAHF(m)
}

// finishVARAHF applies VaraHF-specific setup on top of configureVaraModem
// (namely the default bandwidth) and installs m as a.varaHF.
func (a *App) finishVARAHF(m *vara.Modem) error {
	if bw := a.config.VaraHF.Bandwidth; bw != 0 {
		if err := m.SetBandwidth(fmt.Sprint(bw)); err != nil {
			m.Close()
			return err
		}
	}
	a.varaHF = m
	return nil
}

// VARAFM returns the initialized VARA FM modem, initializing it if necessary.
func (a *App) VARAFM() (*vara.Modem, error) {
	if err := a.initVARAFM(); err != nil {
		return nil, err
	}
	return a.varaFM, nil
}

func (a *App) initVARAFM() error {
	if a.varaFM != nil && a.varaFM.Ping() {
		return nil
	}
	if a.varaFM != nil {
		a.varaFM.Close()
		a.varaFM = nil
	}
	m, err := a.initVARA(MethodVaraFM, a.config.VaraFM)
	if err != nil {
		return err
	}
	a.varaFM = m
	return nil
}

// initVARAFMForConnect is like initVARAFM, but for the outgoing Connect
// path: if a launch_cmd is configured, it waits (see waitForVaraModem) for
// the modem to come up before giving up, since Connect has no other retry
// loop of its own, unlike Listen.
func (a *App) initVARAFMForConnect() error {
	if a.varaFM != nil && a.varaFM.Ping() {
		return nil
	}
	if a.varaFM != nil {
		a.varaFM.Close()
		a.varaFM = nil
	}
	dialed, err := a.waitForVaraModem(MethodVaraFM, a.config.VaraFM)
	if err != nil {
		return fmt.Errorf("vara initialization failed: %w", err)
	}
	m, err := a.configureVaraModem(dialed, MethodVaraFM, a.config.VaraFM)
	if err != nil {
		return err
	}
	a.varaFM = m
	return nil
}

// dialVara is the actual dial+handshake used by openVaraModem and
// waitForVaraModem. Overridden in tests to avoid needing to simulate VARA's
// real protocol.
var dialVara = vara.NewModem

func (a *App) initVARA(scheme string, conf cfg.VaraConfig) (*vara.Modem, error) {
	m, err := a.openVaraModem(scheme, conf)
	if err != nil {
		return nil, fmt.Errorf("vara initialization failed: %w", err)
	}
	return a.configureVaraModem(m, scheme, conf)
}

// openVaraModem makes a single dial+handshake attempt for the given VARA
// scheme (MethodVaraHF or MethodVaraFM), launching conf.LaunchCmd if it
// isn't reachable and no launch is already in flight for it. It does not
// itself wait for a freshly launched daemon to come up: this is the
// primitive shared by both initVARA callers (outgoing Connect and, via
// VaraHFListener/VaraFMListener.Init(), the incoming Listen path) — see
// waitForVaraModem for the Connect-only wait.
func (a *App) openVaraModem(scheme string, conf cfg.VaraConfig) (*vara.Modem, error) {
	vConf := vara.ModemConfig{Host: conf.Host(), CmdPort: conf.CmdPort(), DataPort: conf.DataPort()}
	m, err := dialVara(scheme, a.options.MyCall, vConf)
	if err == nil {
		return m, nil
	}
	if conf.LaunchCmd.Path != "" {
		if startErr := a.startLaunch(scheme, conf.LaunchCmd); startErr != nil {
			debug.Printf("%s: launch_cmd failed to start: %s", scheme, startErr)
			log.Printf("Pat failed to launch %s", scheme)
		}
	}
	return nil, err
}

// waitForVaraModem dials the given VARA scheme, launching conf.LaunchCmd if
// it isn't reachable, and — unlike openVaraModem — retries with a bounded
// poll budget while the daemon starts up. Only *ForConnect callers use
// this: unlike Listen, outgoing Connect has no other retry loop of its own
// to fall back on for the wait.
//
// Like waitForARDOP, it does NOT give up early just because the process it
// spawned has exited: some launch_cmd scripts daemonize (a short-lived
// parent forks the real daemon and exits), so "our child exited" is not
// reliable evidence the daemon isn't still coming up. The one case that IS
// reliable enough to skip polling is cmd.Start() itself failing.
func (a *App) waitForVaraModem(scheme string, conf cfg.VaraConfig) (*vara.Modem, error) {
	vConf := vara.ModemConfig{Host: conf.Host(), CmdPort: conf.CmdPort(), DataPort: conf.DataPort()}
	m, err := dialVara(scheme, a.options.MyCall, vConf)
	if err == nil || conf.LaunchCmd.Path == "" {
		return m, err
	}

	if startErr := a.startLaunch(scheme, conf.LaunchCmd); startErr != nil {
		debug.Printf("%s: launch_cmd failed to start: %s", scheme, startErr)
		log.Printf("Pat failed to launch %s", scheme)
		return nil, err
	}

	deadline := time.Now().Add(launchPollBudget)
	for time.Now().Before(deadline) {
		time.Sleep(launchPollInterval)
		if m, err = dialVara(scheme, a.options.MyCall, vConf); err == nil {
			return m, nil
		}
	}

	log.Printf("Pat failed to launch %s", scheme)
	return nil, err
}

// configureVaraModem finishes initializing a freshly dialed VARA modem.
func (a *App) configureVaraModem(m *vara.Modem, scheme string, conf cfg.VaraConfig) (*vara.Modem, error) {
	transport.RegisterDialer(scheme, m)
	m.SetBusyFunc(a.onBusyChannel)

	if conf.PTTControl {
		rig, ok := a.rigs[conf.Rig]
		if !ok {
			m.Close()
			return nil, fmt.Errorf("unable to set PTT rig '%s': not defined or not loaded", conf.Rig)
		}
		m.SetPTT(rig)
	}
	v, _ := m.Version()
	log.Printf("VARA modem (%s) initialized", v)
	return m, nil
}

// AGWPE returns the initialized AGWPE TNC, initializing it if necessary.
func (a *App) AGWPE() (*agwpe.TNCPort, error) {
	if err := a.initAGWPE(); err != nil {
		return nil, err
	}
	return a.agwpe, nil
}

func (a *App) initAGWPE() error {
	if a.agwpe != nil && a.agwpe.Ping() == nil {
		return nil
	}
	if a.agwpe != nil {
		a.agwpe.Close()
		a.agwpe = nil
	}

	tp, err := a.openAGWPE()
	if err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	}
	return a.configureAGWPE(tp)
}

// initAGWPEForConnect is like initAGWPE, but for the outgoing Connect path:
// if a launch_cmd is configured, it waits (see waitForAGWPE) for the daemon
// to come up before giving up, since Connect has no other retry loop.
func (a *App) initAGWPEForConnect() error {
	if a.agwpe != nil && a.agwpe.Ping() == nil {
		return nil
	}
	if a.agwpe != nil {
		a.agwpe.Close()
		a.agwpe = nil
	}

	tp, err := a.waitForAGWPE()
	if err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	}
	return a.configureAGWPE(tp)
}

// configureAGWPE finishes initializing a freshly dialed AGWPE TNC and
// installs it as a.agwpe.
func (a *App) configureAGWPE(tp *agwpe.TNCPort) error {
	a.agwpe = tp

	if v, err := a.agwpe.Version(); err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	} else {
		log.Printf("AGWPE TNC (%s) initialized", v)
	}

	transport.RegisterContextDialer(MethodAX25AGWPE, a.agwpe)
	return nil
}

// dialAGWPE is the actual network dial+registration used by openAGWPE and
// waitForAGWPE. Overridden in tests to avoid needing to simulate AGWPE's
// real port-registration protocol.
var dialAGWPE = agwpe.OpenPortTCP

// agwpeLaunchName is the launch-tracking key (startLaunch/isLaunchInFlight)
// and log-message name for AGWPE's launch_cmd. Not MethodAX25AGWPE, which is
// "ax25+agwpe" — the terse failure warning must read "Pat failed to launch
// agwpe" per spec.
const agwpeLaunchName = "agwpe"

// openAGWPE makes a single dial attempt against the configured AGWPE TNC
// address, launching the configured AGWPE.LaunchCmd if it isn't reachable
// and no launch is already in flight for it. It does not itself wait for a
// freshly launched daemon to come up: this is the primitive shared by both
// initAGWPE() callers (outgoing Connect and, via AX25AGWPEListener.Init(),
// the incoming Listen path), and only Connect lacks a retry loop of its own
// — see initAGWPEForConnect. Waiting here too would turn ListenerHub's 1s
// retry cadence into launchPollBudget-per-tick.
func (a *App) openAGWPE() (*agwpe.TNCPort, error) {
	tp, err := dialAGWPE(a.config.AGWPE.Addr, a.config.AGWPE.RadioPort, a.options.MyCall)
	if err == nil {
		return tp, nil
	}
	if a.config.AGWPE.LaunchCmd.Path != "" {
		if startErr := a.startLaunch(agwpeLaunchName, a.config.AGWPE.LaunchCmd); startErr != nil {
			debug.Printf("agwpe: launch_cmd failed to start: %s", startErr)
			log.Printf("Pat failed to launch agwpe")
		}
	}
	return nil, err
}

// waitForAGWPE dials the configured AGWPE TNC, launching AGWPE.LaunchCmd if
// it isn't reachable, and — unlike openAGWPE — retries with a bounded poll
// budget while the daemon starts up. Only initAGWPEForConnect uses this:
// unlike Listen, outgoing Connect has no other retry loop of its own to
// fall back on for the wait.
//
// It does NOT give up early just because the process it spawned has
// exited: some launch_cmd scripts daemonize (a short-lived parent forks the
// real daemon and exits), so "our child exited" is not reliable evidence
// the daemon itself isn't still coming up. The one case that IS reliable
// enough to skip polling is cmd.Start() itself failing — that means nothing
// was ever created, so retrying the dial cannot help.
func (a *App) waitForAGWPE() (*agwpe.TNCPort, error) {
	tp, err := dialAGWPE(a.config.AGWPE.Addr, a.config.AGWPE.RadioPort, a.options.MyCall)
	if err == nil || a.config.AGWPE.LaunchCmd.Path == "" {
		return tp, err
	}

	if startErr := a.startLaunch(agwpeLaunchName, a.config.AGWPE.LaunchCmd); startErr != nil {
		debug.Printf("agwpe: launch_cmd failed to start: %s", startErr)
		log.Printf("Pat failed to launch agwpe")
		return nil, err
	}

	deadline := time.Now().Add(launchPollBudget)
	for time.Now().Before(deadline) {
		time.Sleep(launchPollInterval)
		if tp, err = dialAGWPE(a.config.AGWPE.Addr, a.config.AGWPE.RadioPort, a.options.MyCall); err == nil {
			return tp, nil
		}
	}

	log.Printf("Pat failed to launch agwpe")
	return nil, err
}

// defaultAX25Method resolves the generic ax25:// scheme to a implementation specific scheme.
func (a *App) defaultAX25Method() string {
	switch a.config.AX25.Engine {
	case cfg.AX25EngineAGWPE:
		return MethodAX25AGWPE
	case cfg.AX25EngineSerialTNC:
		return MethodAX25SerialTNC
	case cfg.AX25EngineLinux:
		return MethodAX25Linux
	default:
		panic(fmt.Sprintf("invalid ax25 engine: %s", a.config.AX25.Engine))
	}
}
