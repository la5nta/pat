// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package winmor

import (
	"net"
	"testing"

	"github.com/la5nta/wl2k-go/transport"
)

func TestImplementsRobust(t *testing.T) {
	var conn net.Conn
	conn = &tncConn{}

	if _, ok := conn.(transport.Robust); !ok {
		t.Errorf("winmor conn does not implement transport.Robust")
	}
}

func TestParseAddr(t *testing.T) {
	// Default address
	ctrlAddr, connAddr, err := parseAddr(DefaultAddr)
	if err != nil {
		t.Errorf("Failed to parse default address: %s", err)
	}
	if expected := "localhost:8500"; expected != ctrlAddr {
		t.Errorf("Expected %s as control address, got %s", expected, ctrlAddr)
	}
	if expected := "localhost:8501"; expected != connAddr {
		t.Errorf("Expected %s as connection address, got %s", expected, connAddr)
	}

	// IPv6 address
	ctrlAddr, connAddr, err = parseAddr("::1:8500")
	if err != nil {
		t.Errorf("Failed to parse default address: %s", err)
	}
	if expected := "::1:8500"; expected != ctrlAddr {
		t.Errorf("Expected %s as control address, got %s", expected, ctrlAddr)
	}
	if expected := "::1:8501"; expected != connAddr {
		t.Errorf("Expected %s as connection address, got %s", expected, connAddr)
	}

	// Some invalid addresses
	if _, _, err := parseAddr("localhost"); err == nil {
		t.Errorf("Expected error while parsing 'localhost', got %v", err)
	}
	if _, _, err := parseAddr(":8500"); err == nil {
		t.Errorf("Expected error while parsing ':8500', got %v", err)
	}
}
