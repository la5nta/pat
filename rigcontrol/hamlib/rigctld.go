// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package hamlib

import (
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

const DefaultTCPAddr = "localhost:4532"

var ErrUnexpectedValue = fmt.Errorf("Unexpected value in response")

// Rig represents a receiver or tranceiver.
//
// It holds the tcp connection to the service (rigctld).
type TCPRig struct {
	mu   sync.Mutex
	conn *textproto.Conn
	addr string
}

// VFO (Variable Frequency Oscillator) represents a tunable channel,
// from the radio operator's view.
//
// Also referred to as "BAND" (A-band/B-band) by some radio manufacturers.
type tcpVFO struct{ r *TCPRig }

// OpenTCP connects to the rigctld service and returns a ready to use Rig.
//
// Caller must remember to Close the Rig after use.
func OpenTCP(addr string) (*TCPRig, error) {
	r := &TCPRig{addr: addr}
	return r, r.dial()
}

func (r *TCPRig) dial() (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn != nil {
		r.conn.Close()
	}

	r.conn, err = textproto.Dial("tcp", r.addr)

	return err
}

// Closes the connection to the Rig.
func (r *TCPRig) Close() error {
	if r.conn == nil {
		return nil
	}
	return r.conn.Close()
}

// Returns the Rig's active VFO (for control).
func (r *TCPRig) CurrentVFO() VFO { return &tcpVFO{r} }

// Gets the dial frequency for this VFO.
func (v *tcpVFO) GetFreq() (int, error) {
	resp, err := v.cmd("f")
	if err != nil {
		return -1, err
	}

	freq, err := strconv.Atoi(resp)
	if err != nil {
		return -1, err
	}

	return freq, nil
}

// Sets the dial frequency for this VFO.
func (v *tcpVFO) SetFreq(freq int) error {
	_, err := v.cmd("F %d", freq)
	return err
}

// GetPTT returns the PTT state for this VFO.
func (v *tcpVFO) GetPTT() (bool, error) {
	resp, err := v.cmd("t")
	if err != nil {
		return false, err
	}

	switch resp {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, ErrUnexpectedValue
	}
}

// Enable (or disable) PTT on this VFO.
func (v *tcpVFO) SetPTT(on bool) error {
	bInt := 0
	if on == true {
		bInt = 1
	}

	_, err := v.cmd("t %d", bInt)
	return err
}

// TODO: Move retry logic to *TCPRig
func (v *tcpVFO) cmd(format string, args ...interface{}) (string, error) {
	var err error
	var resp string

	// Retry
	for i := 0; i < 3; i++ {
		if v.r.conn == nil {
			// Try re-dialing
			if err = v.r.dial(); err != nil {
				break
			}
		}

		resp, err = v.r.cmd(format, args...)
		if err == nil {
			break
		}

		_, isNetError := err.(net.Error)
		if err == io.EOF || isNetError {
			v.r.conn = nil
		}

	}

	return resp, err
}

func (r *TCPRig) cmd(format string, args ...interface{}) (string, error) {
	id, err := r.conn.Cmd(format, args...)
	if err != nil {
		return "", err
	}

	r.conn.StartResponse(id)
	defer r.conn.EndResponse(id)

	resp, err := r.conn.ReadLine()
	if err != nil {
		return "", err
	} else if err := toError(resp); err != nil {
		return resp, err
	}

	return resp, nil
}

func toError(str string) error {
	if !strings.HasPrefix(str, "RPRT ") {
		return nil
	}

	parts := strings.SplitN(str, " ", 2)

	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	switch code {
	case 0:
		return nil
	default:
		return fmt.Errorf("code %d", code)
	}
}
