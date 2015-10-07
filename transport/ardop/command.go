// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"log"
	"strconv"
	"strings"
)

type Command string

const (
    //ardop
	cmdAbort           Command = "ABORT"           // Immediately aborts an ARQ Connection or a FEC Send session
	cmdArqBW           Command = "ARQBW"           // <200MAX|500MAX|1000MAX|2000MAX|200FORCED|500FORCED|1000FORCED|2000FORCED>
	cmdArqTimeout      Command = "ARQTIMEOUT"      // ARQTIMEOUT<30-240> Set/get the ARQ Timeout in seconds
	cmdArqCall         Command = "ARQCALL"         // <Target Callsign Repeat Count>
	cmdBuffer          Command = "BUFFER"          //
	cmdCapture         Command = "CAPTURE"         // <device name>
	cmdCaptureDevices  Command = "CATPUREDEVICES"  // Returns a comma delimited list of all currently installed capture devices
	cmdClose           Command = "CLOSE"           // Provides an orderly shutdown of all connections, release of all sound card resources and closes the Virtual TNC Program or hardware
	cmdTrace           Command = "CMDTRACE"        // Get/Set Command Trace flag to log all commands to from the TNC to the ARDOP_Win TNC debug log.
	cmdCodec           Command = "CODEC"           // Start the Codec with True, Stop with False. No parameter will return the Codec state
	cmdCWid            Command = "CWID"            // Disable/Enable the CWID option. CWID is optionally sent at the end of each ID frame.
	cmdDataToSend      Command = "DATATOSEND"      // If sent with the parameter 0 (zero) it will clear the TNC’s data to send Queue. If sent without a parameter will return the current number of data to send bytes queued.
    	cmdDebugLog        Command = "DEBUGLOG"        // Enable/disable the debug log
    	cmdDisconnect      Command = "DISCONNECT"      // Initiates a normal disconnect cycle for an ARQ connection. If not connected command is ignored.
    	cmdDisplay         Command = "DISPLAY"         // Sets the Dial frequency display of the Waterfall or Spectrum display. If sent without parameters will return the current Dial frequency display. If > 100000 Display will read in MHz.
    	cmdDriveLevel      Command = "DRIVELEVEL"	   // Set Drive level. Default = 100 (max)
	cmdFECid           Command = "FECID"           // Disable/Enable ID (with optional grid square) at start of FEC transmissions
	cmdFECmode         Command = "FECMODE"         // FECMODE<8FSK.200.25|4FSK.200.50S|4FSK.200.50,4PSK.200.100S|4PSK.200.100|8PSK.200.100|16FSK.500.25S|16FSK.500.25|4FSK.500.100S|4FSK.500.100| 4PSK.500.100|8PSK.500.100|4PSK.500.167|8PSK.500.167|4FSK.1000.100|4PSK.1000.100|8PSK.1000.100|4PSK.1000.167|8PSK.1000.167|4FSK.2000.600S|4FSK.2000.600|4FSK.2000.100|4PSK.2000.100|8PSK.2000.100|4PSK.2000.167|8PSK.2000.167
    	cmdFECrepeats      Command = "FECREPEATS"      // <0-5> Sets the number of times a frame is repeated in FEC (multicast) mode. Higher number of repeats increases good copy probability under marginal conditions but reduces net throughput.
	cmdFECsend         Command = "FECSEND"         // Start/Stop FEC broadcast/multicast mode for specific FECMODE. FECSEND <False> will abort a FEC broadcast.
	cmdGridSquare      Command = "GRIDSQUARE"      // <4, 6 or 8 character grid square>Sets or retrieves the 4, 6, or 8 character Maidenhead grid square (used in ID Frames) an improper grid square syntax will return a FAULT.
    	cmdInitialize      Command = "INITIALIZE"      // Clears any pending queued values in the TNC interface. Should be sent upon initial connection and before any other parameters are sent
	cmdLeader          Command = "LEADER"          // LEADER<100-2000> Get/Set the leader length in ms. (Default is 160 ms). Rounded to the nearest 10 ms.
    	cmdListen          Command = "LISTEN"          // Enables/disables server’s response to an ARQ connect request. Default = True. May be used to block connect requests during scanning.
	cmdMyAux           Command = "MYAUX"           // <aux call sign1, aux call sign2, … aux call sign10>
	cmdMyCall          Command = "MYCALL"          // Sets current call sign. If not a valid call generates a FAULT. Legitimate call signs include from 3 to 7 ASCII characters (A-Z, 0-9) followed by an optional “-“ and an SSID of -0 to -15 or -A to -Z. An SSID of -0 is treated as no SSID
	cmdPlayback        Command = "PLAYBACK"        // <device name>Sets desired sound card playback device. If no device name will reply with the current assigned playback device.
    	cmdPlaybackDevices Command = "PLAYBACKDEVICES" // Returns a comma delimited list of all currently installed playback devices.
	cmdProtocolMode    Command = "PROTOCOLMODE"    // PROTOCOLMODE<ARQ|FEC> Sets/Gets the protocol mode. If ARQ and LISTEN above is TRUE will answer Connect requests to MYCALL or any call signs in MYAUX. If FEC will decode but not respond to any connect request.
	cmdRadioAnt        Command = "RADIOANT"        // Selects the radio antenna 1 or 2 for those radios that support antenna switching. If the parameter is 0 will not change the antenna setting even if the radio supports it. If sent without a parameter will return 0, 1 or 2. If RADIOCONTROL Is false or RADIOMODEL has not been set will return FAULT
	cmdRadioCtrl       Command = "RADIOCTRL"       // Enables/disables the radio control capability of the ARDOP_Win TNC. If sent without a parameter will return the current value of RADIOCONTROL enable.
	cmdRadioCtrlBaud   Command = "RADIOCTRLBAUD"   // <1200-115200)
	cmdRadioCtrlDTR    Command = "RADIOCTRLDTR"    //
	cmdRadioCtrlPort   Command = "RADIOCTRLPORT"   // COMn
	cmdRadioCtrlRTS    Command = "RADIOCTRLRTS"    //
	cmdRadioFilter     Command = "RADIOFILTER"     //
	cmdRadioFreq       Command = "RADIOFREQ"       //
	cmdRadioComAdd     Command = "RADIOCOMADD"     // 00-FF> Sets/reads the current Icom Address for radio control (Icom radios only). Values must be hex 00 through FF
    	cmdRadioISC        Command = "RADIOISC"        // Enable/Disable Radio’s internal sound card (some radios)
	cmdRadioMenu       Command = "RADIOMENU"
    	cmdRadioMode       Command = "RADIOMODE"       // USB,USBD, FM>
	cmdRadioModel      Command = "RADIOMODEL"
	cmdRadioModels     Command = "RADIOMODELS"
	cmdRadioPTT        Command = "RADIOPTT"        // CATPTT|VOX/SIGNALINK|COMn
	cmdRadioPTTDTR     Command = "RADIOPTTDTR"
	cmdRadioPTTRTS     Command = "RADIOPTTRTS"
	// end of radio commands
	
	cmdSendID          Command = "SENDID"
	cmdSetupMenu       Command = "SETUPMENU"
	cmdSquelch         Command = "SQUELCH"
	cmdState           Command = "STATE"
	cmdTrailer         Command = "TRAILER"
	cmdTuneRange       Command = "TUNERANGE"
	cmdTwoToneTest     Command = "TWOTONETEST"
	cmdVersion         Command = "VERSION"

	// ardop not implemented
	cmdAutoBreak       Command = "AUTOBREAK"
	cmdBreak           Command = "BREAK"
	cmdBusyLock        Command = "BUSYLOCK"
	cmdRadioTuner      Command = "RADIOTUNER"
	
	//
	//cmdRobust          Command = "ROBUST"          // <>[bool]: Force the most robust mode (2x4FSK) for the entire session
	//cmdPrompt          Command = "CMD"             // <[]: Seems like a command prompt
	//cmdPrompt2         Command = "CMDTRACE"        // CMDTRACE<True|False> Get/Set Command Trace flag to log all commands to from the TNC to the ARDOP_Win TNC debug log.
	//cmdCodec           Command = "CODEC"           // <>[bool]: Activate sound card (can never be turned off?)
	//cmdNewState        Command = "NEWSTATE"        // <[State]: Sent when the state changes
	//cmdState           Command = "STATE"           // <>[State]: The getter to get current state
	//cmdConnect         Command = "CONNECT"         // >[string]: Connect to the given callsign. Failure response is "FAULT Connect Failure".
	//cmdDisconnect      Command = "DISCONNECT"      // >[]: Disconnect the current session
	//cmdDirtyDisconnect Command = "DIRTYDISCONNECT" // >[]: "abort" connection
	//cmdAbortDisconnect Command = "ABORT"           // Immediately aborts an ARQ Connection or a FEC Send session
	//cmdDisconnected    Command = "DISCONNECTED"    // <[]: Signals that a connect failed. Duplicate state notification?
	//cmdConnected       Command = "CONNECTED"       // <[string]: Signals that a connect was ok. Duplicate state notification?
	//cmdPTT             Command = "PTT"             // <[bool]: PTT active or not
	//cmdClose           Command = "CLOSE"           // >[]: Closes the TNC
	//cmdBuffer         Command = "BUFFER"         // <[int int int int int]: Buffer status?
	//cmdFault           Command = "FAULT"           // <[string]: Error message
	//cmdOffset          Command = "OFFSET"          // <[int]: Offset
	//cmdTune            Command = "TUNE"          // <[int]: <Tuning offset in integer Hz>
	//cmdMyCall          Command = "MYC"             // <>[string]: My callsign
	//cmdMyCall          Command = "MYCALL"             // <>[string]: My callsign
	//cmdGridSquare      Command = "GRIDSQUARE"      // <>[string]: set/get grid square
	//cmdMaxConnReq      Command = "MAXCONREQ"       // <>[int 3-15]: Number of connect requests before giving up. RMS Express sets 10 before connect.
	//cmdDriveLevel      Command = "DRIVELEVEL"      // <>[int]: Set/read the drive level (TX audio drive)
	//cmdBusy            Command = "BUSY"            // <[bool]: Returns whether the channel is busy
	//cmdCapture         Command = "CAPTURE"         // <>[string]: capture device
	//cmdPlayback        Command = "PLAYBACK"        // <>[string]: playback device
	//cmdTwoToneTest     Command = "TWOTONETEST"     // >[bool]: Enable two tone test
	//cmdCWID            Command = "CWID"            // <>[bool]: cw id
	//cmdMode            Command = "MODE"            // <>[string]: Current data mode
	//cmdMyAux           Command = "MYAUX"           // <>[string,string...]: Auxiliary call signs that will answer connect requests
	//cmdVersion         Command = "VERSION"         // <>[string]: Returns the TNC version
	//cmdListen          Command = "LISTEN"          // <>[bool]: Enables/disables server’s response to an ARQ connect request.
	//cmdResponseDelay   Command = "RESPONSEDLY"     // <>[int]: Sets or returns the minimum response delay in ms. (300-2000, documented as 0-2000).
	//cmdBandwidth       Command = "BW"              // <[int]: Used to answer a incoming call. Sets inbound bandwidth (500/1600). Only when in "server" mode.
	//cmdMonitorCall     Command = "MONCALL"         // <[string]: sent when a station id is heard.
	//cmdTarget          Command = "TARGET"          // <[string]: The newly connected station's call sign (in "server" mode)

	// Not implemented in parser
	//cmdCaptureDevices Command = "CAPTUREDEVICES" // <>[string,string...]: List of all available capture devices
	//cmdSuffix         Command = "SUFFIX"         // <>[string]: ?
	//cmdAutoBreak      Command = "AUTOBREAK"      // <>[bool]: ?
	//cmdBusyLock       Command = "BUSYLOCK"       // <>[bool]: ?
	//cmdBusyHold       Command = "BUSYHOLD"       // <>[int]: This defines the time the software waits after the Controller has reported the channel free before considering it free
	//cmdBusyWait       Command = "BUSYWAIT"       // <>[int]: This changes the time the software will wait for a clear channel before failing a connect request
	//cmdVox            Command = "VOX"            // <>[bool]: ?
	//cmdFECRcv         Command = "FECRCV"         // <>[bool]: ?
	//cmdShow           Command = "SHOW"           // <>[bool] ? -- not settable?
	//cmdSpeedTest      Command = "SPEEDTEST"      // <>[int]: DSP speed test
	//cmdSendID         Command = "SENDID"         // >?[int]: delay parameter 0-15 required
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
