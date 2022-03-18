// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
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
		switch strings.ToLower(method) {
		case MethodArdop:
			listenHub.Enable(ARDOPListener{})
		case MethodTelnet:
			listenHub.Enable(TelnetListener{})
		case MethodAX25:
			listenHub.Enable(&AX25Listener{})
		case MethodSerialTNC:
			log.Printf("%s listen not implemented, ignoring.", method)
		default:
			log.Printf("'%s' is not a valid listen method", method)
			return
		}
	}
	log.Printf("Listening for incoming traffic on %s...", listenStr)
}

type AX25Listener struct{ stopBeacon chan<- struct{} }

func (l *AX25Listener) Init() (net.Listener, error) {
	return ax25.ListenAX25(config.AX25.Port, fOptions.MyCall)
}

func (l *AX25Listener) BeaconStart() error {
	if config.AX25.Beacon.Every > 0 {
		l.stopBeacon = l.beaconLoop(time.Duration(config.AX25.Beacon.Every) * time.Second)
	}
	return nil
}

func (l *AX25Listener) BeaconStop() {
	select {
	case l.stopBeacon <- struct{}{}:
	default:
	}
}

func (l *AX25Listener) beaconLoop(dur time.Duration) chan<- struct{} {
	stop := make(chan struct{}, 1)
	go func() {
		b, err := ax25.NewAX25Beacon(config.AX25.Port, fOptions.MyCall, config.AX25.Beacon.Destination, config.AX25.Beacon.Message)
		if err != nil {
			log.Printf("Unable to activate beacon: %s", err)
			return
		}

		t := time.Tick(dur)
		for {
			select {
			case <-t:
				if err := b.Now(); err != nil {
					log.Printf("%s beacon failed: %s", l.Name(), err)
					return
				}
			case <-stop:
				return
			}
		}
	}()
	return stop
}

func (l *AX25Listener) CurrentFreq() (Frequency, bool) { return 0, false }
func (l *AX25Listener) Name() string                   { return MethodAX25 }

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

type TelnetListener struct{}

func (l TelnetListener) Name() string                   { return MethodTelnet }
func (l TelnetListener) Init() (net.Listener, error)    { return telnet.Listen(config.Telnet.ListenAddr) }
func (l TelnetListener) CurrentFreq() (Frequency, bool) { return 0, false }
