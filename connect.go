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

	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/winmor"

	// Register ax25 and telnet dialers
	_ "github.com/la5nta/wl2k-go/transport/ax25"
	_ "github.com/la5nta/wl2k-go/transport/telnet"
)

var (
	wmTNC *winmor.TNC // Pointer to the WINMOR TNC used by Listen and Connect
	adTNC *ardop.TNC  // Pointer to the ARDOP TNC used by Listen and Connect
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

	url, err := transport.ParseURL(connectStr)
	if err != nil {
		log.Println(err)
		return false
	}

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
				url.Params.Set("baud", fmt.Sprint(config.SerialTNC.Baudrate))
			}
		case "ptc":
			url.Host = config.PTC.Path
			if config.PTC.Baudrate > 0 {
				url.Params.Set("baud", fmt.Sprint(config.PTC.Baudrate))
			}
		}
	}

	if url.Scheme == "ptc" && config.PTC.InitScript != "" && url.Params.Get("init_script") == "" {
		url.Params.Set("init_script", config.PTC.InitScript)
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
	if vfo, ok := VFOForTransport(url.Scheme); ok {
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

	log.Printf("Connecting to %s (%s)...", url.Target, url.Scheme)
	conn, err := transport.DialURL(url)

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

func qsy(method, addr string) (revert func(), err error) {
	noop := func() {}

	var rigName string
	switch method {
	case MethodWinmor:
		rigName = config.Winmor.Rig
	case MethodArdop:
		rigName = config.Ardop.Rig
	case MethodAX25:
		rigName = config.AX25.Rig
	case MethodPTC:
		rigName = config.PTC.Rig
	default:
		return noop, fmt.Errorf("Not supported with transport '%s'", method)
	}

	if rigName == "" {
		return noop, fmt.Errorf("Missing rig reference in config section for %s, don't know which rig to qsy", method)
	}

	var ok bool
	rig, ok := rigs[rigName]
	if !ok {
		return noop, fmt.Errorf("Hamlib rig '%s' not loaded.", rigName)
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
