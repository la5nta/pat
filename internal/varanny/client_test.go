// Copyright 2024. All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package varanny

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

// Mock varanny server
func startMockVaranny(t *testing.T, responses map[string][]string) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimSpace(line)

					if resp, ok := responses[line]; ok {
						for _, r := range resp {
							conn.Write([]byte(r + "\n"))
						}
					}
				}
			}()
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func TestConnectAndList(t *testing.T) {
	responses := map[string][]string{
		"list": {"IC705HF", "THD74", "OK"},
	}

	addr, cleanup := startMockVaranny(t, responses)
	defer cleanup()

	host, port, _ := net.SplitHostPort(addr)
	portInt, _ := strconv.Atoi(port)

	ctx := context.Background()
	client, err := Connect(ctx, host, portInt)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	modems, err := client.ListModems(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(modems) != 2 {
		t.Errorf("expected 2 modems, got %d", len(modems))
	}

	if modems[0] != "IC705HF" {
		t.Errorf("expected IC705HF, got %s", modems[0])
	}
}

func TestStartAndStop(t *testing.T) {
	responses := map[string][]string{
		"start IC705HF": {"OK"},
		"stop":          {"OK"},
	}

	addr, cleanup := startMockVaranny(t, responses)
	defer cleanup()

	host, port, _ := net.SplitHostPort(addr)
	portInt, _ := strconv.Atoi(port)

	ctx := context.Background()
	client, err := Connect(ctx, host, portInt)
	if err != nil {
		t.Fatal(err)
	}

	if err := client.StartModem(ctx, "IC705HF"); err != nil {
		t.Fatal(err)
	}

	if !client.IsStarted() {
		t.Error("expected started to be true")
	}

	if err := client.StopModem(ctx); err != nil {
		t.Fatal(err)
	}

	if client.IsStarted() {
		t.Error("expected started to be false")
	}
}

func TestParseZeroconfEntry(t *testing.T) {
	entry := zeroconf.NewServiceEntry("IC705HF", "_vara-modem._tcp", "local.")
	entry.Port = 8300
	entry.AddrIPv4 = []net.IP{net.ParseIP("192.168.1.100")}
	entry.Text = []string{"type=hf", "launchport=8273", "catport=4532", "catdialect=hamlib"}

	info, err := ParseZeroconfEntry(entry)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Name", info.Name, "IC705HF"},
		{"Type", info.Type, "hf"},
		{"Host", info.Host, "192.168.1.100"},
		{"CmdPort", info.CmdPort, 8300},
		{"DataPort", info.DataPort, 8301},
		{"LaunchPort", info.LaunchPort, 8273},
		{"CatPort", info.CatPort, 4532},
		{"CatDialect", info.CatDialect, "hamlib"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestParseZeroconfEntryIPv6(t *testing.T) {
	entry := zeroconf.NewServiceEntry("THD74", "_vara-modem._tcp", "local.")
	entry.Port = 8400
	entry.AddrIPv6 = []net.IP{net.ParseIP("fe80::1")}
	entry.Text = []string{"type=fm"}

	info, err := ParseZeroconfEntry(entry)
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != "THD74" {
		t.Errorf("expected THD74, got %s", info.Name)
	}
	if info.Type != "fm" {
		t.Errorf("expected fm, got %s", info.Type)
	}
	if info.Host != "fe80::1" {
		t.Errorf("expected fe80::1, got %s", info.Host)
	}
	if info.LaunchPort != 8273 { // Default value
		t.Errorf("expected default launch port 8273, got %d", info.LaunchPort)
	}
}

func TestModemIsExpired(t *testing.T) {
	modem := ModemInfo{
		LastSeen: time.Now(),
	}

	if modem.IsExpired(time.Minute) {
		t.Error("modem should not be expired")
	}

	modem.LastSeen = time.Now().Add(-11 * time.Minute)
	if !modem.IsExpired(10 * time.Minute) {
		t.Error("modem should be expired")
	}
}

func TestNewSession(t *testing.T) {
	modemInfo := ModemInfo{
		Name:    "TEST",
		Type:    "hf",
		Host:    "127.0.0.1",
		CmdPort: 8300,
	}

	session := NewSession(nil, modemInfo, "varahf")
	if session == nil {
		t.Fatal("NewSession returned nil")
	}
	if session.Scheme() != "varahf" {
		t.Errorf("expected scheme varahf, got %s", session.Scheme())
	}
	info := session.ModemInfo()
	if info.Name != "TEST" {
		t.Errorf("expected modem name TEST, got %s", info.Name)
	}
	if session.Client() != nil {
		t.Error("Client() should return nil for nil client")
	}
}

func TestWaitForPortBindingTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Use a port that won't bind
	err := WaitForPortBinding(ctx, "127.0.0.1", 65535, 65534)

	if err == nil {
		t.Error("expected error from WaitForPortBinding")
	}
	// Accept either timeout or port binding error
	if !strings.Contains(err.Error(), "did not bind") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("expected port binding or timeout error, got: %v", err)
	}
}

func TestWaitForPortBindingContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	// Use a port that won't bind
	err := WaitForPortBinding(ctx, "127.0.0.1", 65535, 65534)

	if err == nil {
		t.Error("expected error from WaitForPortBinding")
	}
}