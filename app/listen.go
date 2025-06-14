// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"context"
	"log"
	"net"
	"strings"
	"time"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/ax25/agwpe"
	"github.com/la5nta/wl2k-go/transport/telnet"
	"github.com/n8jja/Pat-Vara/vara"
)

func (a *App) Unlisten(param string) {
	methods := strings.FieldsFunc(param, SplitFunc)
	for _, method := range methods {
		ok, err := a.listenHub.Disable(method)
		if err != nil {
			log.Printf("Unable to close %s listener: %s", method, err)
		} else if !ok {
			log.Printf("No active %s listener, ignoring.\n", method)
		}
	}
}

func (a *App) Listen(listenStr string) {
	methods := strings.FieldsFunc(listenStr, SplitFunc)
	for _, method := range methods {
		// Rewrite the generic ax25:// scheme to use a specified AX.25 engine.
		if method == MethodAX25 {
			method = a.defaultAX25Method()
		}

		switch strings.ToLower(method) {
		case MethodArdop:
			a.listenHub.Enable(&ARDOPListener{a, nil})
		case MethodTelnet:
			a.listenHub.Enable(TelnetListener{a})
		case MethodAX25AGWPE:
			a.listenHub.Enable(&AX25AGWPEListener{a, nil})
		case MethodAX25Linux:
			a.listenHub.Enable(&AX25LinuxListener{a, nil})
		case MethodVaraFM:
			a.listenHub.Enable(VaraFMListener{a})
		case MethodVaraHF:
			a.listenHub.Enable(VaraHFListener{a})
		case MethodAX25SerialTNC, MethodSerialTNCDeprecated:
			log.Printf("%s listen not implemented, ignoring.", method)
		default:
			log.Printf("'%s' is not a valid listen method", method)
			return
		}
	}
	log.Printf("Listening for incoming traffic on %s...", listenStr)
}

type AX25LinuxListener struct {
	a interface {
		Options() Options
		Config() cfg.Config
	}

	stopBeacon func()
}

func (l *AX25LinuxListener) Init() (net.Listener, error) {
	return ax25.ListenAX25(l.a.Config().AX25Linux.Port, l.a.Options().MyCall)
}

func (l *AX25LinuxListener) BeaconStart() error {
	interval := time.Duration(l.a.Config().AX25.Beacon.Every) * time.Second
	if interval <= 0 {
		return nil
	}
	b, err := ax25.NewAX25Beacon(l.a.Config().AX25Linux.Port, l.a.Options().MyCall, l.a.Config().AX25.Beacon.Destination, l.a.Config().AX25.Beacon.Message)
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

type ARDOPListener struct {
	a interface {
		ARDOP() (*ardop.TNC, error)
		VFOForRig(string) (hamlib.VFO, bool)
		Config() cfg.Config
	}

	stopBeacon func()
}

func (l ARDOPListener) Name() string { return MethodArdop }
func (l ARDOPListener) Init() (net.Listener, error) {
	m, err := l.a.ARDOP()
	if err != nil {
		return nil, err
	}
	return m.Listen()
}

func (l ARDOPListener) CurrentFreq() (Frequency, bool) {
	if rig, ok := l.a.VFOForRig(l.a.Config().Ardop.Rig); ok {
		f, _ := rig.GetFreq()
		return Frequency(f), ok
	}
	return 0, false
}

func (l *ARDOPListener) BeaconStart() error {
	interval := time.Duration(l.a.Config().Ardop.BeaconInterval) * time.Second
	if interval <= 0 {
		return nil
	}
	m, err := l.a.ARDOP()
	if err != nil {
		return err
	}
	l.stopBeacon = func() { m.BeaconEvery(0) }
	return m.BeaconEvery(interval)
}

func (l ARDOPListener) BeaconStop() {
	if l.stopBeacon != nil {
		l.stopBeacon()
	}
}

type VaraFMListener struct {
	a interface {
		Config() cfg.Config
		VFOForRig(string) (hamlib.VFO, bool)
		VARAFM() (*vara.Modem, error)
	}
}

func (l VaraFMListener) Name() string { return MethodVaraFM }
func (l VaraFMListener) Init() (net.Listener, error) {
	m, err := l.a.VARAFM()
	if err != nil {
		return nil, err
	}
	return m.Listen()
}

func (l VaraFMListener) CurrentFreq() (Frequency, bool) {
	if rig, ok := l.a.VFOForRig(l.a.Config().VaraFM.Rig); ok {
		f, _ := rig.GetFreq()
		return Frequency(f), ok
	}
	return 0, false
}

type VaraHFListener struct {
	a interface {
		Config() cfg.Config
		VFOForRig(string) (hamlib.VFO, bool)
		VARAHF() (*vara.Modem, error)
	}
}

func (l VaraHFListener) Name() string { return MethodVaraHF }
func (l VaraHFListener) Init() (net.Listener, error) {
	m, err := l.a.VARAHF()
	if err != nil {
		return nil, err
	}
	return m.Listen()
}

func (l VaraHFListener) CurrentFreq() (Frequency, bool) {
	if rig, ok := l.a.VFOForRig(l.a.Config().VaraHF.Rig); ok {
		f, _ := rig.GetFreq()
		return Frequency(f), ok
	}
	return 0, false
}

type AX25AGWPEListener struct {
	a interface {
		Config() cfg.Config
		AGWPE() (*agwpe.TNCPort, error)
	}

	stopBeacon func()
}

func (l *AX25AGWPEListener) Name() string { return MethodAX25AGWPE }

func (l *AX25AGWPEListener) Init() (net.Listener, error) {
	m, err := l.a.AGWPE()
	if err != nil {
		return nil, err
	}
	return m.Listen()
}

func (l *AX25AGWPEListener) CurrentFreq() (Frequency, bool) { return 0, false }

func (l *AX25AGWPEListener) BeaconStart() error {
	b := l.a.Config().AX25.Beacon
	interval := time.Duration(b.Every) * time.Second
	if interval <= 0 {
		return nil
	}
	m, err := l.a.AGWPE()
	if err != nil {
		return err
	}
	l.stopBeacon = doEvery(interval, func() {
		if err := m.SendUI([]byte(b.Message), b.Destination); err != nil {
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

type TelnetListener struct {
	a interface {
		Config() cfg.Config
	}
}

func (l TelnetListener) Name() string { return MethodTelnet }
func (l TelnetListener) Init() (net.Listener, error) {
	return telnet.Listen(l.a.Config().Telnet.ListenAddr)
}
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
