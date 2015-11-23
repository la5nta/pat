// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// fbb provides a client-side implementation of the B2 Forwarding Protocol
// and Winlink 2000 Message Structure for transfer of messages to and from
// a Winlink 2000 Radio Email Server (RMS) gateway.
package fbb

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// Objects implementing the MBoxHandler interface can be used to handle inbound and outbound messages for a Session.
type MBoxHandler interface {
	InboundHandler
	OutboundHandler

	// Prepare is called before any other operation in a session.
	//
	// The returned error can be used to indicate that the mailbox is
	// not ready for a new session, the error will be forwarded to the
	// remote node.
	Prepare() error
}

// An OutboundHandler offer messages that can be delivered (a proposal) to the remote node and is notified when a message is sent or defered.
type OutboundHandler interface {
	// GetOutbound should return all pending (outbound) messages addressed to (and only to) one of the fw addresses.
	//
	// No fw address implies that the remote node could be a Winlink CMS and all oubound
	// messages can be delivered through the connected node.
	GetOutbound(fw ...Address) (out []*Message)

	// SetSent should mark the the message identified by MID as successfully sent.
	//
	// If rejected is true, it implies that the remote node has already received the message.
	SetSent(MID string, rejected bool)

	// SetDeferred should mark the outbound message identified by MID as deferred.
	//
	// SetDeferred is called when the remote want's to receive the proposed message
	// (see MID) later.
	SetDeferred(MID string)
}

// An InboundHandler handles all messages that can/is sent from the remote node.
type InboundHandler interface {
	// ProcessInbound should persist/save/process all messages received (msgs) returning an error if the operation was unsuccessful.
	//
	// The error will be delivered (if possble) to the remote to indicate that an error has occurred.
	ProcessInbound(msg ...*Message) error

	// GetInboundAnswer should return a ProposalAnwer (Accept/Reject/Defer) based on the remote's message Proposal p.
	//
	// An already successfully received message (see MID) should be rejected.
	GetInboundAnswer(p Proposal) ProposalAnswer
}

// Session represents a B2F exchange session.
//
// A session should only be used once.
type Session struct {
	mycall     string
	targetcall string
	locator    string
	motd       []string

	h             MBoxHandler
	statusUpdater StatusUpdater

	// Callback when secure login password is needed
	secureLoginHandleFunc func() (password string, err error)

	master bool

	remoteSID sid
	remoteFW  []Address // Addresses the remote requests messages on behalf of
	localFW   []Address // Addresses we request messages on behalf of

	trafficStats TrafficStats

	quitReceived bool
	quitSent     bool
	remoteNoMsgs bool // True if last remote turn had no more messages

	rd *bufio.Reader

	log  *log.Logger
	pLog *log.Logger
	ua   UserAgent
}

// Struct used to hold information that is reported during B2F handshake.
//
// Non of the fields must contain a dash (-).
//
type UserAgent struct {
	Name    string
	Version string
}

type StatusUpdater interface {
	UpdateStatus(s Status)
}

// Status holds information about ongoing transfers.
type Status struct {
	Receiving        *Proposal
	Sending          *Proposal
	BytesTransferred int
	BytesTotal       int
	When             time.Time
}

// TrafficStats holds exchange message traffic statistics.
type TrafficStats struct {
	Received []string // Received message MIDs.
	Sent     []string // Sent message MIDs.
}

var StdLogger = log.New(os.Stderr, "", log.LstdFlags)
var StdUA = UserAgent{Name: "wl2kgo", Version: "0.1a"}

// Constructs a new Session object.
//
// The Handler can be nil (but no messages will be exchanged).
func NewSession(mycall, targetcall, locator string, h MBoxHandler) *Session {
	return &Session{
		mycall:     mycall,
		localFW:    []Address{AddressFromString(mycall)},
		targetcall: targetcall,
		log:        StdLogger,
		h:          h,
		pLog:       StdLogger,
		ua:         StdUA,
		locator:    locator,
		trafficStats: TrafficStats{
			Received: make([]string, 0),
			Sent:     make([]string, 0),
		},
	}
}

// SetMOTD sets one or more lines to be sent before handshake.
//
// The MOTD is only sent if the local node is session master.
func (s *Session) SetMOTD(line ...string) { s.motd = line }

// IsMaster sets whether this end should initiate the handshake.
func (s *Session) IsMaster(isMaster bool) { s.master = isMaster }

// RemoteSID returns the remote's SID (if available).
func (s *Session) RemoteSID() string { return string(s.remoteSID) }

// Exchange is the main method for exchanging messages with a remote over the B2F protocol.
//
// Sends outbound messages and downloads inbound messages prepared for this session.
//
// Outbound messages should be added as proposals before calling the Exchange() method.
//
// After Exchange(), messages that was accepted and delivered successfully to the RMS is
// available through a call to Sent(). Messages downloaded successfully from the RMS is
// retrieved by calling Received().
//
// The connection is closed at the end of the exchange. If the connection is closed before
// the exchange is done, is will return io.EOF.
//
// Subsequent Exchange calls on the same session is a noop.
func (s *Session) Exchange(conn net.Conn) (stats TrafficStats, err error) {
	if s.Done() {
		return stats, nil
	}

	// The given conn should always be closed after returning from this method.
	// If an error occured, echo it to the remote.
	defer func() {
		if err == nil {
			return
		}

		// In case another go-routine closes the connection...
		localEOF := strings.Contains(err.Error(), "use of closed network connection")
		if localEOF {
			err = io.EOF
		}

		if err != io.EOF {
			conn.SetDeadline(time.Now().Add(time.Minute))
			fmt.Fprintf(conn, "*** %s\r\n", err)
			conn.Close()
		}
	}()

	// Prepare mailbox handler
	if s.h != nil {
		err = s.h.Prepare()
		if err != nil {
			return
		}
	}

	s.rd = bufio.NewReader(conn)

	err = s.handshake(conn)
	if err != nil {
		return
	}

	if gzipExperimentEnabled() && s.remoteSID.Has(sGzip) {
		s.log.Println("GZIP_EXPERIMENT:", "Gzip compression enabled in this session.")
	}

	for myTurn := !s.master; !s.Done(); myTurn = !myTurn {
		if myTurn {
			s.quitSent, err = s.handleOutbound(conn)
		} else {
			s.quitReceived, err = s.handleInbound(conn)
		}

		if err != nil {
			return s.trafficStats, err
		}
	}

	return s.trafficStats, conn.Close()
}

// Done() returns true if either parties have existed from this session.
func (s *Session) Done() bool { return s.quitReceived || s.quitSent }

// Waits for connection to be closed, returning an error if seen on the line.
func waitRemoteHangup(conn net.Conn) error {
	conn.SetDeadline(time.Now().Add(time.Minute))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()

		if err := errLine(line); err != nil {
			conn.Close()
			return err
		}
		log.Println(line)
	}
	return scanner.Err()
}

func remoteErr(str string) error {
	if !strings.HasPrefix(str, "***") {
		return nil
	}

	idx := strings.LastIndex(str, "*")
	if idx+1 >= len(str) {
		return nil
	}

	return fmt.Errorf(strings.TrimSpace(str[idx+1:]))
}

// Mycall returns this stations call sign.
func (s *Session) Mycall() string { return s.mycall }

// Targetcall returns the remote stations call sign (if known).
func (s *Session) Targetcall() string { return s.targetcall }

// SetSecureLoginHandleFunc registers a callback function used to prompt for password when a secure login challenge is received.
func (s *Session) SetSecureLoginHandleFunc(f func() (password string, err error)) {
	s.secureLoginHandleFunc = f
}

// This method returns the call signs the remote is requesting traffic on behalf of. The call signs are not available until
// the handshake is done.
//
// It will typically be the call sign of the remote P2P station and empty when the remote is a Winlink CMS.
func (s *Session) RemoteForwarders() []Address { return s.remoteFW }

// AddAuxiliaryAddress adds one or more addresses to request messages on behalf of.
//
// Currently the Winlink System only support requesting messages for call signs, not full email addresses.
func (s *Session) AddAuxiliaryAddress(aux ...Address) { s.localFW = append(s.localFW, aux...) }

// Set callback for status updates on receiving / sending messages
func (s *Session) SetStatusUpdater(updater StatusUpdater) { s.statusUpdater = updater }

// Sets custom logger.
func (s *Session) SetLogger(logger *log.Logger) {
	if logger == nil {
		logger = StdLogger
	}
	s.log = logger
	s.pLog = logger

}

// Set this session's user agent
func (s *Session) SetUserAgent(ua UserAgent) { s.ua = ua }

// Get this session's user agent
func (s *Session) UserAgent() UserAgent { return s.ua }

func (s *Session) outbound() []*Proposal {
	if s.h == nil {
		return []*Proposal{}
	}

	msgs := s.h.GetOutbound(s.remoteFW...)
	props := make([]*Proposal, 0, len(msgs))

	for _, m := range msgs {
		prop, err := m.Proposal(s.highestPropCode())
		if err != nil {
			// TODO: This should result in an error somewhere
			s.log.Printf("Unable to prepare proposal for '%s'. Corrupt message? Skipping...", prop.MID())
			continue
		}

		props = append(props, prop)
	}
	return props
}

func (s *Session) highestPropCode() PropCode {
	if s.remoteSID.Has(sGzip) && gzipExperimentEnabled() {
		return GzipProposal
	}
	return Wl2kProposal
}
