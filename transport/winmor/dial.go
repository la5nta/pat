// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package winmor

import (
	"errors"
	"fmt"
	"net"
	"time"
)

type tncConn struct {
	net.Conn
	ctrlOut chan<- string
	ctrlIn  broadcaster

	remoteAddr Addr
	localAddr  Addr
}

func (tnc *TNC) Dial(targetcall string) (net.Conn, error) {
	if err := tnc.connect(targetcall); err != nil {
		return nil, err
	}

	time.Sleep(200 * time.Millisecond) // To give WINMOR time to listen
	dataConn, err := net.Dial("tcp", tnc.connAddr)
	if err != nil {
		return nil, err
	}

	mycall, err := tnc.MyCall()
	if err != nil {
		return nil, fmt.Errorf("Error when getting mycall: %s", err)
	}

	tnc.data = dataConn

	if err := tnc.data.(*net.TCPConn).SetReadBuffer(0); err != nil {
		return nil, err
	}
	if err := tnc.data.(*net.TCPConn).SetWriteBuffer(0); err != nil {
		return nil, err
	}

	return &tncConn{
		Conn:       dataConn,
		remoteAddr: Addr{targetcall},
		localAddr:  Addr{mycall},
		ctrlOut:    tnc.out,
		ctrlIn:     tnc.in,
	}, nil
}

func (conn *tncConn) Close() error {
	if conn.Conn == nil {
		return nil
	}

	r := conn.ctrlIn.Listen()
	defer r.Close()

	conn.ctrlOut <- "DISCONNECT"
	for msg := range r.Msgs() { // Wait for TNC to disconnect
		if msg.cmd == cmdDisconnect {
			// The command echo
		} else if msg.cmd == cmdNewState && msg.State() == Disconnected {
			// The control loop have already closed the data connection
			return nil
			//return conn.Conn.Close()
		}
	}
	return errors.New("TNC hung up while waiting for requested disconnect")
}

func (conn *tncConn) RemoteAddr() net.Addr {
	return conn.remoteAddr
}

func (conn *tncConn) LocalAddr() net.Addr {
	return conn.localAddr
}
