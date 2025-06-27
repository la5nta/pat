// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/api/types"
	"github.com/la5nta/pat/internal/buildinfo"

	"github.com/la5nta/wl2k-go/fbb"
)

type ex struct {
	conn   net.Conn
	target string
	master bool
	errors chan error
}

func (a *App) exchangeLoop(ctx context.Context) chan ex {
	ce := make(chan ex)
	go func() {
		for {
			select {
			case ex := <-ce:
				ex.errors <- a.sessionExchange(ex.conn, ex.target, ex.master)
				close(ex.errors)
			case <-ctx.Done():
				return
			}
		}
	}()
	return ce
}

func (a *App) exchange(conn net.Conn, targetCall string, master bool) error {
	e := ex{
		conn:   conn,
		target: targetCall,
		master: master,
		errors: make(chan error),
	}
	a.exchangeChan <- e
	return <-e.errors
}

type NotifyMBox struct {
	fbb.MBoxHandler
	*App
}

func (m NotifyMBox) ProcessInbound(msgs ...*fbb.Message) error {
	if err := m.MBoxHandler.ProcessInbound(msgs...); err != nil {
		return err
	}
	for _, msg := range msgs {
		m.websocketHub.WriteNotification(types.Notification{
			Title: fmt.Sprintf("New message from %s", msg.From().Addr),
			Body:  msg.Subject(),
		})
		if isSystemMessage(msg) {
			m.onSystemMessageReceived(msg)
		}
	}
	return nil
}

func (m NotifyMBox) GetInboundAnswers(p []fbb.Proposal) []fbb.ProposalAnswer {
	answers := make([]fbb.ProposalAnswer, len(p))
	var outsideLimit bool
	for idx, p := range p {
		answers[idx] = m.GetInboundAnswer(p)
		outsideLimit = outsideLimit || p.CompressedSize() >= m.config.AutoDownloadSizeLimit
	}
	if !outsideLimit || m.config.AutoDownloadSizeLimit < 0 {
		// All proposals are within the prompt limit. Go ahead.
		return answers
	}

	// Build multi-select build options for those accepted by the mailbox handler.
	var options []PromptOption
	for idx, p := range p {
		if answers[idx] != fbb.Accept {
			continue
		}
		answers[idx] = fbb.Defer // Defer unless user explicitly accepts through prompt answer.
		sender, subject := "Unkown sender", "Unknown subject"
		if pm := p.PendingMessage(); pm != nil {
			sender, subject = pm.From.String(), pm.Subject
		}
		desc := fmt.Sprintf("%s (%d bytes): %s", sender, p.CompressedSize(), subject)
		options = append(options, PromptOption{Value: p.MID(), Desc: desc, Checked: p.CompressedSize() < m.config.AutoDownloadSizeLimit})
	}

	// Prompt the user
	ans := <-m.promptHub.Prompt(context.Background(), time.Minute, PromptKindMultiSelect, "Select messages for download", options...)

	// If timeout was reached, use our default values to fill in for the user
	if ans.Err == context.DeadlineExceeded {
		var checked []string
		for _, opt := range options {
			if opt.Checked {
				checked = append(checked, opt.Value)
			}
		}
		ans.Value = strings.Join(checked, ",")
	}

	// For each mid in answer, search the proposals and update answer to Accept.
	for _, val := range strings.Split(ans.Value, ",") {
		for idx, p := range p {
			if p.MID() != val {
				continue
			}
			answers[idx] = fbb.Accept
		}
	}

	return answers
}

func (a *App) sessionExchange(conn net.Conn, targetCall string, master bool) error {
	a.exchangeConn = conn
	a.websocketHub.UpdateStatus()
	defer func() { a.exchangeConn = nil; a.websocketHub.UpdateStatus() }()

	// New wl2k Session
	targetCall = strings.Split(targetCall, ` `)[0]
	session := fbb.NewSession(
		a.options.MyCall,
		targetCall,
		a.config.Locator,
		NotifyMBox{a.mbox, a},
	)

	session.SetUserAgent(fbb.UserAgent{
		Name:    buildinfo.AppName,
		Version: buildinfo.Version,
	})

	if len(a.config.MOTD) > 0 {
		session.SetMOTD(a.config.MOTD...)
	}

	// Handle secure login
	session.SetSecureLoginHandleFunc(func(addr fbb.Address) (string, error) {
		if addr.Addr == a.options.MyCall && a.config.SecureLoginPassword != "" {
			return a.config.SecureLoginPassword, nil
		}
		for _, aux := range a.config.AuxAddrs {
			if !addr.EqualString(aux.Address) {
				continue
			}
			switch {
			case aux.Password != nil:
				return *aux.Password, nil
			case a.config.SecureLoginPassword != "":
				return a.config.SecureLoginPassword, nil
			}
		}
		resp := <-a.promptHub.Prompt(context.Background(), time.Minute, PromptKindPassword, "Enter secure login password for "+addr.String())
		return resp.Value, resp.Err
	})

	for _, addr := range a.config.AuxAddrs {
		session.AddAuxiliaryAddress(fbb.AddressFromString(addr.Address))
	}

	session.IsMaster(master)
	session.SetLogger(log.New(a.logWriter, "", 0))

	session.SetStatusUpdater(StatusUpdate{a.websocketHub})

	if a.options.Robust {
		session.SetRobustMode(fbb.RobustForced)
	}

	log.Printf("Connected to %s (%s)", conn.RemoteAddr(), conn.RemoteAddr().Network())

	start := time.Now()

	stats, err := session.Exchange(conn)
	if fbb.IsLoginFailure(err) {
		fmt.Println("NOTE: A new password scheme for Winlink is being implemented as of 2018-01-31.")
		fmt.Println("      Users with passwords created/changed prior to January 31, 2018 should be")
		fmt.Println("      aware that their password MUST be entered in ALL-UPPERCASE letters. Only")
		fmt.Println("      passwords created/changed/issued after January 31, 2018 should/may contain")
		fmt.Println("      lowercase letters. - https://github.com/la5nta/pat/issues/113")
	}

	if t, _ := strconv.ParseBool(os.Getenv("PAT_MOCK_NEW_ACCOUNT_MSG")); t {
		log.Println("Mocking new account msg...")
		NotifyMBox{a.mbox, a}.ProcessInbound(mockNewAccountMsg())
	}

	event := map[string]interface{}{
		"mycall":              session.Mycall(),
		"targetcall":          session.Targetcall(),
		"remote_fw":           session.RemoteForwarders(),
		"remote_sid":          session.RemoteSID(),
		"master":              master,
		"local_locator":       a.config.Locator,
		"auxiliary_addresses": a.config.AuxAddrs,
		"network":             conn.RemoteAddr().Network(),
		"remote_addr":         conn.RemoteAddr().String(),
		"local_addr":          conn.LocalAddr().String(),
		"sent":                stats.Sent,
		"received":            stats.Received,
		"start":               start.Unix(),
		"end":                 time.Now().Unix(),
		"success":             err == nil,
	}
	if err != nil {
		event["error"] = err.Error()
	}

	a.eventLog.Log("exchange", event)

	return err
}

func (a *App) AbortActiveConnection(dirty bool) (ok bool) {
	switch {
	case dirty:
		// This mean we've already tried to abort, but the connection is still active.
		// Fallback to the below cases to try to identify the busy modem and abort hard.
	case a.dialing != nil:
		// If we're currently dialing a transport, attempt to abort by cancelling the associated context.
		log.Printf("Got abort signal while dialing %s, cancelling...", a.dialing.Scheme)
		go a.dialCancelFunc()
		return true
	case a.exchangeConn != nil:
		// If we have an active connection, close it gracefully.
		log.Println("Got abort signal, disconnecting...")
		go a.exchangeConn.Close()
		return true
	}

	// Any connection and/or dial operation has been cancelled at this point.
	// User is attempting to abort something, so try to identify any non-idling transports and abort.
	// It might be a "dirty disconnect" of an already cancelled connection or dial operation which is in the
	// process of gracefully terminating. It might also be an attempt to close an inbound P2P connection.
	switch {
	case a.ardop != nil && !a.ardop.Idle():
		if dirty {
			log.Println("Dirty disconnecting ardop...")
			a.ardop.Abort()
			return true
		}
		log.Println("Disconnecting ardop...")
		go func() {
			if err := a.ardop.Disconnect(); err != nil {
				log.Println(err)
			}
		}()
		return true
	case a.varaFM != nil && !a.varaFM.Idle():
		if dirty {
			log.Println("Dirty disconnecting varafm...")
			a.varaFM.Abort()
			return true
		}
		log.Println("Disconnecting varafm...")
		go func() {
			if err := a.varaFM.Close(); err != nil {
				log.Println(err)
			}
		}()
		return true
	case a.varaHF != nil && !a.varaHF.Idle():
		if dirty {
			log.Println("Dirty disconnecting varahf...")
			a.varaHF.Abort()
			return true
		}
		log.Println("Disconnecting varahf...")
		go func() {
			if err := a.varaHF.Close(); err != nil {
				log.Println(err)
			}
		}()
		return true
	case a.pactor != nil:
		log.Println("Disconnecting pactor...")
		err := a.pactor.Close()
		if err != nil {
			log.Println(err)
		}
		return err == nil
	default:
		return false
	}
}

type StatusUpdate struct{ WSHub }

func (s StatusUpdate) UpdateStatus(stat fbb.Status) {
	var prop fbb.Proposal
	switch {
	case stat.Receiving != nil:
		prop = *stat.Receiving
	case stat.Sending != nil:
		prop = *stat.Sending
	}

	s.WriteProgress(types.Progress{
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
