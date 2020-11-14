// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
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

	"github.com/la5nta/wl2k-go/fbb"
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

type NotifyMBox struct{ fbb.MBoxHandler }

func (m NotifyMBox) ProcessInbound(msgs ...*fbb.Message) error {
	if err := m.MBoxHandler.ProcessInbound(msgs...); err != nil {
		return err
	}
	for _, msg := range msgs {
		websocketHub.WriteJSON(struct{ Notification Notification }{
			Notification{
				Title: fmt.Sprintf("New message from %s", msg.From().Addr),
				Body:  msg.Subject(),
			},
		})
	}
	return nil
}

func sessionExchange(conn net.Conn, targetCall string, master bool) error {
	exchangeConn = conn
	websocketHub.UpdateStatus()
	defer func() { exchangeConn = nil; websocketHub.UpdateStatus() }()

	// New wl2k Session
	targetCall = strings.Split(targetCall, ` `)[0]
	session := fbb.NewSession(
		fOptions.MyCall,
		targetCall,
		config.Locator,
		NotifyMBox{mbox},
	)

	session.SetUserAgent(fbb.UserAgent{
		Name:    AppName,
		Version: Version,
	})

	if len(config.MOTD) > 0 {
		session.SetMOTD(config.MOTD...)
	}

	// Handle secure login
	session.SetSecureLoginHandleFunc(func(addr fbb.Address) (string, error) {
		if addr.Addr == fOptions.MyCall && config.SecureLoginPassword != "" {
			return config.SecureLoginPassword, nil
		}
		for _, aux := range config.AuxAddrs {
			if addr.Addr != aux.Address {
				continue
			}
			switch {
			case aux.Password != nil:
				return *aux.Password, nil
			case config.SecureLoginPassword != "":
				return config.SecureLoginPassword, nil
			}
		}
		resp := <-promptHub.Prompt("password", "Enter secure login password for "+addr.String())
		return resp.Value, resp.Err
	})

	for _, addr := range config.AuxAddrs {
		session.AddAuxiliaryAddress(fbb.AddressFromString(addr.Address))
	}

	session.IsMaster(master)
	session.SetLogger(log.New(logWriter, "", 0))

	session.SetStatusUpdater(new(StatusUpdate))

	if fOptions.Robust {
		session.SetRobustMode(fbb.RobustForced)
	}

	log.Printf("Connected to %s (%s)", conn.RemoteAddr(), conn.RemoteAddr().Network())

	// Close connection on os.Interrupt
	stop := handleInterrupt()
	defer close(stop)

	startTs := time.Now()

	stats, err := session.Exchange(conn)
	if fbb.IsLoginFailure(err) {
		fmt.Println("NOTE: A new password scheme for Winlink is being implemented as of 2018-01-31.")
		fmt.Println("      Users with passwords created/changed prior to January 31, 2018 should be")
		fmt.Println("      aware that their password MUST be entered in ALL-UPPERCASE letters. Only")
		fmt.Println("      passwords created/changed/issued after January 31, 2018 should/may contain")
		fmt.Println("      lowercase letters. - https://github.com/la5nta/pat/issues/113")
	}

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

		dirtyDisconnectNext := false // So we can do a dirty disconnect on the second interrupt
		for {
			select {
			case <-stop:
				return
			case <-sig:
				abortActiveConnection(dirtyDisconnectNext)
				dirtyDisconnectNext = !dirtyDisconnectNext
			}
		}
	}()

	return stop
}

func abortActiveConnection(dirty bool) (ok bool) {
	switch {
	case exchangeConn != nil:
		log.Println("Got abort signal, disconnecting...")
		exchangeConn.Close()
		return true
	case pModem != nil:
		log.Println("Disconnecting pactor...")
		err := pModem.Close()
		if err != nil {
			log.Println(err)
		}
		return err == nil
	case wmTNC != nil && !wmTNC.Idle():
		if dirty {
			log.Println("Dirty disconnecting winmor...")
			wmTNC.DirtyDisconnect()
			return true
		}
		log.Println("Disconnecting winmor...")
		go func() {
			if err := wmTNC.Disconnect(); err != nil {
				log.Println(err)
			}
		}()
		return true
	case adTNC != nil && !adTNC.Idle():
		if dirty {
			log.Println("Dirty disconnecting ardop...")
			adTNC.Abort()
			return true
		}
		log.Println("Disconnecting ardop...")
		go func() {
			if err := adTNC.Disconnect(); err != nil {
				log.Println(err)
			}
		}()
		return true
	case dialing != nil:
		log.Printf("Transport %s's dialer can not be aborted at this stage", dialing.Scheme)
		return false
	default:
		return false
	}
}

type StatusUpdate int

func (s *StatusUpdate) UpdateStatus(stat fbb.Status) {
	var prop fbb.Proposal
	switch {
	case stat.Receiving != nil:
		prop = *stat.Receiving
	case stat.Sending != nil:
		prop = *stat.Sending
	}

	websocketHub.WriteProgress(Progress{
		MID:              prop.MID(),
		BytesTotal:       stat.BytesTotal,
		BytesTransferred: stat.BytesTransferred,
		Subject:          prop.Title(),
		Receiving:        stat.Receiving != nil,
		Sending:          stat.Sending != nil,
		Done:             stat.Done,
	})

	percent := float64(stat.BytesTransferred) / float64(stat.BytesTotal) * 100
	fmt.Printf("\r%s: %3.0f%%", prop.Title(), percent)

	if stat.Done {
		fmt.Println("")
	}
	os.Stdout.Sync()
}
