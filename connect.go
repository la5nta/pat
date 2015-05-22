// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/telnet"
)

func ConnectDefaults() {
	if len(config.ConnectDefaults) == 0 {
		log.Println("No default connect methods defined in the configuration.")
		return
	}

	for _, connectStr := range config.ConnectDefaults {
		if Connect(connectStr) {
			break
		}
	}
}

func Connect(connectStr string) (success bool) {
	var err error

	if connectStr == "" {
		ConnectDefaults()
		return
	}

	parts := strings.SplitN(connectStr, ":", 2)

	method := strings.ToLower(strings.TrimSpace(parts[0]))

	var uri string
	var targetcall, password, address string
	if len(parts) > 1 {
		uri = strings.TrimSpace(parts[1])
		targetcall, password, address, err = parseConnectURI(uri)
	}
	if err != nil {
		log.Println(err)
	}

	// QSY
	var revertFreq func()
	if address != "" {
		revertFreq, err = qsy(method, address)
		if err != nil {
			log.Printf("Unable to QSY: %s", err)
			return
		}
		defer revertFreq()
	}

	log.Printf("Connecting to %s...", connectStr)

	var conn net.Conn
	switch method {
	case MethodWinmor:
		done := handleInterrupt()
		conn, err = connectWinmor(targetcall)
		close(done)
	case MethodTelnet:
		if address == "" {
			conn, err = telnet.DialCMS(fMyCall)
			targetcall = telnet.CMSTargetCall
			break
		}
		conn, err = telnet.Dial(address, fMyCall, password)

	case MethodAX25:
		conn, err = ax25.DialAX25Timeout(
			config.AX25.Port,
			fMyCall,
			targetcall,
			45*time.Second,
		)

	case MethodSerialTNC:
		conn, err = ax25.DialKenwood(
			config.SerialTNC.Path,
			fMyCall,
			targetcall,
			ax25.NewConfig(ax25.Baudrate(config.SerialTNC.Baudrate)),
			nil,
		)

	default:
		log.Printf("'%s' is not a valid connect method", method)
		return
	}

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

	switch method {
	case MethodWinmor:
		log.Printf("QSY %s: %s", method, addr)
		var ok bool
		rig, ok := rigs[config.Winmor.Rig]
		if !ok {
			return noop, fmt.Errorf("Hamlib rig %s not loaded.", config.Winmor.Rig)
		}
		_, oldFreq, err := setFreq(rig, addr)
		if err != nil {
			return noop, err
		}
		time.Sleep(2 * time.Second)
		return func() {
			time.Sleep(time.Second)
			log.Printf("QSX %s: %.3f", method, float64(oldFreq)/1e3)
			rig.CurrentVFO().SetFreq(oldFreq)
		}, nil
	case MethodTelnet:
		return noop, nil
	default:
		return noop, fmt.Errorf("Not supported with method %s", method)
	}
}

func connectWinmor(target string) (net.Conn, error) {
	if wmTNC == nil {
		initWmTNC()
	}

	waitBusy(wmTNC)
	return wmTNC.Dial(target)
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
		if !printed && fIgnoreBusy {
			log.Println("Ignoring busy channel!")
			break
		} else if !printed {
			log.Println("Waiting for clear channel...")
			printed = true
		}
		time.Sleep(300 * time.Millisecond)
	}
}
