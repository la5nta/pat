// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package telnet provides a method of connecting to Winlink CMS over tcp ("telnet-mode")
package telnet

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	CMSTargetCall = "wl2k"
	CMSPassword   = "CMSTelnet"
	CMSAddress    = "server.winlink.org:8772"
)

// DialCMS dials a random CMS server through server.winlink.org.
//
// The function will retry 4 times before giving up and returning an error.
func DialCMS(mycall string) (net.Conn, error) {
	var conn net.Conn
	var err error

	// Dial with retry, in case we hit an unavailable CMS.
	for i := 0; i < 4; i++ {
		conn, err = Dial(CMSAddress, mycall, CMSPassword)
		if err == nil {
			break
		}
	}

	return conn, err
}

func Dial(addr, mycall, password string) (net.Conn, error) {
	conn, err := net.DialTimeout(`tcp`, addr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	// Log in to telnet server
	reader := bufio.NewReader(conn)
L:
	for {
		line, err := reader.ReadString('\r')
		line = strings.TrimSpace(strings.ToLower(line))
		switch {
		case err != nil:
			conn.Close()
			return nil, fmt.Errorf("Error while logging in: %s", err)
		case strings.HasPrefix(line, "callsign"):
			fmt.Fprintf(conn, "%s\r", mycall)
		case strings.HasPrefix(line, "password"):
			fmt.Fprintf(conn, "%s\r", password)
			break L
		}
	}

	return &Conn{conn, CMSTargetCall}, nil
}
