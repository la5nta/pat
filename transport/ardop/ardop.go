// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package ardop provides means of establishing a connection to a remote node using ARDOP TNC
package ardop

import (
	"errors"
	"os"
	"strings"
)

type PTT interface {
	SetPTT(on bool) error
}

type State uint8

// The default address Ardop TNC listens on
const DefaultAddr = "localhost:8515"

var (
	ErrConnectTimeout = errors.New("Connect timeout")
	ErrInvalidAddr    = errors.New("Invalid address format")
)

//go:generate stringer -type=State
const (
	Unknown        State = iota
	Offline              // Sound card disabled and all sound card resources are released
	Disconnected         // The session is disconnected, the sound card remains active
	Connecting           // The station is sending Connect Requests to a target station
	ConnectPending       // A connect request frame was sensed and a connection is pending and capture/decoding is in process
	SendID               // Sending ID
	ISS                  // Information Sending Station (Sending Data)
	IRS                  // Information Receiving Station (Receiving data)
	IRSToISS             // Transition state from IRS to ISS to insure proper link turnover
	IRSModeShift         // Supplying Packets Sequenced information to the ISS for a requested mode shift
	ISSModeShift         // Requesting Packets Sequenced information from the IRS in preparation for a mode shift
	FECReceive           // Receiving FEC (unproto) data
	FEC500               // Sending FEC data 500Hz bandwidth
	FEC1600              // Sending FEC data 1600Hz bandwidth
)

var stateMap = map[string]State{
	"":               Unknown,
	"OFFLINE":        Offline,
	"DISCONNECTED":   Disconnected,
	"CONNECTING":     Connecting,
	"CONNECTPENDING": ConnectPending,
	"SENDID":         SendID,
	"ISS":            ISS,
	"IRS":            IRS,
	"IRSTOISS":       IRSToISS,
	"IRS MODE SHIFT": IRSModeShift,
	"ISS MODE SHIFT": ISSModeShift,
	"FECRCV":         FECReceive,
	"FEC500":         FEC500,
	"FEC1600":        FEC1600,
}

func strToState(str string) (State, bool) {
	state, ok := stateMap[strings.ToUpper(str)]
	return state, ok
}

func debugEnabled() bool {
	return os.Getenv("ardop_debug") != ""
}
