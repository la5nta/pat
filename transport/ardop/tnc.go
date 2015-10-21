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
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/transport"
)

const DefaultARQTimeout = 90 * time.Second

var (
	ErrBusy              = errors.New("TNC control port is busy.")
	ErrConnectInProgress = errors.New("A connect is in progress.")
	ErrFlushTimeout      = errors.New("Flush timeout.")
)

type TNC struct {
	ctrl io.ReadWriteCloser
	data *tncConn

	in      broadcaster
	out     chan<- string
	dataOut chan<- []byte
	dataIn  chan []byte

	busy bool

	state State

	selfClose bool

	ptt transport.PTTController

	connected      bool
	listenerActive bool
}

// OpenTCP opens and initializes an ardop TNC over TCP.
func OpenTCP(addr string, mycall, gridSquare string) (*TNC, error) {
	tcpConn, err := net.Dial(`tcp`, addr)
	if err != nil {
		return nil, err
	}

	return Open(tcpConn, mycall, gridSquare)
}

// OpenTCP opens and initializes an ardop TNC.
func Open(conn io.ReadWriteCloser, mycall, gridSquare string) (*TNC, error) {
	var err error

	tnc := &TNC{
		in:     newBroadcaster(),
		dataIn: make(chan []byte, 4096),
		ctrl:   conn,
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

// Set the PTT that should be controlled by the TNC.
//
// If nil, the PTT request from the TNC is ignored.
func (tnc *TNC) SetPTT(ptt transport.PTTController) {
	tnc.ptt = ptt
}

func (tnc *TNC) init() (err error) {
	if err = tnc.set(cmdInitialize, nil); err != nil {
		return err
	}

	if tnc.state = tnc.getState(); tnc.state == Offline {
		if err = tnc.SetCodec(true); err != nil {
			return fmt.Errorf("Enable codec failed: %s", err)
		}
	}

	if err = tnc.set(cmdProtocolMode, ModeARQ); err != nil {
		return fmt.Errorf("Set protocol mode ARQ failed: %s", err)
	}

	if err = tnc.SetARQTimeout(DefaultARQTimeout); err != nil {
		return fmt.Errorf("Set ARQ timeout failed: %s", err)
	}

	// Not yet implemented by TNC
	/*if err = tnc.SetAutoBreak(true); err != nil {
		return fmt.Errorf("Enable autobreak failed: %s", err)
	}*/

	// The TNC should only answer inbound ARQ connect requests when
	// requested by the user.
	if err = tnc.SetListenEnabled(false); err != nil {
		return fmt.Errorf("Disable listen failed: %s", err)
	}

	return nil
}

var ErrChecksumMismatch = fmt.Errorf("Control protocol checksum mismatch")

func (tnc *TNC) runControlLoop() error {
	rd := bufio.NewReader(tnc.ctrl)

	// Read prompt so we know the TNC is ready
	if tcpConn, ok := tnc.ctrl.(net.Conn); ok {
		tcpConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	}

	if f, err := readFrame(rd); err != nil {
		return err
	} else if cf, _ := f.(cmdFrame); cf != "RDY" {
		return fmt.Errorf("Unexpected TNC prompt")
	}

	if tcpConn, ok := tnc.ctrl.(net.Conn); ok {
		tcpConn.SetReadDeadline(time.Time{})
	}

	go func() {
		for { // Handle incoming TNC data
			frame, err := readFrame(rd)
			if err != nil {
				if debugEnabled() {
					log.Println("Error reading frame: %s", err)
				}

				tnc.out <- string(cmdCRCFault)
				continue
			}

			if debugEnabled() {
				log.Println("frame", frame)
			}

			if d, ok := frame.(dFrame); ok {
				if d.ARQFrame() {
					tnc.out <- string(cmdReady) // CRC ok

					select {
					case tnc.dataIn <- d.data:
					case <-time.After(time.Minute):
						go tnc.Disconnect() // Buffer full and timeout
					}
				}

				continue
			}

			line, ok := frame.(cmdFrame)
			if !ok {
				continue // TODO: Handle IDF frame
			}

			msg := line.Parsed()

			switch msg.cmd {
			case cmdPTT:
				if tnc.ptt != nil {
					tnc.ptt.SetPTT(msg.Bool())
				}
			case cmdDisconnected:
				tnc.state = Disconnected
				tnc.eof()
			case cmdBuffer:
				tnc.data.updateBuffer(msg.value.(int))
			case cmdNewState:
				tnc.state = msg.State()

				// Close ongoing connections if the new state is Disconnected
				if msg.State() == Disconnected {
					tnc.eof()
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
	dataOut := make(chan []byte)

	tnc.out = out
	tnc.dataOut = dataOut

	go func() {
		for {
			select {
			case str, ok := <-out:
				if !ok {
					return
				}

				if debugEnabled() {
					log.Println("-->", str)
				}

				if err := writeCtrlFrame(tnc.ctrl, str); err != nil {
					panic(err)
				}
			case data, ok := <-dataOut:
				if !ok {
					return
				}

				for len(data) > 0 {
					n, err := tnc.ctrl.Write(data)
					if err != nil {
						panic(err)
					}
					data = data[n:]
				}
			}
		}
	}()
	return nil
}

func (tnc *TNC) eof() {
	if tnc.data != nil {
		close(tnc.dataIn)       // Signals EOF to pending reads
		tnc.data.signalClosed() // Signals EOF to pending writes
		tnc.connected = false   // connect() is responsible for setting it to true
		tnc.dataIn = make(chan []byte, 4096)
		tnc.data = nil
	}
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

	close(tnc.out)
	close(tnc.dataOut)

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

// Returns the grid square as reported by the TNC
func (tnc *TNC) GridSquare() (string, error) {
	return tnc.getString(cmdGridSquare)
}

// Returns mycall as reported by the TNC
func (tnc *TNC) MyCall() (string, error) {
	return tnc.getString(cmdMyCall)
}

// Autobreak returns wether or not automatic link turnover is enabled.
func (tnc *TNC) AutoBreak() (bool, error) {
	return tnc.getBool(cmdAutoBreak)
}

// SetAutoBreak Disables/enables automatic link turnover.
func (tnc *TNC) SetAutoBreak(on bool) error {
	return tnc.set(cmdAutoBreak, on)
}

// Sets the ARQ bandwidth
func (tnc *TNC) SetARQBandwidth(bw Bandwidth) error {
	return tnc.set(cmdARQBW, bw)
}

// Sets the ARQ timeout
func (tnc *TNC) SetARQTimeout(d time.Duration) error {
	return tnc.set(cmdARQTimeout, int(d/time.Second))
}

// Gets the ARQ timeout
func (tnc *TNC) ARQTimeout() (time.Duration, error) {
	seconds, err := tnc.getInt(cmdARQTimeout)
	return time.Duration(seconds) * time.Second, err
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

// Enable/disable sound card and other resources
//
// This is done automatically on Open(), users should
// normally don't do this.
func (tnc *TNC) SetCodec(state bool) error {
	return tnc.set(cmdCodec, fmt.Sprintf("%t", state))
}

// ListenState() returns a StateReceiver which can be used to get notification when the TNC state changes.
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

// Disconnect gracefully disconnects the active connection or cancels an ongoing connect.
//
// The method will block until the TNC is disconnected.
//
// If the TNC is not connecting/connected, Disconnect is
// a noop.
func (tnc *TNC) Disconnect() error {
	if tnc.Idle() {
		return nil
	}

	tnc.eof()

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

// Idle returns true if the TNC is not in a connecting or connected state.
func (tnc *TNC) Idle() bool {
	return tnc.state == Disconnected || tnc.state == Offline
}

// Abort immediately aborts an ARQ Connection or a FEC Send session.
func (tnc *TNC) Abort() error {
	return tnc.set(cmdAbort, nil)
}

func (tnc *TNC) getState() State {
	v, err := tnc.get(cmdState)
	if err != nil {
		panic(fmt.Sprintf("getState(): %s", err))
	}
	return v.(State)
}

// Sends a connect command to the TNC. Users should call Dial().
func (tnc *TNC) arqCall(targetcall string, repeat int) error {
	if !tnc.Idle() {
		return ErrConnectInProgress
	}

	r := tnc.in.Listen()
	defer r.Close()

	tnc.out <- fmt.Sprintf("%s %s %d", cmdARQCall, targetcall, repeat)
	for msg := range r.Msgs() {
		switch msg.cmd {
		case cmdFault:
			return fmt.Errorf(msg.String())
		case cmdNewState:
			if tnc.state == Disconnected {
				return ErrConnectTimeout
			}
		case cmdConnected: // TODO: Probably not what we should look for
			tnc.connected = true
			return nil
		}
	}
	return nil
}

func (tnc *TNC) set(cmd Command, param interface{}) (err error) {
	r := tnc.in.Listen()
	defer r.Close()

	if param != nil {
		tnc.out <- fmt.Sprintf("%s %v", cmd, param)
	} else {
		tnc.out <- string(cmd)
	}

	for msg := range r.Msgs() {
		if msg.cmd == cmdReady {
			return
		} else if msg.cmd == cmdFault {
			return errors.New(msg.String())
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

func (tnc *TNC) getBool(cmd Command) (bool, error) {
	v, err := tnc.get(cmd)
	if err != nil {
		return false, nil
	}
	return v.(bool), nil
}

func (tnc *TNC) getInt(cmd Command) (int, error) {
	v, err := tnc.get(cmd)
	if err != nil {
		return 0, err
	}
	return v.(int), nil
}

func (tnc *TNC) get(cmd Command) (value interface{}, err error) {
	r := tnc.in.Listen()
	defer r.Close()

	tnc.out <- string(cmd)
	for msg := range r.Msgs() {
		if msg.cmd == cmd {
			value = msg.value
		} else if msg.cmd == cmdFault {
			err = errors.New(msg.String())
		} else if msg.cmd == cmdReady {
			return
		}
	}
	return nil, errors.New("TNC hung up")
}
