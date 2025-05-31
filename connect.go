// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

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

var (
	dialing *transport.URL // The connect URL currently being dialed (if any)

	adTNC       *ardop.TNC     // Pointer to the ARDOP TNC used by Listen and Connect
	agwpeTNC    *agwpe.TNCPort // Pointer to the AGWPE TNC combined TNC and Port
	pModem      *pactor.Modem
	varaHFModem *vara.Modem
	varaFMModem *vara.Modem

	// Context cancellation function for aborting while dialing.
	dialCancelFunc func() = func() {}
)

func hasSSID(str string) bool { return strings.Contains(str, "-") }

func connectAny(connectStr ...string) bool {
	for _, str := range connectStr {
		if Connect(str) {
			return true
		}
	}
	return false
}

func Connect(connectStr string) (success bool) {
	if connectStr == "" {
		return false
	} else if aliased, ok := config.ConnectAliases[connectStr]; ok {
		return Connect(aliased)
	}

	// Hack around bug in frontend which may occur if the status updates too quickly.
	if websocketHub != nil {
		defer func() { time.Sleep(time.Second); websocketHub.UpdateStatus() }()
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
		url.Scheme = defaultAX25Method()
	}

	// Init TNCs
	switch url.Scheme {
	case MethodAX25AGWPE:
		if err := initAGWPE(); err != nil {
			log.Println(err)
			return
		}
	case MethodArdop:
		if err := initArdopTNC(); err != nil {
			log.Println(err)
			return
		}
	case MethodPactor:
		ptCmdInit := ""
		if val, ok := url.Params["init"]; ok {
			ptCmdInit = strings.Join(val, "\n")
		}
		if err := initPactorModem(ptCmdInit); err != nil {
			log.Println(err)
			return
		}
	case MethodVaraHF:
		if err := initVaraHFModem(); err != nil {
			log.Println(err)
			return
		}
	case MethodVaraFM:
		if err := initVaraFMModem(); err != nil {
			log.Println(err)
			return
		}
	}

	// Set default userinfo (mycall)
	if url.User == nil {
		url.SetUser(fOptions.MyCall)
	}

	// Set default host interface address
	if url.Host == "" {
		switch url.Scheme {
		case MethodAX25Linux:
			url.Host = config.AX25Linux.Port
		case MethodAX25SerialTNC:
			url.Host = config.SerialTNC.Path
			if hbaud := config.SerialTNC.HBaud; hbaud > 0 {
				url.Params.Set("hbaud", fmt.Sprint(hbaud))
			}
			if sbaud := config.SerialTNC.SerialBaud; sbaud > 0 {
				url.Params.Set("serial_baud", fmt.Sprint(sbaud))
			}
		}
	}

	// Radio Only?
	radioOnly := fOptions.RadioOnly
	if v := url.Params.Get("radio_only"); v != "" {
		radioOnly, _ = strconv.ParseBool(v)
	}
	if radioOnly {
		if hasSSID(fOptions.MyCall) {
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
		revertFreq, err = qsy(url.Scheme, freq)
		if err != nil {
			log.Printf("Unable to QSY: %s", err)
			return
		}
		defer revertFreq()
	}
	var currFreq Frequency
	if vfo, _, ok, _ := VFOForTransport(url.Scheme); ok {
		f, _ := vfo.GetFreq()
		currFreq = Frequency(f)
	}

	ctx, cancel := context.WithCancel(context.Background())
	dialCancelFunc = func() { dialing = nil; cancel() }
	defer dialCancelFunc()

	// Signal web gui that we are dialing a connection
	dialing = url
	websocketHub.UpdateStatus()

	if exec := url.Params.Get("prehook"); exec != "" {
		if err := prehook.Verify(exec); err != nil {
			log.Printf("prehook invalid: %s", err)
			return
		}
	}

	log.Printf("Connecting to %s (%s)...", url.Target, url.Scheme)
	conn, err := transport.DialURLContext(ctx, url)

	// Signal web gui that we are no longer dialing
	dialing = nil
	websocketHub.UpdateStatus()

	eventLog.LogConn("connect "+connectStr, currFreq, conn, err)

	switch {
	case errors.Is(err, context.Canceled):
		log.Printf("Connect cancelled")
		return
	case err != nil:
		log.Printf("Unable to establish connection to remote: %s", err)
		return
	}

	if exec := url.Params.Get("prehook"); exec != "" {
		log.Println("Running prehook...")
		script := prehook.Script{
			File: exec,
			Args: url.Params["prehook-arg"],
			Env: append([]string{
				buildinfo.AppName + "_DIAL_URL=" + connectStr,
				buildinfo.AppName + "_REMOTE_ADDR=" + conn.RemoteAddr().String(),
				buildinfo.AppName + "_LOCAL_ADDR=" + conn.LocalAddr().String(),
			}, append(os.Environ(), envAll()...)...),
		}
		conn = prehook.Wrap(conn)
		if err := script.Execute(ctx, conn); err != nil {
			conn.Close()
			log.Printf("Prehook script failed: %s", err)
			return
		}
		log.Println("Prehook succeeded")
	}

	err = exchange(conn, url.Target, false)
	if err != nil {
		log.Printf("Exchange failed: %s", err)
	} else {
		log.Println("Disconnected.")
		success = true
	}

	return
}

func qsy(method, addr string) (revert func(), err error) {
	noop := func() {}
	rig, rigName, ok, err := VFOForTransport(method)
	if err != nil {
		return noop, err
	} else if !ok {
		return noop, fmt.Errorf("hamlib rig '%s' not loaded", rigName)
	}

	log.Printf("QSY %s: %s", method, addr)
	_, oldFreq, err := setFreq(rig, addr)
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

func onBusyChannel(ctx context.Context) (abort bool) {
	if fOptions.IgnoreBusy {
		log.Println("Ignoring busy channel!")
		return false
	}

	// TODO: Extend this to prompt the user (continue anyway)
	log.Println("Waiting for clear channel...")
	<-ctx.Done()
	return false
}

func initArdopTNC() error {
	if adTNC != nil && adTNC.Ping() == nil {
		return nil
	}

	if adTNC != nil {
		adTNC.Close()
	}

	var err error
	adTNC, err = ardop.OpenTCP(config.Ardop.Addr, fOptions.MyCall, config.Locator)
	if err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %w", err)
	}

	adTNC.SetBusyFunc(onBusyChannel)

	if !config.Ardop.ARQBandwidth.IsZero() {
		if err := adTNC.SetARQBandwidth(config.Ardop.ARQBandwidth); err != nil {
			return fmt.Errorf("unable to set ARQ bandwidth for ardop TNC: %w", err)
		}
	}

	if err := adTNC.SetCWID(config.Ardop.CWID); err != nil {
		return fmt.Errorf("unable to configure CWID for ardop TNC: %w", err)
	}

	if v, err := adTNC.Version(); err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %s", err)
	} else {
		log.Printf("ARDOP TNC (%s) initialized", v)
	}

	transport.RegisterDialer(MethodArdop, adTNC)

	if !config.Ardop.PTTControl {
		return nil
	}

	rig, ok := rigs[config.Ardop.Rig]
	if !ok {
		return fmt.Errorf("unable to set PTT rig '%s': Not defined or not loaded", config.Ardop.Rig)
	}

	adTNC.SetPTT(rig)
	return nil
}

func initPactorModem(cmdlineinit string) error {
	if pModem != nil {
		pModem.Close()
	}
	var err error
	pModem, err = pactor.OpenModem(config.Pactor.Path, config.Pactor.Baudrate, fOptions.MyCall, config.Pactor.InitScript, cmdlineinit)
	if err != nil || pModem == nil {
		return fmt.Errorf("pactor initialization failed: %w", err)
	}

	transport.RegisterDialer(MethodPactor, pModem)

	return nil
}

func initVaraHFModem() error {
	if varaHFModem != nil && varaHFModem.Ping() {
		return nil
	}
	if varaHFModem != nil {
		varaHFModem.Close()
	}
	m, err := initVaraModem(MethodVaraHF, config.VaraHF)
	if err != nil {
		return err
	}
	if bw := config.VaraHF.Bandwidth; bw != 0 {
		if err := m.SetBandwidth(fmt.Sprint(bw)); err != nil {
			m.Close()
			return err
		}
	}
	varaHFModem = m
	return nil
}

func initVaraFMModem() error {
	if varaFMModem != nil && varaFMModem.Ping() {
		return nil
	}
	if varaFMModem != nil {
		varaFMModem.Close()
	}
	m, err := initVaraModem(MethodVaraFM, config.VaraFM)
	if err != nil {
		return err
	}
	varaFMModem = m
	return nil
}

func initVaraModem(scheme string, conf cfg.VaraConfig) (*vara.Modem, error) {
	vConf := vara.ModemConfig{
		Host:     conf.Host(),
		CmdPort:  conf.CmdPort(),
		DataPort: conf.DataPort(),
	}
	m, err := vara.NewModem(scheme, fOptions.MyCall, vConf)
	if err != nil {
		return nil, fmt.Errorf("vara initialization failed: %w", err)
	}
	transport.RegisterDialer(scheme, m)

	if conf.PTTControl {
		rig, ok := rigs[conf.Rig]
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

func initAGWPE() error {
	if agwpeTNC != nil && agwpeTNC.Ping() == nil {
		return nil
	}

	if agwpeTNC != nil {
		agwpeTNC.Close()
	}

	var err error
	agwpeTNC, err = agwpe.OpenPortTCP(config.AGWPE.Addr, config.AGWPE.RadioPort, fOptions.MyCall)
	if err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	}

	if v, err := agwpeTNC.Version(); err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	} else {
		log.Printf("AGWPE TNC (%s) initialized", v)
	}

	transport.RegisterContextDialer(MethodAX25AGWPE, agwpeTNC)
	return nil
}

// defaultAX25Method resolves the generic ax25:// scheme to a implementation specific scheme.
func defaultAX25Method() string {
	switch config.AX25.Engine {
	case cfg.AX25EngineAGWPE:
		return MethodAX25AGWPE
	case cfg.AX25EngineSerialTNC:
		return MethodAX25SerialTNC
	case cfg.AX25EngineLinux:
		return MethodAX25Linux
	default:
		panic(fmt.Sprintf("invalid ax25 engine: %s", config.AX25.Engine))
	}
}
