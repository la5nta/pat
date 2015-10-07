// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBusy              = errors.New("TNC control port is busy.")
	ErrConnectInProgress = errors.New("A connect is in progress.")
)

type TNC struct {
	ctrlAddr string
	connAddr string

	ctrl net.Conn
	data *tncConn

	in  broadcaster
	out chan<- string

	busy bool

	state State

	selfClose bool

	ptt PTT

	heard map[string]time.Time

	connected      bool
	listenerActive bool
}

func Open(addr string, mycall, gridSquare string) (*TNC, error) {
	ctrlAddr, connAddr, err := parseAddr(addr)
	if err != nil {
		return nil, ErrInvalidAddr
	}

	ctrlConn, err := net.Dial(`tcp`, ctrlAddr)
	if err != nil {
		return nil, err
	}

	tnc := &TNC{
		ctrlAddr: ctrlAddr,
		connAddr: connAddr,

		in:   newBroadcaster(),
		ctrl: ctrlConn,

		heard: make(map[string]time.Time),
	}

	if err := tnc.runControlLoop(); err == io.EOF {
		return nil, ErrBusy
	} else if err != nil {
		return nil, err
	}

	runtime.SetFinalizer(tnc, (*TNC).Close)

	if err := tnc.init(); err == io.EOF {
		return nil, ErrBusy
	} else if err != nil {
		return nil, fmt.Errorf("Failed to initialize TNC: %s", err)
	}

	if err = tnc.SetMycall(mycall); err != nil {
		return nil, fmt.Errorf("Set my call failed: %s", err)
	}

	if err = tnc.SetGridSquare(gridSquare); err != nil {
		return nil, fmt.Errorf("Set grid square failed: %s", err)
	}

	return tnc, nil
}

// Heard returns all stations heard by the TNC since it was opened.
//
// It returns a map from callsign to last time it was heard. Each call
// returns a new copy of the latest map.
func (tnc *TNC) Heard() map[string]time.Time {
	slice := make(map[string]time.Time, len(tnc.heard))
	for k, v := range tnc.heard {
		slice[k] = v
	}
	return slice
}

// Set the PTT that should be controlled by the TNC.
//
// If nil, the PTT request from the TNC is ignored.
func (tnc *TNC) SetPTT(ptt PTT) {
	tnc.ptt = ptt
}

func (tnc *TNC) init() (err error) {
	if tnc.state = tnc.getState(); tnc.state == Offline {
		if err = tnc.SetCodec(true); err != nil {
			return fmt.Errorf("Enable codec failed: %s", err)
		}
		time.Sleep(100 * time.Millisecond) // Give it some time
	}

	if err = tnc.SetMaxConnReq(10); err != nil {
		return fmt.Errorf("Set max connection requests failed: %s", err)
	}
	if err = tnc.SetRobust(false); err != nil {
		return fmt.Errorf("Disable robust mode failed: %s", err)
	}
	if v, err := tnc.get(cmdBusy); err != nil {
		return fmt.Errorf("Failed to get busy indication: %s", err)
	} else {
		tnc.busy = v.(bool)
	}

	// The TNC should only answer inbound ARQ connect requests when
	// requested by the user.
	if err = tnc.SetListenEnabled(false); err != nil {
		return fmt.Errorf("Disable listen failed: %s", err)
	}

	return nil
}

func (tnc *TNC) runControlLoop() error {
	// Read prompt so we know the TNC is ready
	tnc.ctrl.SetReadDeadline(time.Now().Add(3 * time.Second))
	rd := bufio.NewReader(tnc.ctrl)
	_, err := rd.ReadString('\n')
	if err != nil {
		return err
	}
	tnc.ctrl.SetReadDeadline(time.Time{})

	var selfDisconnect bool
	go func() {
		scanner := bufio.NewScanner(tnc.ctrl)

		for scanner.Scan() { // Handle async commands (status commands)
			line := scanner.Text()
			msg := parseCtrlMsg(line)

			switch msg.cmd {
			case cmdPTT:
				if tnc.ptt != nil {
					tnc.ptt.SetPTT(msg.Bool())
				}
			case cmdDisconnect:
				selfDisconnect = true
			case cmdMonitorCall:
				//TODO: the format is "N0CALL (JP20qe)", so we could keep the locator and return it in Heard()
				callsign := strings.Split(msg.value.(string), " ")[0]
				tnc.heard[callsign] = time.Now()
			case cmdBuffers:
				buffers := msg.value.([]int)
				tnc.data.updateBuffers(buffers)
			case cmdNewState:
				tnc.state = msg.State()

				// Close ongoing connections if the new state is Disconnected
				if msg.State() == Disconnected && tnc.data != nil {
					tnc.connected = false // connect() is responsible for setting it to true
					if tcpConn := tnc.data.Conn.(*net.TCPConn); !selfDisconnect {
						tcpConn.CloseRead()
						tcpConn.CloseWrite()
					} else {
						tcpConn.Close()
						selfDisconnect = false
					}
				}
			case cmdBusy:
				tnc.busy = msg.value.(bool)
			}

			if debugEnabled() {
				log.Printf("<-- %s\t[%#v]", line, msg)
			}
			tnc.in.Send(msg)
		}

		tnc.in.Close()
		close(tnc.out)
	}()

	out := make(chan string)
	tnc.out = out

	go func() {
		for str := range out {
			if debugEnabled() {
				log.Println("-->", str)
			}
			fmt.Fprintf(tnc.ctrl, "%s\r\n", str)
		}
	}()
	return nil
}

// Closes the connection to the TNC (and any on-going connections).
//
// This will not actually close the TNC software.
func (tnc *TNC) Close() error {
	if err := tnc.SetListenEnabled(false); err != nil {
		return err
	}

	if err := tnc.Disconnect(); err != nil { // Noop if idle
		return err
	}

	tnc.ctrl.Close()

	// no need for a finalizer anymore
	runtime.SetFinalizer(tnc, nil)

	return nil
}

// Returns true if channel is clear
func (tnc *TNC) Busy() bool {
	return tnc.busy
}

// Version returns the software version of the TNC
func (tnc *TNC) Version() (string, error) {
	return tnc.getString(cmdVersion)
}

// Returns the current state of the TNC
func (tnc *TNC) State() State {
	return tnc.state
}

func (tnc *TNC) SetResponseDelay(ms int) error {
	return tnc.set(cmdResponseDelay, ms)
}

// Returns the grid square as reported by the TNC
func (tnc *TNC) GridSquare() (string, error) {
	return tnc.getString(cmdGridSquare)
}

// Returns mycall as reported by the TNC
func (tnc *TNC) MyCall() (string, error) {
	return tnc.getString(cmdMyCall)
}

// Sets the grid square
func (tnc *TNC) SetGridSquare(gs string) error {
	return tnc.set(cmdGridSquare, gs)
}

// SetMycall sets the provided callsign as the main callsign for the TNC
func (tnc *TNC) SetMycall(mycall string) error {
	return tnc.set(cmdMyCall, mycall)
}

// Sets the auxiliary call signs that the TNC should answer to on incoming connections.
func (tnc *TNC) SetAuxiliaryCalls(calls []string) (err error) {
	return tnc.set(cmdMyAux, strings.Join(calls, ", "))
}

// Set the number of connect requests before giving up.
//
// Allowed values are 3-15
func (tnc *TNC) SetMaxConnReq(n int) error {
	return tnc.set(cmdMaxConnReq, n)
}

// SetRobust sets the TNC in robust mode.
//
// In robust mode the TNC will only use modes FSK4_2CarShort, FSK4_2Car or PSK4_2Car regardless of current data mode bandwidth setting.
// If in the ISS or ISSModeShift states changes will be delayed until the outbound queue is empty.
func (tnc *TNC) SetRobust(robust bool) error {
	return tnc.set(cmdRobust, robust)
}

// Enable/disable sound card and other resources
//
// This is done automatically on Open(), users should
// normally don't do this.
func (tnc *TNC) SetCodec(state bool) error {
	return tnc.set(cmdCodec, fmt.Sprintf("%t", state))
}

// ListenState() returns a StateReceiver which can be used
// to get notification when the TNC state changes.
func (tnc *TNC) ListenEnabled() StateReceiver {
	return tnc.in.ListenState()
}

// Enable/disable TNC response to an ARQ connect request.
//
// This is disabled automatically on Open(), and enabled
// when needed. Users should normally don't do this.
func (tnc *TNC) SetListenEnabled(listen bool) error {
	return tnc.set(cmdListen, fmt.Sprintf("%t", listen))
}

// Disconnect gracefully disconnects the active connection
// or cancels an ongoing connect.
//
// The method will block until the TNC is disconnected.
//
// If the TNC is not connecting/connected, Disconnect is
// a noop.
func (tnc *TNC) Disconnect() error {
	if tnc.Idle() {
		return nil
	}

	r := tnc.in.Listen()
	defer r.Close()

	tnc.out <- fmt.Sprintf("%s", cmdDisconnect)
	for msg := range r.Msgs() {
		if msg.cmd == cmdDisconnected {
			return nil
		}
	}
	panic("not possible")
}

// Idle returns true if the TNC is not in a connecting
// or connected state.
func (tnc *TNC) Idle() bool {
	return tnc.state == Disconnected || tnc.state == Offline
}

// DirtyDisconnect will send a dirty disconnect command to
// the TNC.
func (tnc *TNC) DirtyDisconnect() error {
	return tnc.set(cmdDirtyDisconnect, nil)
}

func (tnc *TNC) getState() State {
	v, err := tnc.get(cmdState)
	if err != nil {
		panic(fmt.Sprintf("getState(): %s", err))
	}
	return v.(State)
}

// Sends a connect command to the TNC. Users should call Dial().
func (tnc *TNC) connect(targetcall string) error {
	if !tnc.Idle() {
		return ErrConnectInProgress
	}

	r := tnc.in.Listen()
	defer r.Close()

	// Manual book keeping of state because ardop does not
	// send async state update after issuing a connect command.
	tnc.state = Connecting

	tnc.out <- fmt.Sprintf("%s %s", cmdConnect, targetcall)
	for msg := range r.Msgs() {
		if msg.cmd == cmdConnected {
			tnc.connected = true
			break
		} else if msg.cmd == cmdDisconnected {
			return ErrConnectTimeout
		}
	}
	return nil
}

func (tnc *TNC) set(cmd Command, param interface{}) (err error) {
	time.Sleep(100 * time.Millisecond)
	r := tnc.in.Listen()
	defer r.Close()

	if param != nil {
		tnc.out <- fmt.Sprintf("%s %v", cmd, param)
	} else {
		tnc.out <- string(cmd)
	}
	for msg := range r.Msgs() {
		if msg.cmd == cmdPrompt {
			return
		} else if msg.cmd == cmdFault {
			err = errors.New(msg.String())
		}
	}
	return errors.New("TNC hung up")
}

func (tnc *TNC) getString(cmd Command) (string, error) {
	v, err := tnc.get(cmd)
	if err != nil {
		return "", nil
	}
	return v.(string), nil
}

func (tnc *TNC) get(cmd Command) (value interface{}, err error) {
	time.Sleep(100 * time.Millisecond)

	r := tnc.in.Listen()
	defer r.Close()

	tnc.out <- string(cmd)
	for msg := range r.Msgs() {
		if msg.cmd == cmd {
			value = msg.value
		} else if msg.cmd == cmdFault {
			err = errors.New(msg.String())
		} else if msg.cmd == cmdPrompt {
			return
		}
	}
	return nil, errors.New("TNC hung up")
}

func parseAddr(addr string) (ctrlAddr, connAddr string, err error) {
	idxPort := strings.LastIndex(addr, ":")
	if idxPort < 0 || len(addr) < idxPort+1 {
		return ctrlAddr, connAddr, errors.New("Missing port")
	}

	host, strPort := addr[0:idxPort], addr[idxPort+1:]
	if len(strPort) < 2 {
		return ctrlAddr, connAddr, errors.New("Invalid port")
	} else if len(host) == 0 {
		return ctrlAddr, connAddr, errors.New("Invalid host address")
	}
	port, err := strconv.ParseInt(strPort, 0, 0)
	if err != nil {
		return ctrlAddr, connAddr, err
	}

	ctrlAddr = fmt.Sprintf("%s:%d", host, port)
	connAddr = fmt.Sprintf("%s:%d", host, port+1)
	return
}
