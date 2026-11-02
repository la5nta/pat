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
	case MethodAX25AGWPE, MethodArdop, MethodVaraHF, MethodVaraFM:
		_, release, err := a.modems.Acquire(url.Scheme)
		if err != nil {
			log.Println(err)
			return
		}
		defer release()
	case MethodPactor:
		ptCmdInit := ""
		if val, ok := url.Params["init"]; ok {
			ptCmdInit = strings.Join(val, "\n")
		}
		if err := a.initPACTOR(ptCmdInit); err != nil {
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

func (a *App) newModemHub() *ModemHub {
	return NewModemHub(map[string]ModemOpener{
		MethodArdop:     a.openARDOP,
		MethodVaraHF:    a.openVARAHF,
		MethodVaraFM:    a.openVARAFM,
		MethodAX25AGWPE: a.openAGWPE,
	})
}

func (a *App) openARDOP() (Modem, error) {
	tnc, err := ardop.OpenTCP(a.config.Ardop.Addr, a.options.MyCall, a.config.Locator)
	if err != nil {
		return nil, fmt.Errorf("ARDOP TNC initialization failed: %w", err)
	}
	if err := a.configureARDOP(tnc); err != nil {
		tnc.Close()
		return nil, err
	}
	transport.RegisterDialer(MethodArdop, tnc)
	return tnc, nil
}

func (a *App) configureARDOP(tnc *ardop.TNC) error {
	tnc.SetBusyFunc(a.onBusyChannel)

	if !a.config.Ardop.ARQBandwidth.IsZero() {
		if err := tnc.SetARQBandwidth(a.config.Ardop.ARQBandwidth); err != nil {
			return fmt.Errorf("unable to set ARQ bandwidth for ardop TNC: %w", err)
		}
	}

	if err := tnc.SetCWID(a.config.Ardop.CWID); err != nil {
		return fmt.Errorf("unable to configure CWID for ardop TNC: %w", err)
	}

	if v, err := tnc.Version(); err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %s", err)
	} else {
		log.Printf("ARDOP TNC (%s) initialized", v)
	}

	if !a.config.Ardop.PTTControl {
		return nil
	}

	rig, ok := a.rigs[a.config.Ardop.Rig]
	if !ok {
		return fmt.Errorf("unable to set PTT rig '%s': Not defined or not loaded", a.config.Ardop.Rig)
	}

	tnc.SetPTT(rig)
	return nil
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

type varaModem struct{ *vara.Modem }

func (m varaModem) Ping() error {
	if !m.Modem.Ping() {
		return errors.New("ping failed")
	}
	return nil
}

func (a *App) openVARAHF() (Modem, error) {
	m, err := a.openVARA(MethodVaraHF, a.config.VaraHF)
	if err != nil {
		return nil, err
	}
	if bw := a.config.VaraHF.Bandwidth; bw != 0 {
		if err := m.SetBandwidth(fmt.Sprint(bw)); err != nil {
			m.Close()
			return nil, err
		}
	}
	return varaModem{m}, nil
}

func (a *App) openVARAFM() (Modem, error) {
	m, err := a.openVARA(MethodVaraFM, a.config.VaraFM)
	if err != nil {
		return nil, err
	}
	return varaModem{m}, nil
}

func (a *App) openVARA(scheme string, conf cfg.VaraConfig) (*vara.Modem, error) {
	vConf := vara.ModemConfig{
		Host:     conf.Host(),
		CmdPort:  conf.CmdPort(),
		DataPort: conf.DataPort(),
	}
	m, err := vara.NewModem(scheme, a.options.MyCall, vConf)
	if err != nil {
		return nil, fmt.Errorf("vara initialization failed: %w", err)
	}
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

func (a *App) openAGWPE() (Modem, error) {
	port, err := agwpe.OpenPortTCP(a.config.AGWPE.Addr, a.config.AGWPE.RadioPort, a.options.MyCall)
	if err != nil {
		return nil, fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	}

	if v, err := port.Version(); err != nil {
		port.Close()
		return nil, fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	} else {
		log.Printf("AGWPE TNC (%s) initialized", v)
	}

	transport.RegisterContextDialer(MethodAX25AGWPE, port)
	return port, nil
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
