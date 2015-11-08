// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/telnet"
)

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
	}

	if aliased, ok := config.ConnectAliases[connectStr]; ok {
		return Connect(aliased)
	}

	url, err := url.Parse(connectStr)
	if err != nil {
		log.Println(err)
		return false
	}

	targetcall := path.Base(url.Path)

	if len(targetcall) < 3 {
		log.Println("Missing targetcall in connection URL")
		return false
	}

	// QSY
	var revertFreq func()
	if freq := url.Query().Get("freq"); freq != "" {
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

	log.Printf("Connecting to %s...", url)

	var conn net.Conn
	switch url.Scheme {
	case MethodWinmor:
		done := handleInterrupt()
		conn, err = connectWinmor(targetcall)
		close(done)
	case MethodArdop:
		done := handleInterrupt()
		conn, err = connectArdop(targetcall)
		close(done)
	case MethodTelnet:
		var user, pass string
		if url.User != nil {
			pass, _ = url.User.Password()
			user = url.User.Username()
		}
		if user == "" {
			user = fOptions.MyCall
		}
		conn, err = telnet.Dial(url.Host, user, pass)
	case MethodAX25:
		axport := url.Host
		if axport == "" {
			axport = config.AX25.Port
		}
		conn, err = ax25.DialAX25Timeout(
			axport,
			fOptions.MyCall,
			targetcall,
			45*time.Second,
		)

	case MethodSerialTNC:
		conn, err = ax25.DialKenwood(
			config.SerialTNC.Path,
			fOptions.MyCall,
			targetcall,
			ax25.NewConfig(ax25.Baudrate(config.SerialTNC.Baudrate)),
			nil,
		)

	default:
		log.Printf("'%s' is not a valid transport scheme.", url.Scheme)
		return
	}

	eventLog.LogConn("connect "+connectStr, currFreq, conn, err)

	if err != nil {
		log.Printf("Unable to establish connection to remote: %s", err)
		return
	}

	err = exchange(conn, targetcall, false)
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

	time.Sleep(2 * time.Second)

	return func() {
		time.Sleep(time.Second)
		log.Printf("QSX %s: %.3f", method, float64(oldFreq)/1e3)
		rig.SetFreq(oldFreq)
	}, nil
}

func connectWinmor(target string) (net.Conn, error) {
	if wmTNC == nil {
		initWinmorTNC()
	}

	waitBusy(wmTNC)
	return wmTNC.Dial(target)
}

func connectArdop(target string) (net.Conn, error) {
	if adTNC == nil {
		initArdopTNC()
	}

	waitBusy(adTNC)
	return adTNC.Dial(target)
}

func parseConnectURI(uri string) (callsign, password, addr string, err error) {
	parts := strings.Split(uri, "@")
	if len(parts) > 1 {
		addr = parts[1]
		uri = parts[0]
	}

	parts = strings.Split(uri, ":")

	callsign = parts[0]
	if callsign == "" {
		err = fmt.Errorf("Invalid connect uri, missing call sign.")
	}

	if len(parts) > 1 {
		password = parts[1]
	}

	return
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
