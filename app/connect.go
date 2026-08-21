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
	"github.com/la5nta/pat/internal/varanny"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"

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
		if err := a.initAGWPE(); err != nil {
			log.Println(err)
			return
		}
	case MethodArdop:
		if err := a.initARDOP(); err != nil {
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
		if err := a.initVARAHF(); err != nil {
			log.Println(err)
			return
		}
	case MethodVaraFM:
		if err := a.initVARAFM(); err != nil {
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
			// Clean up varanny session before returning
			if url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM {
				if _, ok := a.getVarannySession(url.Scheme); ok {
					log.Printf("Radio Only SSID check failed, cleaning up varanny session for %s", url.Scheme)
					a.removeVarannySession(url.Scheme)
				}
			}
			return
		}

		if strings.HasPrefix(url.Scheme, MethodAX25) {
			log.Printf("Radio-Only is not available for %s", url.Scheme)
			// Clean up varanny session before returning
			if url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM {
				if _, ok := a.getVarannySession(url.Scheme); ok {
					log.Printf("Radio Only unavailable for transport, cleaning up varanny session for %s", url.Scheme)
					a.removeVarannySession(url.Scheme)
				}
			}
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
			// Clean up varanny session before returning
			if url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM {
				if _, ok := a.getVarannySession(url.Scheme); ok {
					log.Printf("QSY failed, cleaning up varanny session for %s", url.Scheme)
					a.removeVarannySession(url.Scheme)
				}
			}
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
			// Clean up varanny session before returning
			if url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM {
				if _, ok := a.getVarannySession(url.Scheme); ok {
					log.Printf("Prehook verification failed, cleaning up varanny session for %s", url.Scheme)
					a.removeVarannySession(url.Scheme)
				}
			}
			return
		}
	}

	log.Printf("Connecting to %s (%s)...", url.Target, url.Scheme)
	conn, err := transport.DialURLContext(ctx, url)

	if err != nil {
		// Check if modem is still alive for varanny
		if a.varaHF != nil && (url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM) {
			alive := a.varaHF.Ping()
			if !alive {
				log.Printf("VARA modem not responding, will re-initialize on next attempt")
				// Trigger re-initialization on next connect attempt
				a.varaHF = nil
			}
		}
	}

	// Signal web gui that we are no longer dialing
	a.dialing = nil
	a.websocketHub.UpdateStatus()

	a.eventLog.LogConn("connect "+connectStr, currFreq, conn, err)

	// Clean up varanny session if dial failed and varanny was used
	if err != nil && (url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM) {
		if _, ok := a.getVarannySession(url.Scheme); ok {
			log.Printf("Dial failed, cleaning up varanny session for %s", url.Scheme)
			a.removeVarannySession(url.Scheme)
		}
	}

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

	// Clean up varanny connection if this specific connection used varanny
	// NOTE: Don't clean up for listen mode - that's handled separately in app.Close()
	if url.Scheme == MethodVaraHF || url.Scheme == MethodVaraFM {
		if _, ok := a.getVarannySession(url.Scheme); ok {
			log.Printf("Cleaning up varanny session for %s", url.Scheme)
			a.removeVarannySession(url.Scheme)
		}
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

func (a *App) initARDOP() error {
	if a.ardop != nil && a.ardop.Ping() == nil {
		return nil
	}

	if a.ardop != nil {
		a.ardop.Close()
	}

	var err error
	a.ardop, err = ardop.OpenTCP(a.config.Ardop.Addr, a.options.MyCall, a.config.Locator)
	if err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %w", err)
	}

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
	}
	m, err := a.initVARA(MethodVaraHF, a.config.VaraHF, true)
	if err != nil {
		return err
	}
	// Bandwidth is now set in initVARA, not here
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
	}
	m, err := a.initVARA(MethodVaraFM, a.config.VaraFM, false)
	if err != nil {
		return err
	}
	a.varaFM = m
	return nil
}

func (a *App) initVARA(scheme string, conf cfg.VaraConfig, isHF bool) (*vara.Modem, error) {
	// Try varanny if enabled
	var useVaranny bool
	var modemInfo varanny.ModemInfo
	var varannySession *varanny.Session

	if a.config.Varanny.Enable {
		modemType := "hf"
		if !isHF {
			modemType = "fm"
		}

		var ok bool
		modemInfo, ok = a.findVarannyModem(modemType)
		if ok {
			// Try to connect via varanny
			client, err := a.connectVaranny(modemInfo)
			if err == nil {
				useVaranny = true

				// Create session (will be published after fully constructed)
				varannySession = varanny.NewSession(client, modemInfo, scheme)

				log.Printf("Using varanny for %s: %s", scheme, modemInfo.Name)
			} else {
				log.Printf("Failed to connect to varanny for %s: %v", scheme, err)
				if a.config.Varanny.FallbackToDirect {
					log.Printf("Falling back to direct VARA connection")
				} else {
					return nil, fmt.Errorf("varanny connection failed and fallback disabled: %w", err)
				}
			}
		} else {
			log.Printf("No varanny modems found for type=%s", modemType)
			if a.config.Varanny.FallbackToDirect {
				log.Printf("Falling back to direct VARA connection")
			} else {
				return nil, fmt.Errorf("varanny enabled but no modems found and fallback disabled")
			}
		}
	}

	// Determine connection parameters
	var modemHost string
	var modemCmdPort, modemDataPort int

	if useVaranny {
		modemHost = modemInfo.Host
		modemCmdPort = modemInfo.CmdPort
		modemDataPort = modemInfo.DataPort
	} else {
		modemHost = conf.Host()
		modemCmdPort = conf.CmdPort()
		modemDataPort = conf.DataPort()
	}

	// Create VARA modem
	vConf := vara.ModemConfig{
		Host:     modemHost,
		CmdPort:  modemCmdPort,
		DataPort: modemDataPort,
	}

	m, err := vara.NewModem(scheme, a.options.MyCall, vConf)
	if err != nil {
		if useVaranny {
			// Session might not be in map yet if this error occurs early
			if varannySession != nil {
				varannySession.Close()
				varannySession = nil
			} else {
				a.removeVarannySession(scheme)
			}
		}
		return nil, fmt.Errorf("vara initialization failed: %w", err)
	}

	// Register dialer
	transport.RegisterDialer(scheme, m)
	m.SetBusyFunc(a.onBusyChannel)

	// Setup bandwidth for HF
	if isHF {
		if bw := a.config.VaraHF.Bandwidth; bw != 0 {
			if useVaranny {
				// Don't set bandwidth via VARA library when using varanny
				// varanny should handle this via its configuration templates
			} else if err := m.SetBandwidth(fmt.Sprint(bw)); err != nil {
				m.Close()
				if useVaranny {
					// Session might not be in map yet if this error occurs early
					if varannySession != nil {
						varannySession.Close()
						varannySession = nil
					} else {
						a.removeVarannySession(scheme)
					}
				}
				return nil, fmt.Errorf("unable to set bandwidth: %w", err)
			}
		}
	}

	// Setup PTT
	rig := ""
	if isHF {
		rig = a.config.VaraHF.Rig
	} else {
		rig = a.config.VaraFM.Rig
	}

	if useVaranny && a.config.Varanny.UseVarannyCAT {
		// Use varanny's CAT control if available
		if modemInfo.CatPort != 0 && modemInfo.CatDialect == "hamlib" {
			hamlibRig, err := hamlib.Open("tcp",
				fmt.Sprintf("%s:%d", modemInfo.Host, modemInfo.CatPort))
			if err == nil {
				// Get current VFO which implements SetPTT
				vfo := hamlibRig.CurrentVFO()
				m.SetPTT(vfo)
				log.Printf("Using varanny CAT control for %s", scheme)

				// Store the hamlib closer in the session so it's cleaned up properly
				if varannySession != nil {
					varannySession.SetCATCloser(hamlibRig)
				}
			}
		}
	}

	// Publish the fully-constructed session to prevent data race
	// (catCloser is set before this point, and the session is only visible
	// to other goroutines after being published to the map)
	if useVaranny && varannySession != nil {
		a.setVarannySession(scheme, varannySession)
		// Clear local reference so error paths don't double-close
		varannySession = nil
	}

	if conf.PTTControl {
		// Fall back to pat's hamlib
		r, ok := a.rigs[rig]
		if !ok {
			m.Close()
			// Session is already published at this point, remove from map
			if useVaranny {
				a.removeVarannySession(scheme)
			}
			return nil, fmt.Errorf("unable to set PTT rig '%s': not defined or not loaded", rig)
		}
		m.SetPTT(r)
	}

	v, _ := m.Version()
	via := "direct connection"
	if useVaranny {
		via = "varanny"
	}
	log.Printf("VARA modem (%s) initialized via %s", v, via)
	return m, nil
}

// connectVaranny connects to varanny and starts the modem.
func (a *App) connectVaranny(modemInfo varanny.ModemInfo) (*varanny.Client, error) {
	client, err := varanny.Connect(a.varannyCtx, modemInfo.Host, modemInfo.LaunchPort)
	if err != nil {
		return nil, err
	}

	// Startup timeout from config (higher for emulation/Wine)
	startupTimeout := time.Duration(a.config.Varanny.StartupTimeout) * time.Second
	commandTimeout := time.Duration(a.config.Varanny.CommandTimeout) * time.Second

	// Start the modem - use startup timeout for the start command itself
	// since VARA initialization can take a while, especially on first start
	startCtx, startCancel := context.WithTimeout(a.varannyCtx, startupTimeout)
	err = client.StartModem(startCtx, modemInfo.Name)
	startCancel()

	// If modem is already running, try to connect to existing instance
	if err != nil && strings.Contains(err.Error(), "already running") {
		log.Printf("Varanny modem %s is already running, checking if ports are available", modemInfo.Name)
		// Close control connection since we don't need it for already-running modems
		client.Close()
		client = nil

		// Try to connect to the VARA ports directly to verify they're available
		bindCtx, bindCancel := context.WithTimeout(context.Background(), startupTimeout)
		defer bindCancel()

		if err = varanny.WaitForPortBinding(bindCtx, modemInfo.Host, modemInfo.CmdPort, modemInfo.DataPort); err != nil {
			return nil, fmt.Errorf("VARA ports not available: %w", err)
		}
		log.Printf("Using existing VARA instance on %s", modemInfo.Host)
		// Clear the error since we can use the existing instance
		err = nil
	}

	if err != nil {
		if client != nil {
			client.Close()
		}
		return nil, err
	}

	// Wait for VARA to bind to ports (cancellable via context)
	// Use startup timeout since this is part of the startup sequence
	log.Printf("Waiting for VARA to bind to ports on %s...", modemInfo.Host)
	bindCtx, bindCancel := context.WithTimeout(a.varannyCtx, startupTimeout)
	if err := varanny.WaitForPortBinding(bindCtx,
		modemInfo.Host, modemInfo.CmdPort, modemInfo.DataPort); err != nil {
		bindCancel()
		// Stop the modem with a fresh bounded context
		stopCtx, stopCancel := context.WithTimeout(context.Background(), commandTimeout)
		if stopErr := client.StopModem(stopCtx); stopErr != nil {
			stopCancel()
			log.Printf("Error stopping varanny modem after port bind failure: %v", stopErr)
		} else {
			stopCancel()
		}
		client.Close()
		return nil, fmt.Errorf("VARA port binding failed: %w", err)
	}
	bindCancel()

	return client, nil
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
	}

	var err error
	a.agwpe, err = agwpe.OpenPortTCP(a.config.AGWPE.Addr, a.config.AGWPE.RadioPort, a.options.MyCall)
	if err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	}

	if v, err := a.agwpe.Version(); err != nil {
		return fmt.Errorf("AGWPE TNC initialization failed: %w", err)
	} else {
		log.Printf("AGWPE TNC (%s) initialized", v)
	}

	transport.RegisterContextDialer(MethodAX25AGWPE, a.agwpe)
	return nil
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
