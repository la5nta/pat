// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

var ErrDisconnectTimeout = fmt.Errorf("Disconnect timeout: aborted connection.")

type tncConn struct {
	dataLock sync.Mutex
	ctrlOut  chan<- string
	dataOut  chan<- []byte
	dataIn   <-chan []byte
	eofChan  chan struct{}
	ctrlIn   broadcaster

	remoteAddr Addr
	localAddr  Addr

	// The flushLock is used to keep track of the "out queued" buffer.
	//
	// It is locked on write, and Flush() will block until it's unlocked.
	// It is the control loop's responsibility to unlock this lock when buffer reached zero.
	flushLock lock

	mu       sync.Mutex
	buffer   int
	nWritten int
}

//TODO: implement
func (conn *tncConn) SetDeadline(t time.Time) error      { return nil }
func (conn *tncConn) SetReadDeadline(t time.Time) error  { return nil }
func (conn *tncConn) SetWriteDeadline(t time.Time) error { return nil }

func (conn *tncConn) RemoteAddr() net.Addr { return conn.remoteAddr }
func (conn *tncConn) LocalAddr() net.Addr  { return conn.localAddr }

func (conn *tncConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	data, ok := <-conn.dataIn
	if !ok {
		return 0, io.EOF
	}

	for i, b := range data {
		p[i] = b //TODO: Handle too small buffer. Will panic now.
	}
	return len(data), nil
}

func (conn *tncConn) Write(p []byte) (int, error) {
	conn.dataLock.Lock()
	defer conn.dataLock.Unlock()

	if len(p) > 65535 { // uint16 max
		p = p[0 : 65535-1]
	}

	//"D:" + 2 byte count big endian + binary data + 2 byte CRC
	var buf bytes.Buffer

	fmt.Fprint(&buf, "D:")

	if err := binary.Write(&buf, binary.BigEndian, uint16(len(p))); err != nil {
		return 0, err
	}

	n, _ := buf.Write(p)

	sum := crc16Sum(buf.Bytes()[2:])
	if err := binary.Write(&buf, binary.BigEndian, sum); err != nil {
		return 0, err
	}

	r := conn.ctrlIn.Listen()
	defer r.Close()

L:
	for i := 0; ; i++ {
		if i == 3 {
			return 0, fmt.Errorf("CRC failure")
		}

		conn.dataOut <- buf.Bytes()
		for {
			select {
			case msg := <-r.Msgs():
				if msg.cmd == cmdReady {
					conn.mu.Lock()
					conn.nWritten += n
					conn.mu.Unlock()
				} else if msg.cmd == cmdBuffer {
					conn.flushLock.Lock()
					break L // Wait until we get a buffer update before returning
				} else if msg.cmd == cmdCRCFault {
					continue L
				}
			case <-conn.eofChan:
				return n, io.EOF
			}
		}
	}

	return n, nil
}

func (conn *tncConn) Flush() error {
	select {
	case <-conn.flushLock.WaitChan():
		return nil
	case <-conn.eofChan:
		return io.EOF
	}
	panic("not happening!")
}

func (conn *tncConn) signalClosed() { close(conn.eofChan) }

const flushAndCloseTimeout = 30 * time.Second //TODO: Remove when time is right (see Close).

// Close closes the current connection.
//
// Will abort ("dirty disconnect") after 30 seconds if normal "disconnect" have not succeeded yet.
func (conn *tncConn) Close() error {
	if conn == nil {
		return nil
	}

	// Flush: (THIS WILL PROBABLY BE REMOVED WHEN ARDOP MATURES)
	// We have to flush, because ardop will disconnect without waiting for the last
	// data in buffer to be sent.
	//
	// We also need to timeout the flush, because ardop does not seem to switch from IRS to ISS
	// if we only write one simple line (*** error line). (autobreak).
	select {
	case <-conn.flushLock.WaitChan():
	case <-time.After(flushAndCloseTimeout):
	}

	r := conn.ctrlIn.Listen()
	defer r.Close()

	conn.ctrlOut <- string(cmdDisconnect)
	timeout := time.After(flushAndCloseTimeout)
	for {
		select {
		case msg, ok := <-r.Msgs(): // Wait for TNC to disconnect
			if !ok {
				return errors.New("TNC hung up while waiting for requested disconnect")
			}

			if msg.cmd == cmdReady {
				// The command echo
			}
			if msg.cmd == cmdDisconnected || (msg.cmd == cmdNewState && msg.State() == Disconnected) {
				// The control loop have already closed the data connection
				return nil
			}
		case <-timeout:
			conn.ctrlOut <- string(cmdAbort)
			return ErrDisconnectTimeout
		}
	}
	return nil
}

// TxBufferLen returns the number of bytes in the out buffer queue.
func (conn *tncConn) TxBufferLen() int {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	//TODO: We don't use BufferOutQueued, because it may be outdated (not updated since last Write call).

	return conn.buffer
}

func (conn *tncConn) updateBuffer(b int) {
	if conn == nil {
		return
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.buffer = b

	if b == 0 {
		conn.flushLock.Unlock()
	}
}
