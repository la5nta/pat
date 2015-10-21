// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package ardop provides means of establishing a connection to a remote node using ARDOP TNC
package ardop

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// The default address Ardop TNC listens on
const DefaultAddr = "localhost:8515"

var (
	ErrConnectTimeout = errors.New("Connect timeout")
	ErrInvalidAddr    = errors.New("Invalid address format")
)

type Bandwidth struct {
	Forced bool
	Max    uint
}

var (
	Bandwidth200Max  = Bandwidth{false, 200}
	Bandwidth500Max  = Bandwidth{false, 500}
	Bandwidth1000Max = Bandwidth{false, 1000}
	Bandwidth2000Max = Bandwidth{false, 2000}

	Bandwidth200Forced  = Bandwidth{true, 200}
	Bandwidth500Forced  = Bandwidth{true, 500}
	Bandwidth1000Forced = Bandwidth{true, 1000}
	Bandwidth2000Forced = Bandwidth{true, 2000}
)

func (bw Bandwidth) String() string {
	str := fmt.Sprintf("%d", bw.Max)
	if bw.Forced {
		str += "FORCED"
	} else {
		str += "MAX"
	}
	return str
}

func (bw Bandwidth) IsZero() bool { return bw.Max == 0 }

type State uint8

//go:generate stringer -type=State .
const (
	Unknown      State = iota
	Offline            // Sound card disabled and all sound card resources are released
	Disconnected       // The session is disconnected, the sound card remains active
	ISS                // Information Sending Station (Sending Data)
	IRS                // Information Receiving Station (Receiving data)
	Idle               // ??
	FECSend            // ??
	FECReceive         // Receiving FEC (unproto) data
)

var stateMap = map[string]State{
	"":        Unknown,
	"OFFLINE": Offline,
	"DISC":    Disconnected,
	"ISS":     ISS,
	"IRS":     IRS,
	"IDLE":    Idle,
	"FECRcv":  FECReceive,
	"FECSend": FECSend,
}

func strToState(str string) (State, bool) {
	state, ok := stateMap[strings.ToUpper(str)]
	return state, ok
}

func debugEnabled() bool {
	return os.Getenv("ardop_debug") != ""
}
