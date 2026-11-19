// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/prehook"

	"github.com/harenber/ptc-go/v2/pactor"
	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/ax25/agwpe"
	"github.com/la5nta/wl2k-go/transport/telnet"
	"github.com/n8jja/Pat-Vara/vara"
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
	return a.connectURL(connectStr, url)
}

func (a *App) connectURL(connectStr string, url *transport.URL) (success bool) {
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

	ctx, cancel := context.WithCancel(context.Background())
	a.dialCancelFunc = func() { a.dialing = nil; cancel() }
	defer a.dialCancelFunc()

	// Signal web gui that we are dialing, so an abort can cancel a stuck Acquire too.
	a.dialing = url
	a.websocketHub.UpdateStatus()

	modem, release, err := a.modems.Acquire(ctx, url.Scheme)
	switch {
	case errors.Is(err, context.Canceled):
		log.Printf("Connect cancelled")
		return
	case err != nil:
		log.Println(err)
		return
	}
	defer release()

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
	conn, err := modem.DialURLContext(ctx, url)

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
		MethodArdop:         a.openARDOP,
		MethodVaraHF:        a.openVARAHF,
		MethodVaraFM:        a.openVARAFM,
		MethodAX25AGWPE:     a.openAGWPE,
		MethodPactor:        a.openPactor,
		MethodTelnet:        statelessOpener(telnet.DefaultDialer, a.listenTelnet),
		MethodAX25Linux:     statelessOpener(ax25.DefaultDialer, a.listenAX25Linux),
		MethodAX25SerialTNC: statelessOpener(ax25.DefaultDialer, nil),
	})
}

// statelessModem adapts a dialer with no connection state to the Modem interface.
type statelessModem struct {
	transport.ContextDialer
	listen func() (net.Listener, error)
}

func (m statelessModem) Ping() error  { return nil }
func (m statelessModem) Close() error { return nil }
func (m statelessModem) Listen() (net.Listener, error) {
	if m.listen == nil {
		return nil, errors.New("listen not implemented")
	}
	return m.listen()
}

func statelessOpener(d transport.ContextDialer, listen func() (net.Listener, error)) ModemOpener {
	return func() (Modem, error) { return statelessModem{d, listen}, nil }
}

func (a *App) listenTelnet() (net.Listener, error) {
	return telnet.Listen(a.config.Telnet.ListenAddr)
}

func (a *App) listenAX25Linux() (net.Listener, error) {
	return ax25.ListenAX25(a.config.AX25Linux.Port, a.options.MyCall)
}

func (a *App) openARDOP() (Modem, error) {
	tnc, err := ardop.OpenTCP(a.config.Ardop.Addr, a.options.MyCall, a.config.Locator.String())
	if err != nil {
		return nil, fmt.Errorf("ARDOP TNC initialization failed: %w", err)
	}
	fail := func(err error) (Modem, error) {
		tnc.Close()
		return nil, err
	}

	tnc.SetBusyFunc(a.onBusyChannel)

	if !a.config.Ardop.ARQBandwidth.IsZero() {
		if err := tnc.SetARQBandwidth(a.config.Ardop.ARQBandwidth); err != nil {
			return fail(fmt.Errorf("unable to set ARQ bandwidth for ardop TNC: %w", err))
		}
	}

	if err := tnc.SetCWID(a.config.Ardop.CWID); err != nil {
		return fail(fmt.Errorf("unable to configure CWID for ardop TNC: %w", err))
	}

	v, err := tnc.Version()
	if err != nil {
		return fail(fmt.Errorf("ARDOP TNC initialization failed: %w", err))
	}
	log.Printf("ARDOP TNC (%s) initialized", v)

	if a.config.Ardop.PTTControl {
		rig, ok := a.rigs[a.config.Ardop.Rig]
		if !ok {
			return fail(fmt.Errorf("unable to set PTT rig '%s': Not defined or not loaded", a.config.Ardop.Rig))
		}
		tnc.SetPTT(rig)
	}
	return tnc, nil
}

// pactorModem reopens the PTC on every dial, applying that connect's init commands.
type pactorModem struct {
	a  *App
	mu sync.Mutex
	m  *pactor.Modem
}

func (a *App) openPactor() (Modem, error) { return &pactorModem{a: a}, nil }

func (p *pactorModem) current() *pactor.Modem {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.m
}

func (p *pactorModem) DialURLContext(ctx context.Context, url *transport.URL) (net.Conn, error) {
	p.Close()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conf := p.a.config.Pactor
	m, err := pactor.OpenModem(conf.Path, conf.Baudrate, p.a.options.MyCall, conf.InitScript, strings.Join(url.Params["init"], "\n"))
	if err != nil || m == nil {
		return nil, fmt.Errorf("pactor initialization failed: %w", err)
	}
	p.mu.Lock()
	p.m = m
	p.mu.Unlock()
	// The PTC call ignores ctx; closing the modem is the only way to abort it.
	stop := context.AfterFunc(ctx, func() { m.Close() })
	defer stop()
	conn, err := m.DialURL(url)
	if err == nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return conn, err
}

func (p *pactorModem) Ping() error { return nil }
func (p *pactorModem) Listen() (net.Listener, error) {
	return nil, errors.New("pactor listen not implemented")
}

func (p *pactorModem) Close() error {
	if m := p.current(); m != nil {
		return m.Close()
	}
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
