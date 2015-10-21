// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

//[WL2K-2.8.4.8-B2FWIHJM$]
//Brentwood CMS >
//	;FW: LA5NTA
//	[RMS Express-1.2.35.0-B2FHM$]
//	; WL2K DE LA5NTA (JO39EQ)
//	FF
//FQ

func TestSessionP2P(t *testing.T) {
	client, master := net.Pipe()

	clientErr := make(chan error)
	go func() {
		s := NewSession("LA5NTA", "N0CALL", "JO39EQ", nil)
		_, err := s.Exchange(client)
		clientErr <- err
	}()

	masterErr := make(chan error)
	go func() {
		s := NewSession("N0CALL", "LA5NTA", "JO39EQ", nil)
		s.IsMaster(true)
		_, err := s.Exchange(master)
		masterErr <- err
	}()

	if err := <-masterErr; err != nil {
		t.Errorf("Master returned with error: %s", err)
	}
	if err := <-clientErr; err != nil {
		t.Errorf("Client returned with error: %s", err)
	}
}

func TestSessionCMS(t *testing.T) {
	client, srv := net.Pipe()

	cerrs := make(chan error)
	go func() {
		s := NewSession("LA5NTA", "LA1B-10", "JO39EQ", nil)
		_, err := s.Exchange(client)
		cerrs <- err
	}()

	fmt.Fprint(srv, "[WL2K-2.8.4.8-B2FWIHJM$]\r")
	fmt.Fprint(srv, "Test CMS >\r")

	expectLines := []string{
		";FW: LA5NTA\r",
		"[wl2kgo-0.1a-B2FHM$]\r",
		"; LA1B-10 DE LA5NTA (JO39EQ)\r",
		"FF\r",
	}

	// Read until FF
	rd := bufio.NewReader(srv)
	for i, expected := range expectLines {
		line, _ := rd.ReadString('\r')
		if line != expected {
			line, expected = strings.TrimSpace(line), strings.TrimSpace(expected)
			t.Fatalf("Unexpected line [%d]: Got '%s', expected '%s'.", i, line, expected)
		}
	}

	fmt.Fprint(srv, "FQ\r")
	srv.Close()

	if err := <-cerrs; err != nil {
		t.Errorf("Session exchange returned error: %s", err)
	}
}

func TestSessionCMDWithMessage(t *testing.T) {
	client, srv := net.Pipe()

	cerrs := make(chan error)
	go func() {
		s := NewSession("LA5NTA", "LA1B-10", "JO39EQ", nil)
		_, err := s.Exchange(client)
		cerrs <- err
	}()

	fmt.Fprint(srv, "[WL2K-2.8.4.8-B2FWIHJM$]\r")
	fmt.Fprint(srv, "Test CMS >\r")

	expectLines := []string{
		";FW: LA5NTA\r",
		"[wl2kgo-0.1a-B2FHM$]\r",
		"; LA1B-10 DE LA5NTA (JO39EQ)\r",
		"FF\r",
	}

	// Read until FF
	rd := bufio.NewReader(srv)
	for i, expected := range expectLines {
		line, _ := rd.ReadString('\r')
		if line != expected {
			line, expected = strings.TrimSpace(line), strings.TrimSpace(expected)
			t.Fatalf("Unexpected line [%d]: Got '%s', expected '%s'.", i, line, expected)
		}
	}

	// Send one proposal
	fmt.Fprintf(srv, "FC EM TJKYEIMMHSRB 527 123 0\r")
	fmt.Fprintf(srv, "F> 3b\r") // No more proposals + checksum

	propAnswer, _ := rd.ReadString('\r')
	if propAnswer != "FS =\r" {
		t.Errorf("Expected 'FS =', got '%s'", propAnswer)
	}
	fmt.Fprintf(srv, "FF\r") // No more messages

	if line, _ := rd.ReadString('\r'); line != "FQ\r" {
		t.Errorf("Expected 'FQ', got '%s'", line)
	}

	if err := <-cerrs; err != nil {
		t.Errorf("Session exchange returned error: %s", err)
	}
}
