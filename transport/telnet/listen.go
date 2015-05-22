// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package telnet

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

type Conn struct {
	net.Conn
	remoteCall string
}

func (conn Conn) RemoteCall() string { return conn.remoteCall }

type listener struct{ net.Listener }

// Starts a new net.Listener listening for incoming connections.
//
// The Listener takes care of the special Winlink telnet login.
func Listen(addr string) (ln net.Listener, err error) {
	ln, err = net.Listen("tcp", addr)
	return listener{ln}, err
}

// Accept waits for and returns the next connection to the listener.
//
// The returned net.Conn is a *Conn that holds the remote stations
// call sign. When Accept returns, the caller is logged in and the
// connection can be used directly in a B2F exchange.
//
// BUG(martinhpedersen): Password is discarded and not supported yet.
func (ln listener) Accept() (net.Conn, error) {
	conn, err := ln.Listener.Accept()
	if err != nil {
		return conn, err
	}

	reader := bufio.NewReader(conn)

	fmt.Fprintf(conn, "Callsign :\r")
	remoteCall, err := reader.ReadString('\r')
	if err != nil {
		return conn, err
	}

	remoteCall = strings.TrimSpace(remoteCall)

	fmt.Fprintf(conn, "Password :\r")
	_, err = reader.ReadString('\r') //TODO

	return &Conn{conn, remoteCall}, err
}
