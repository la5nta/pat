// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/harenber/ptc-go/pactor/v2"
	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/winmor"

	// Register other dialers
	_ "github.com/la5nta/wl2k-go/transport/ax25"
	_ "github.com/la5nta/wl2k-go/transport/telnet"
)

var (
	dialing *transport.URL // The connect URL currently being dialed (if any)
	wmTNC   *winmor.TNC    // Pointer to the WINMOR TNC used by Listen and Connect
	adTNC   *ardop.TNC     // Pointer to the ARDOP TNC used by Listen and Connect
	pModem  *pactor.Modem
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
		log.Printf("DJC aliased:%s", aliased)
		return Connect(aliased)
	}

	url, err := transport.ParseURL(connectStr)
	if err != nil {
		log.Println(err)
		return false
	}
	log.Printf("DJC Connect() connectStr:%s", connectStr)
	log.Printf("DJC Connect() url:%s", url)

	// Init TNCs
	switch url.Scheme {
	case "ardop":
		if err := initArdopTNC(); err != nil {
			log.Println(err)
			return
		}
	case "winmor":
		if err := initWinmorTNC(); err != nil {
			log.Println(err)
			return
		}
	case "pactor":
		pt_cmdinit := ""
		if val, ok := url.Params["init"]; ok {
			pt_cmdinit = strings.Join(val, "\n")
		}
		if err := initPactorModem(pt_cmdinit); err != nil {
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
		case "ax25":
			url.Host = config.AX25.Port
		case "serial-tnc":
			url.Host = config.SerialTNC.Path
			if config.SerialTNC.Baudrate > 0 {
				url.Params.Set("hbaud", fmt.Sprint(config.SerialTNC.Baudrate))
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

		switch url.Scheme {
		case "ax25", "serial-tnc":
			log.Printf("Radio-Only is not available for %s", url.Scheme)
			return
		default:
			url.SetUser(url.User.Username() + "-T")
		}
	}

	// QSY
	var revertVFOState func()

	freq := url.Params.Get("freq")
	rigmode := strings.ToUpper(url.Params.Get("rig_mode"))
	log.Printf("DJC connect freq:%s, rigmode:%s", freq, rigmode)

	if freq != "" {
		revertVFOState, err = qsy(url.Scheme, freq, rigmode)
		if err != nil {
			log.Printf("Unable to QSY: %s", err)
			return
		}
		defer revertVFOState()
	}

	var currFreq Frequency
	if vfo, _, ok, _ := VFOForTransport(url.Scheme); ok {
		f, _ := vfo.GetFreq()
		currFreq = Frequency(f)
	}

	// Wait for a clear channel
	switch url.Scheme {
	case "ardop":
		waitBusy(adTNC)
	case "winmor":
		waitBusy(wmTNC)
	}

	// Catch interrupts (signals) while dialing, so users can abort ardop/winmor connects.
	doneHandleInterrupt := handleInterrupt()

	// Signal web gui that we are dialing a connection
	dialing = url
	websocketHub.UpdateStatus()

	log.Printf("Connecting to %s (%s)...", url.Target, url.Scheme)
	conn, err := transport.DialURL(url)

	// Signal web gui that we are no longer dialing
	dialing = nil
	websocketHub.UpdateStatus()

	close(doneHandleInterrupt)

	eventLog.LogConn("connect "+connectStr, currFreq, conn, err)

	if err != nil {
		log.Printf("Unable to establish connection to remote: %s", err)
		return
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

func qsy(method, addr string, rigmode string) (revert func(), err error) {
	log.Printf("DJC qsy method:%v addr:%v rigmode:%v", method, addr, rigmode)
	noop := func() {}
	rig, rigName, ok, err := VFOForTransport(method)
	if err != nil {
		return noop, err
	} else if !ok {
		return noop, fmt.Errorf("Hamlib rig '%s' not loaded.", rigName)
	}

	log.Printf("QSY %s: %s %s", method, addr, rigmode)
	_, oldFreq, err := setFreq(rig, addr)
	if err != nil {
		return noop, err
	}

	var oldrigmode, oldbw string
	if rigmode != "" { // If rigmode is to be set then we need to preserve and restore the old rigmode and bandwidth
		log.Printf("DJC qsy - about to setRigMode %s %s", rigmode, "0")
		oldrigmode, oldbw, err = setRigMode(rig, rigmode, "0") // For now, always take the default bandwidth for the given mode.
		if err != nil {
			log.Printf("DJC qsy - setRigMode() failed:%v", err)
			return noop, err
		}
	} else { // rigmode was not specified, no need to preserve old rigmode - just keep whatever
		log.Printf("DJC qsy - no rigmode set, none preserved.")
	}
	log.Printf("DJC qsy oldrigmode:%s oldbw:%s rigmode:%s", oldrigmode, oldbw, rigmode)

	time.Sleep(3 * time.Second)

	return func() {
		time.Sleep(time.Second)
		log.Printf("QSX %s: %.3f %s %s", method, float64(oldFreq)/1e3, oldrigmode, oldbw)
		rig.SetFreq(oldFreq)
		if oldrigmode != "" {
			// Didn't change rigmode, don't restore it.
			rig.SetModeAsString(oldrigmode, oldbw)
		}
	}, nil
}

func waitBusy(b transport.BusyChannelChecker) {
	printed := false

	for b.Busy() {
		if !printed && fOptions.IgnoreBusy {
			log.Println("Ignoring busy channel!")
			break
		} else if !printed {
			log.Println("Waiting for clear channel...")
			printed = true
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func initWinmorTNC() error {
	if wmTNC != nil && wmTNC.Ping() == nil {
		return nil
	}

	if wmTNC != nil {
		wmTNC.Close()
	}

	var err error
	wmTNC, err = winmor.Open(config.Winmor.Addr, fOptions.MyCall, config.Locator)
	if err != nil {
		return fmt.Errorf("WINMOR TNC initialization failed: %s", err)
	}

	if config.Winmor.DriveLevel != 0 {
		if err := wmTNC.SetDriveLevel(config.Winmor.DriveLevel); err != nil {
			log.Println("Failed to set WINMOR drive level:", err)
		}
	}

	if v, err := wmTNC.Version(); err != nil {
		return fmt.Errorf("WINMOR TNC initialization failed: %s", err)
	} else {
		log.Printf("WINMOR TNC v%s initialized", v)
	}

	transport.RegisterDialer("winmor", wmTNC)

	if !config.Winmor.PTTControl {
		return nil
	}

	rig, ok := rigs[config.Winmor.Rig]
	if !ok {
		return fmt.Errorf("Unable to set PTT rig '%s': Not defined or not loaded.", config.Winmor.Rig)
	}
	wmTNC.SetPTT(rig)

	return nil
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
		return fmt.Errorf("ARDOP TNC initialization failed: %s", err)
	}

	if !config.Ardop.ARQBandwidth.IsZero() {
		if err := adTNC.SetARQBandwidth(config.Ardop.ARQBandwidth); err != nil {
			return fmt.Errorf("Unable to set ARQ bandwidth for ardop TNC: %s", err)
		}
	}

	if err := adTNC.SetCWID(config.Ardop.CWID); err != nil {
		return fmt.Errorf("Unable to configure CWID for ardop TNC: %s", err)
	}

	if v, err := adTNC.Version(); err != nil {
		return fmt.Errorf("ARDOP TNC initialization failed: %s", err)
	} else {
		log.Printf("ARDOP TNC (%s) initialized", v)
	}

	transport.RegisterDialer("ardop", adTNC)

	if !config.Ardop.PTTControl {
		return nil
	}

	rig, ok := rigs[config.Ardop.Rig]
	if !ok {
		return fmt.Errorf("Unable to set PTT rig '%s': Not defined or not loaded.", config.Ardop.Rig)
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
		return fmt.Errorf("Pactor initialization failed: %s", err)
	}

	transport.RegisterDialer("pactor", pModem)

	return nil
}
