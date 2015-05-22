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
)

const (
	CMSTargetCall = "wl2k"
	CMSPassword   = "CMSTelnet"
	CMSAddress    = "server.winlink.org:8772"
)

func DialCMS(mycall string) (net.Conn, error) {
	return Dial(CMSAddress, mycall, CMSPassword)
}

func Dial(addr, mycall, password string) (net.Conn, error) {
	conn, err := net.Dial(`tcp`, addr)
	if err != nil {
		return conn, err
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

	return conn, nil // We could return a proper telnet.Conn here
}
