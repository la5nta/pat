// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/howeyc/gopass"

	"github.com/la5nta/wl2k-go"
)

type ex struct {
	conn   net.Conn
	target string
	master bool
	errors chan error
}

func exchangeLoop() (ce chan ex) {
	ce = make(chan ex)
	go func() {
		for ex := range ce {
			ex.errors <- sessionExchange(ex.conn, ex.target, ex.master)
			close(ex.errors)
		}
	}()
	return ce
}

func exchange(conn net.Conn, targetCall string, master bool) error {
	e := ex{
		conn:   conn,
		target: targetCall,
		master: master,
		errors: make(chan error),
	}
	exchangeChan <- e
	return <-e.errors
}

func sessionExchange(conn net.Conn, targetCall string, master bool) error {
	exchangeConn = conn
	defer func() { exchangeConn = nil }()

	// New wl2k Session
	targetCall = strings.Split(targetCall, ` `)[0]
	session := wl2k.NewSession(
		fOptions.MyCall,
		targetCall,
		config.Locator,
		mbox,
	)

	if len(config.MOTD) > 0 {
		session.SetMOTD(config.MOTD...)
	}

	// Handle secure login
	session.SetSecureLoginHandleFunc(func() (string, error) {
		if config.SecureLoginPassword != "" {
			return config.SecureLoginPassword, nil
		}

		fmt.Print("Enter secure login password: ")
		return string(gopass.GetPasswdMasked()), nil
	})

	for _, addr := range config.AuxAddrs {
		session.AddAuxiliaryAddress(wl2k.AddressFromString(addr))
	}

	session.IsMaster(master)
	session.SetStatusUpdater(new(StatusUpdate))
	session.SetLogger(log.New(logWriter, "", 0))

	log.Printf("Connected to %s:%s", conn.RemoteAddr().Network(), conn.RemoteAddr())

	// Close connection on os.Interrupt
	stop := handleInterrupt()
	defer close(stop)

	startTs := time.Now()

	stats, err := session.Exchange(conn)

	event := map[string]interface{}{
		"mycall":              session.Mycall(),
		"targetcall":          session.Targetcall(),
		"remote_fw":           session.RemoteForwarders(),
		"remote_sid":          session.RemoteSID(),
		"master":              master,
		"local_locator":       config.Locator,
		"auxiliary_addresses": config.AuxAddrs,
		"network":             conn.RemoteAddr().Network(),
		"remote_addr":         conn.RemoteAddr().String(),
		"local_addr":          conn.LocalAddr().String(),
		"sent":                stats.Sent,
		"received":            stats.Received,
		"start":               startTs.Unix(),
		"end":                 time.Now().Unix(),
		"success":             err == nil,
	}
	if err != nil {
		event["error"] = err.Error()
	}

	eventLog.Log("exchange", event)

	return err
}

func handleInterrupt() (stop chan struct{}) {
	stop = make(chan struct{})

	go func() {
		sig := make(chan os.Signal)
		signal.Notify(sig, os.Interrupt)
		defer func() { signal.Stop(sig); close(sig) }()

		wmDisc := false // So we can DirtyDisconnect on second interrupt
		for {
			select {
			case <-stop:
				return
			case s := <-sig:
				if exchangeConn != nil {
					log.Printf("Got %s, disconnecting...", s)
					exchangeConn.Close()
					break
				}
				if wmTNC != nil && !wmTNC.Idle() {
					if wmDisc {
						log.Println("Dirty disconnecting winmor...")
						wmTNC.DirtyDisconnect()
						wmDisc = false
					} else {
						log.Println("Disconnecting winmor...")
						wmDisc = true
						go func() {
							if err := wmTNC.Disconnect(); err != nil {
								log.Println(err)
							} else {
								wmDisc = false
							}
						}()
					}
				}
				if adTNC != nil && !adTNC.Idle() {
					if adDisc {
						log.Println("Dirty disconnecting ardop...")
						adTNC.DirtyDisconnect()
						adDisc = false
					} else {
						log.Println("Disconnecting ardop...")
						adDisc = true
						go func() {
							if err := adTNC.Disconnect(); err != nil {
								log.Println(err)
							} else {
								adDisc = false
							}
						}()
					}
				}
			}
		}
	}()

	return stop
}

type StatusUpdate int

func (s *StatusUpdate) UpdateStatus(stat wl2k.Status) {
	var prop *wl2k.Proposal
	if stat.Receiving != nil {
		prop = stat.Receiving
	} else {
		prop = stat.Sending
	}
	percent := float64(stat.BytesTransferred) / float64(stat.BytesTotal) * 100
	fmt.Printf("\r%s: %3.0f%%", prop.Title(), percent)
	if int(percent) == 100 {
		fmt.Println("")
	}
	os.Stdout.Sync()
}
