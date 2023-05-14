// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"log"
	"net"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/telnet"
)

func Unlisten(param string) {
	methods := strings.FieldsFunc(param, SplitFunc)
	for _, method := range methods {
		ok, err := listenHub.Disable(method)
		if err != nil {
			log.Printf("Unable to close %s listener: %s", method, err)
		} else if !ok {
			log.Printf("No active %s listener, ignoring.\n", method)
		}
	}
}

func Listen(listenStr string) {
	methods := strings.FieldsFunc(listenStr, SplitFunc)
	for _, method := range methods {
		// Rewrite the generic ax25:// scheme to use a specified AX.25 engine.
		if method == MethodAX25 {
			method = defaultAX25Method()
		}

		switch strings.ToLower(method) {
		case MethodArdop:
			listenHub.Enable(ARDOPListener{})
		case MethodTelnet:
			listenHub.Enable(TelnetListener{})
		case MethodAX25AGWPE:
			listenHub.Enable(&AX25AGWPEListener{})
		case MethodAX25Linux:
			listenHub.Enable(&AX25LinuxListener{})
		case MethodVaraFM:
			listenHub.Enable(VaraFMListener{})
		case MethodVaraHF:
			listenHub.Enable(VaraHFListener{})
		case MethodAX25SerialTNC, MethodSerialTNC:
			log.Printf("%s listen not implemented, ignoring.", method)
		default:
			log.Printf("'%s' is not a valid listen method", method)
			return
		}
	}
	log.Printf("Listening for incoming traffic on %s...", listenStr)
}

type AX25LinuxListener struct{ stopBeacon func() }

func (l *AX25LinuxListener) Init() (net.Listener, error) {
	return ax25.ListenAX25(config.AX25Linux.Port, fOptions.MyCall)
}

func (l *AX25LinuxListener) BeaconStart() error {
	interval := time.Duration(config.AX25.Beacon.Every) * time.Second
	if interval == 0 {
		return nil
	}
	b, err := ax25.NewAX25Beacon(config.AX25Linux.Port, fOptions.MyCall, config.AX25.Beacon.Destination, config.AX25.Beacon.Message)
	if err != nil {
		return err
	}
	l.stopBeacon = doEvery(interval, func() {
		if err := b.Now(); err != nil {
			log.Printf("%s beacon failed: %s", l.Name(), err)
			l.stopBeacon()
		}
	})
	return nil
}

func (l *AX25LinuxListener) BeaconStop() {
	if l.stopBeacon != nil {
		l.stopBeacon()
	}
}

func (l *AX25LinuxListener) CurrentFreq() (Frequency, bool) { return 0, false }
func (l *AX25LinuxListener) Name() string                   { return MethodAX25Linux }

type ARDOPListener struct{}

func (l ARDOPListener) Name() string { return MethodArdop }
func (l ARDOPListener) Init() (net.Listener, error) {
	if err := initArdopTNC(); err != nil {
		return nil, err
	}
	ln, err := adTNC.Listen()
	if err != nil {
		return nil, err
	}
	return ln, err
}

func (l ARDOPListener) CurrentFreq() (Frequency, bool) {
	if rig, ok := rigs[config.Ardop.Rig]; ok {
		f, _ := rig.GetFreq()
		return Frequency(f), ok
	}
	return 0, false
}

func (l ARDOPListener) BeaconStart() error {
	return adTNC.BeaconEvery(time.Duration(config.Ardop.BeaconInterval) * time.Second)
}

func (l ARDOPListener) BeaconStop() { adTNC.BeaconEvery(0) }

type VaraFMListener struct{}

func (l VaraFMListener) Name() string { return MethodVaraFM }
func (l VaraFMListener) Init() (net.Listener, error) {
	if err := initVaraFMModem(); err != nil {
		return nil, err
	}
	ln, err := varaFMModem.Listen()
	if err != nil {
		return nil, err
	}
	return ln, err
}

func (l VaraFMListener) CurrentFreq() (Frequency, bool) {
	if rig, ok := rigs[config.VaraFM.Rig]; ok {
		f, _ := rig.GetFreq()
		return Frequency(f), ok
	}
	return 0, false
}

type VaraHFListener struct{}

func (l VaraHFListener) Name() string { return MethodVaraHF }
func (l VaraHFListener) Init() (net.Listener, error) {
	if err := initVaraHFModem(); err != nil {
		return nil, err
	}
	ln, err := varaHFModem.Listen()
	if err != nil {
		return nil, err
	}
	return ln, err
}

func (l VaraHFListener) CurrentFreq() (Frequency, bool) {
	if rig, ok := rigs[config.VaraHF.Rig]; ok {
		f, _ := rig.GetFreq()
		return Frequency(f), ok
	}
	return 0, false
}

type AX25AGWPEListener struct{ stopBeacon func() }

func (l *AX25AGWPEListener) Name() string { return MethodAX25AGWPE }

func (l *AX25AGWPEListener) Init() (net.Listener, error) {
	if err := initAGWPE(); err != nil {
		return nil, err
	}
	return agwpeTNC.Listen()
}

func (l *AX25AGWPEListener) CurrentFreq() (Frequency, bool) { return 0, false }

func (l *AX25AGWPEListener) BeaconStart() error {
	b := config.AX25.Beacon
	interval := time.Duration(b.Every) * time.Second
	l.stopBeacon = doEvery(interval, func() {
		if err := agwpeTNC.SendUI([]byte(b.Message), b.Destination); err != nil {
			log.Printf("%s beacon failed: %s", l.Name(), err)
			l.stopBeacon()
		}
	})
	return nil
}

func (l AX25AGWPEListener) BeaconStop() {
	if l.stopBeacon != nil {
		l.stopBeacon()
	}
}

type TelnetListener struct{}

func (l TelnetListener) Name() string                   { return MethodTelnet }
func (l TelnetListener) Init() (net.Listener, error)    { return telnet.Listen(config.Telnet.ListenAddr) }
func (l TelnetListener) CurrentFreq() (Frequency, bool) { return 0, false }

func doEvery(interval time.Duration, fn func()) (cancel func()) {
	if interval == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fn()
			}
		}
	}()
	return cancel
}
