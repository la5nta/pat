// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"log"
	"strconv"
	"strings"
)

const (
	ModeARQ = "ARQ"
	ModeFEC = "FEC"
)

type Command string

const (
	cmdPending         Command = "PENDING"         // Indicates to the host application a Connect Request frame type has been detected (may not necessarily be to MYCALL or one of the MYAUX call signs). This provides an early warning to the host that a connection may be in process so it can hold any scanning activity.
	cmdCancelPending   Command = "CANCELPENDING"   // Indicates to the host that the prior PENDING Connect Request was not to MYCALL or one of the MYAUX call signs) This allows the Host to resume scanning.
	cmdCRCFault        Command = "CRCFAULT"        // Prompt to resend last frame
	cmdReady           Command = "RDY"             // Prompt
	cmdAbort           Command = "ABORT"           // Immediately aborts an ARQ Connection or a FEC Send session
	cmdARQBW           Command = "ARQBW"           // <200MAX|500MAX|1000MAX|2000MAX|200FORCED|500FORCED|1000FORCED|2000FORCED>
	cmdARQTimeout      Command = "ARQTIMEOUT"      // ARQTIMEOUT<30-240> Set/get the ARQ Timeout in seconds
	cmdARQCall         Command = "ARQCALL"         // <Target Callsign Repeat Count>
	cmdBuffer          Command = "BUFFER"          // <[int int int int int]: Buffer statistics
	cmdClose           Command = "CLOSE"           // Provides an orderly shutdown of all connections, release of all sound card resources and closes the Virtual TNC Program or hardware
	cmdCodec           Command = "CODEC"           // Start the Codec with True, Stop with False. No parameter will return the Codec state
	cmdCWID            Command = "CWID"            // <>[bool]: Disable/Enable the CWID option. CWID is optionally sent at the end of each ID frame.
	cmdDisconnect      Command = "DISCONNECT"      // Initiates a normal disconnect cycle for an ARQ connection. If not connected command is ignored.
	cmdCapture         Command = "CAPTURE"         // <device name>
	cmdDriveLevel      Command = "DRIVELEVEL"      // Set Drive level. Default = 100 (max)
	cmdGridSquare      Command = "GRIDSQUARE"      // <4, 6 or 8 character grid square>Sets or retrieves the 4, 6, or 8 character Maidenhead grid square (used in ID Frames) an improper grid square syntax will return a FAULT.
	cmdInitialize      Command = "INITIALIZE"      // Clears any pending queued values in the TNC interface. Should be sent upon initial connection and before any other parameters are sent
	cmdListen          Command = "LISTEN"          // Enables/disables server’s response to an ARQ connect request. Default = True. May be used to block connect requests during scanning.
	cmdMyAux           Command = "MYAUX"           // <aux call sign1, aux call sign2, … aux call sign10>
	cmdMyCall          Command = "MYCALL"          // Sets current call sign. If not a valid call generates a FAULT. Legitimate call signs include from 3 to 7 ASCII characters (A-Z, 0-9) followed by an optional “-“ and an SSID of -0 to -15 or -A to -Z. An SSID of -0 is treated as no SSID
	cmdPlayback        Command = "PLAYBACK"        // <device name>Sets desired sound card playback device. If no device name will reply with the current assigned playback device.
	cmdProtocolMode    Command = "PROTOCOLMODE"    // PROTOCOLMODE<ARQ|FEC> Sets/Gets the protocol mode. If ARQ and LISTEN above is TRUE will answer Connect requests to MYCALL or any call signs in MYAUX. If FEC will decode but not respond to any connect request.
	cmdTwoToneTest     Command = "TWOTONETEST"     // Send 5 second two-tone burst at the normal leader amplitude. May be used in adjusting drive level to the radio. If sent while in any state except DISC will result in a fault “not from state .....”
	cmdVersion         Command = "VERSION"         // Returns the name and version of the ARDOP TNC program or hardware implementation.
	cmdStatus          Command = "STATUS"          // ? e.g.: "STATUS CONNECT TO LA3F FAILED!"
	cmdNewState        Command = "NEWSTATE"        // <[State]: Sent when the state changes
	cmdDisconnected    Command = "DISCONNECTED"    // <[]: Signals that a connect failed. Duplicate state notification?
	cmdConnected       Command = "CONNECTED"       // <[string string]: Signals that an ARQ connection has been established. e.g. “CONNECTED W1ABC 500”
	cmdPTT             Command = "PTT"             // <[bool]: PTT active or not
	cmdFault           Command = "FAULT"           // <[string]: Error message
	cmdBusy            Command = "BUSY"            // <[bool]: Returns whether the channel is busy
	cmdTarget          Command = "TARGET"          // <[string]: Identifies the target call sign of the connect request. The target call will be either MYC or one of the MYAUX call signs.
	cmdCaptureDevices  Command = "CATPUREDEVICES"  // Returns a comma delimited list of all currently installed capture devices
	cmdPlaybackDevices Command = "PLAYBACKDEVICES" // Returns a comma delimited list of all currently installed playback devices.
	cmdAutoBreak       Command = "AUTOBREAK"       // <>[bool]: Disables/enables automatic link turnover (BREAK) by IRS when IRS has outbound data pending and receives an IDLE frame from ISS indicating its’ outbound queue is empty. Default is True.

	// Some of the commands that has not been implemented:
	cmdBreak         Command = "BREAK"
	cmdBusyLock      Command = "BUSYLOCK"
	cmdRadioTuner    Command = "RADIOTUNER"
	cmdRadioAnt      Command = "RADIOANT"      // Selects the radio antenna 1 or 2 for those radios that support antenna switching. If the parameter is 0 will not change the antenna setting even if the radio supports it. If sent without a parameter will return 0, 1 or 2. If RADIOCONTROL Is false or RADIOMODEL has not been set will return FAULT
	cmdRadioCtrl     Command = "RADIOCTRL"     // Enables/disables the radio control capability of the ARDOP_Win TNC. If sent without a parameter will return the current value of RADIOCONTROL enable.
	cmdRadioCtrlBaud Command = "RADIOCTRLBAUD" // <1200-115200)
	cmdRadioCtrlDTR  Command = "RADIOCTRLDTR"  //
	cmdRadioCtrlPort Command = "RADIOCTRLPORT" // COMn
	cmdRadioCtrlRTS  Command = "RADIOCTRLRTS"  //
	cmdRadioFilter   Command = "RADIOFILTER"   //
	cmdRadioFreq     Command = "RADIOFREQ"     //
	cmdRadioComAdd   Command = "RADIOCOMADD"   // 00-FF> Sets/reads the current Icom Address for radio control (Icom radios only). Values must be hex 00 through FF
	cmdRadioISC      Command = "RADIOISC"      // Enable/Disable Radio’s internal sound card (some radios)
	cmdRadioMenu     Command = "RADIOMENU"
	cmdRadioMode     Command = "RADIOMODE" // USB,USBD, FM>
	cmdRadioModel    Command = "RADIOMODEL"
	cmdRadioModels   Command = "RADIOMODELS"
	cmdRadioPTT      Command = "RADIOPTT" // CATPTT|VOX/SIGNALINK|COMn
	cmdRadioPTTDTR   Command = "RADIOPTTDTR"
	cmdRadioPTTRTS   Command = "RADIOPTTRTS"
	cmdSendID        Command = "SENDID"
	cmdSetupMenu     Command = "SETUPMENU"
	cmdSquelch       Command = "SQUELCH"
	cmdState         Command = "STATE"
	cmdTrailer       Command = "TRAILER"
	cmdTuneRange     Command = "TUNERANGE"
	cmdLeader        Command = "LEADER"     // LEADER<100-2000> Get/Set the leader length in ms. (Default is 160 ms). Rounded to the nearest 10 ms.
	cmdDataToSend    Command = "DATATOSEND" // If sent with the parameter 0 (zero) it will clear the TNC’s data to send Queue. If sent without a parameter will return the current number of data to send bytes queued.
	cmdDebugLog      Command = "DEBUGLOG"   // Enable/disable the debug log
	cmdDisplay       Command = "DISPLAY"    // Sets the Dial frequency display of the Waterfall or Spectrum display. If sent without parameters will return the current Dial frequency display. If > 100000 Display will read in MHz.
	cmdTrace         Command = "CMDTRACE"   // Get/Set Command Trace flag to log all commands to from the TNC to the ARDOP_Win TNC debug log.
	cmdFECid         Command = "FECID"      // Disable/Enable ID (with optional grid square) at start of FEC transmissions
	cmdFECmode       Command = "FECMODE"    // FECMODE<8FSK.200.25|4FSK.200.50S|4FSK.200.50,4PSK.200.100S|4PSK.200.100|8PSK.200.100|16FSK.500.25S|16FSK.500.25|4FSK.500.100S|4FSK.500.100| 4PSK.500.100|8PSK.500.100|4PSK.500.167|8PSK.500.167|4FSK.1000.100|4PSK.1000.100|8PSK.1000.100|4PSK.1000.167|8PSK.1000.167|4FSK.2000.600S|4FSK.2000.600|4FSK.2000.100|4PSK.2000.100|8PSK.2000.100|4PSK.2000.167|8PSK.2000.167
	cmdFECrepeats    Command = "FECREPEATS" // <0-5> Sets the number of times a frame is repeated in FEC (multicast) mode. Higher number of repeats increases good copy probability under marginal conditions but reduces net throughput.
	cmdFECsend       Command = "FECSEND"    // Start/Stop FEC broadcast/multicast mode for specific FECMODE. FECSEND <False> will abort a FEC broadcast.

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
	case cmdCodec, cmdPTT, cmdBusy, cmdTwoToneTest, cmdCWID, cmdListen, cmdAutoBreak:
		msg.value = strings.ToLower(parts[1]) == "true"

	// (no params)
	case cmdReady, cmdAbort, cmdDisconnect, cmdClose, cmdDisconnected, cmdCRCFault, cmdPending, cmdCancelPending:

	// State
	case cmdNewState, cmdState:
		msg.value = stateMap[strings.ToUpper(parts[1])]

	// string
	case cmdFault, cmdMyCall, cmdGridSquare, cmdCapture,
		cmdPlayback, cmdVersion, cmdTarget, cmdStatus:
		msg.value = parts[1]

	// []string (space separated)
	case cmdConnected:
		msg.value = parseList(parts[1], " ")

		// []string (comma separated)
	case cmdCaptureDevices, cmdPlaybackDevices, cmdMyAux:
		msg.value = parseList(parts[1], ",")

	// int
	case cmdDriveLevel, cmdBuffer, cmdARQTimeout:
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

func parseList(str, sep string) []string {
	parts := strings.Split(str, sep)
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
