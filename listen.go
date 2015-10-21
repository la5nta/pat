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

	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/telnet"
)

type incomingConnect struct {
	conn       net.Conn
	remoteCall string
	kind       string
	freq       Frequency
}

func Unlisten(param string) {
	methods := strings.Split(param, ",")
	for _, method := range methods {
		ln, ok := listeners[method]
		if !ok {
			fmt.Printf("No active %s listener, ignoring.\n", method)
		} else if err := ln.Close(); err != nil {
			log.Printf("Unable to close %s listener: %s", method, err)
		}
	}
}

func Listen(listenStr string) {
	cc := make(chan incomingConnect, 2)

	methods := strings.Split(listenStr, ",")
	for _, method := range methods {
		method = strings.TrimSpace(strings.ToLower(method))

		switch method {
		case MethodWinmor:
			if wmTNC == nil {
				initWinmorTNC()
			}
			listenWinmor(cc)
		case MethodArdop:
			if adTNC == nil {
				initArdopTNC()
			}
			listenArdop(cc)
		case MethodTelnet:
			listenTelnet(cc)
		case MethodAX25:
			listenAX25(cc)
		case MethodSerialTNC:
			log.Printf("%s listen not implemented, ignoring.", method)
		default:
			log.Printf("'%s' is not a valid listen method", method)
			return
		}
	}

	log.Printf("Listening for incoming traffic (%s)...", listenStr)
	go func() {
		for {
			connect := <-cc
			eventLog.LogConn("accept", connect.freq, connect.conn, nil)
			log.Printf("Got connect (%s:%s)", connect.kind, connect.remoteCall)

			err := exchange(connect.conn, connect.remoteCall, true)
			if err != nil {
				log.Printf("Exchange failed: %s", err)
			} else {
				log.Println("Disconnected.")
			}
		}
	}()
}

func listenWinmor(incoming chan<- incomingConnect) {
	// RMS Express runs bw at 500Hz except when sending/receiving message. Why?
	// ... Or is it cmdRobust True?
	ln, err := wmTNC.Listen(config.Winmor.InboundBandwidth)
	if err != nil {
		log.Fatal(err)
	}

	listeners[MethodWinmor] = ln
	go func() {
		defer func() {
			delete(listeners, MethodWinmor)
			log.Printf("%s listener closed.", MethodWinmor)
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			var freq Frequency
			if rig, ok := rigs[config.Winmor.Rig]; ok {
				f, _ := rig.CurrentVFO().GetFreq()
				freq = Frequency(f)
			}

			incoming <- incomingConnect{
				conn:       conn,
				remoteCall: conn.RemoteAddr().String(),
				kind:       MethodWinmor,
				freq:       freq,
			}
		}
	}()
}

func listenArdop(incoming chan<- incomingConnect) {
	ln, err := adTNC.Listen()
	if err != nil {
		log.Fatal(err)
	}

	listeners[MethodArdop] = ln
	go func() {
		defer func() {
			delete(listeners, MethodArdop)
			log.Printf("%s listener closed.", MethodArdop)
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			var freq Frequency
			if rig, ok := rigs[config.Ardop.Rig]; ok {
				f, _ := rig.CurrentVFO().GetFreq()
				freq = Frequency(f)
			}

			incoming <- incomingConnect{
				conn:       conn,
				remoteCall: conn.RemoteAddr().String(),
				kind:       MethodArdop,
				freq:       freq,
			}
		}
	}()
}

func listenAX25(incoming chan<- incomingConnect) {
	if config.AX25.Beacon.Every > 0 {
		b, err := ax25.NewAX25Beacon(config.AX25.Port, fOptions.MyCall, config.AX25.Beacon.Destination, config.AX25.Beacon.Message)
		if err != nil {
			log.Printf("Unable to activate beacon: %s", err)
		} else {
			go b.Every(time.Duration(config.AX25.Beacon.Every) * time.Second)
		}
	}

	ln, err := ax25.ListenAX25(config.AX25.Port, fOptions.MyCall)
	if err != nil {
		log.Printf("Unable to start AX.25 listener: %s", err)
		return
	}

	listeners[MethodAX25] = ln
	go func() {
		defer func() {
			delete(listeners, MethodAX25)
			log.Printf("%s listener closed.", MethodAX25)
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			incoming <- incomingConnect{
				conn:       conn,
				remoteCall: conn.RemoteAddr().String(),
				kind:       MethodAX25,
			}
		}
	}()
}

func listenTelnet(incoming chan<- incomingConnect) {
	ln, err := telnet.Listen(config.Telnet.ListenAddr)
	if err != nil {
		log.Fatal(err)
	}

	listeners[MethodTelnet] = ln
	go func() {
		defer func() {
			delete(listeners, MethodTelnet)
			log.Printf("%s listener closed.", MethodTelnet)
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			incoming <- incomingConnect{
				conn:       conn,
				remoteCall: conn.(*telnet.Conn).RemoteCall(),
				kind:       MethodTelnet,
			}
		}
	}()
}
