// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package winmor

import (
	"log"
	"strconv"
	"strings"
)

type Command string

const (
	cmdRobust          Command = "ROBUST"          // <>[bool]: Force the most robust mode (2x4FSK) for the entire session
	cmdPrompt          Command = "CMD"             // <[]: Seems like a command prompt
	cmdCodec           Command = "CODEC"           // <>[bool]: Activate sound card (can never be turned off?)
	cmdNewState        Command = "NEWSTATE"        // <[State]: Sent when the state changes
	cmdState           Command = "STATE"           // <>[State]: The getter to get current state
	cmdConnect         Command = "CONNECT"         // >[string]: Connect to the given callsign. Failure response is "FAULT Connect Failure".
	cmdDisconnect      Command = "DISCONNECT"      // >[]: Disconnect the current session
	cmdDirtyDisconnect Command = "DIRTYDISCONNECT" // >[]: "abort" connection
	cmdDisconnected    Command = "DISCONNECTED"    // <[]: Signals that a connect failed. Duplicate state notification?
	cmdConnected       Command = "CONNECTED"       // <[string]: Signals that a connect was ok. Duplicate state notification?
	cmdPTT             Command = "PTT"             // <[bool]: PTT active or not
	cmdClose           Command = "CLOSE"           // >[]: Closes the TNC
	cmdBuffers         Command = "BUFFERS"         // <[int int int int int]: Buffer status?
	cmdFault           Command = "FAULT"           // <[string]: Error message
	cmdOffset          Command = "OFFSET"          // <[int]: Offset
	cmdMyCall          Command = "MYC"             // <>[string]: My callsign
	cmdGridSquare      Command = "GRIDSQUARE"      // <>[string]: set/get grid square
	cmdMaxConnReq      Command = "MAXCONREQ"       // <>[int 3-15]: Number of connect requests before giving up. RMS Express sets 10 before connect.
	cmdDriveLevel      Command = "DRIVELEVEL"      // <>[int]: Set/read the drive level (TX audio drive)
	cmdBusy            Command = "BUSY"            // <[bool]: Returns whether the channel is busy
	cmdCapture         Command = "CAPTURE"         // <>[string]: capture device
	cmdPlayback        Command = "PLAYBACK"        // <>[string]: playback device
	cmdTwoToneTest     Command = "TWOTONETEST"     // >[bool]: Enable two tone test
	cmdCWID            Command = "CWID"            // <>[bool]: cw id
	cmdMode            Command = "MODE"            // <>[string]: Current data mode
	cmdMyAux           Command = "MYAUX"           // <>[string,string...]: Auxiliary call signs that will answer connect requests
	cmdVersion         Command = "VERSION"         // <>[string]: Returns the TNC version
	cmdListen          Command = "LISTEN"          // <>[bool]: Enables/disables server’s response to an ARQ connect request.
	cmdResponseDelay   Command = "RESPONSEDLY"     // <>[int]: Sets or returns the minimum response delay in ms. (300-2000, documented as 0-2000).
	cmdBandwidth       Command = "BW"              // <[int]: Used to answer a incoming call. Sets inbound bandwidth (500/1600). Only when in "server" mode.
	cmdMonitorCall     Command = "MONCALL"         // <[string]: sent when a station id is heard.
	cmdTarget          Command = "TARGET"          // <[string]: The newly connected station's call sign (in "server" mode)

	// Not implemented in parser
	cmdCaptureDevices Command = "CAPTUREDEVICES" // <>[string,string...]: List of all available capture devices
	cmdSuffix         Command = "SUFFIX"         // <>[string]: ?
	cmdAutoBreak      Command = "AUTOBREAK"      // <>[bool]: ?
	cmdBusyLock       Command = "BUSYLOCK"       // <>[bool]: ?
	cmdBusyHold       Command = "BUSYHOLD"       // <>[int]: This defines the time the software waits after the Controller has reported the channel free before considering it free
	cmdBusyWait       Command = "BUSYWAIT"       // <>[int]: This changes the time the software will wait for a clear channel before failing a connect request
	cmdVox            Command = "VOX"            // <>[bool]: ?
	cmdFECRcv         Command = "FECRCV"         // <>[bool]: ?
	cmdShow           Command = "SHOW"           // <>[bool] ? -- not settable?
	cmdSpeedTest      Command = "SPEEDTEST"      // <>[int]: DSP speed test
	cmdSendID         Command = "SENDID"         // >?[int]: delay parameter 0-15 required
)

// Buffer slice index
//
// "BUFFERS <in queued> <in sequenced> <out queued> <out confirmed> <1m avg throughput in bytes/minute>"
const (
	BufferInQueued = iota
	BufferInSequenced
	BufferOutQueued
	BufferOutConfirmed
	BufferAvgThroughput
)

type ctrlMsg struct {
	cmd   Command
	value interface{}
}

func (msg ctrlMsg) Bool() bool {
	return msg.value.(bool)
}

func (msg ctrlMsg) State() State {
	return msg.value.(State)
}

func (msg ctrlMsg) String() string {
	return msg.value.(string)
}

func (msg ctrlMsg) Int() int {
	return msg.value.(int)
}

func parseCtrlMsg(str string) ctrlMsg {
	parts := strings.SplitN(str, " ", 2)
	parts[0] = strings.ToUpper(parts[0])

	msg := ctrlMsg{
		cmd: Command(parts[0]),
	}

	switch msg.cmd {
	// bool
	case cmdRobust, cmdCodec, cmdPTT, cmdBusy, cmdTwoToneTest, cmdCWID, cmdListen:
		msg.value = strings.ToLower(parts[1]) == "true"

	// (no params)
	case cmdPrompt, cmdDisconnect, cmdDirtyDisconnect, cmdClose, cmdDisconnected:

	// State
	case cmdNewState, cmdState:
		msg.value = stateMap[strings.ToUpper(parts[1])]

	// string
	case cmdConnect, cmdFault, cmdConnected, cmdMyCall, cmdGridSquare, cmdCapture,
		cmdPlayback, cmdMode, cmdVersion, cmdMonitorCall, cmdTarget:
		msg.value = parts[1]

	// []string
	case cmdMyAux:
		msg.value = parseCommaList(parts[1])

	// []int (whitespace separated)
	case cmdBuffers: // <in queued> <in sequenced> <out queued> <out confirmed> <1m avg throughput in bytes/minute>
		v, err := parseIntList(parts[1], " ")
		if err != nil {
			log.Printf("Failed to parse %s: %s", cmdBuffers, err)
		}
		msg.value = v

	// int
	case cmdOffset, cmdMaxConnReq, cmdDriveLevel, cmdResponseDelay, cmdBandwidth:
		i, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Failed to parse offset value: %s", err)
		}
		msg.value = i

	default:
		log.Printf("Unable to parse '%s'", str)
	}

	return msg
}

func parseIntList(str, delim string) ([]int, error) {
	strSlice := strings.Split(str, delim)

	ints := make([]int, len(strSlice))
	for i, p := range strSlice {
		n, err := strconv.Atoi(p)
		if err != nil {
			return ints, err
		}

		ints[i] = n
	}

	return ints, nil
}

func parseCommaList(str string) []string {
	parts := strings.Split(str, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
