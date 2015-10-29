// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ax25

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/la5nta/wl2k-go"
	"github.com/tarm/goserial"
)

// KenwoodConn implements net.Conn using a
// Kenwood (or similar) TNC in connected transparent mode.
//
// Tested with Kenwood TH-D72 and TM-D710 in "packet-mode".
//
// TODO: github.com/term/goserial does not support setting the
// line flow control. Thus, KenwoodConn is not suitable for
// sending messages > the TNC's internal buffer size.
//
// We should probably be using software flow control (XFLOW),
// as hardware flow is not supported by many USB->RS232 adapters
// including the adapter build into TH-D72 (at least, not using the
// current linux kernel module.
type KenwoodConn struct{ Conn }

// Dial a packet node using a Kenwood (or similar) radio over serial
func DialKenwood(dev, mycall, targetcall string, config Config, logger *log.Logger) (*KenwoodConn, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	localAddr, remoteAddr := tncAddrFromString(mycall), tncAddrFromString(targetcall)
	conn := &KenwoodConn{Conn{
		localAddr:  AX25Addr{localAddr},
		remoteAddr: AX25Addr{remoteAddr},
	}}

	if dev == "socket" {
		c, err := net.Dial("tcp", "127.0.0.1:8081")
		if err != nil {
			panic(err)
		}
		conn.Conn.ReadWriteCloser = c
	} else {
		c := &serial.Config{Name: dev, Baud: int(B9600)}
		s, err := serial.OpenPort(c)
		if err != nil {
			return conn, err
		} else {
			conn.Conn.ReadWriteCloser = s
		}
	}

	conn.Write([]byte{3, 3, 3}) // ETX
	fmt.Fprint(conn, "\r\nrestart\r\n")
	for {
		line, _ := wl2k.ReadLine(conn)

		if strings.HasPrefix(line, "cmd:") {
			fmt.Fprint(conn, "ECHO OFF\r") // Don't echo commands
			fmt.Fprint(conn, "FLOW OFF\r")
			fmt.Fprint(conn, "XFLOW ON\r")    // Enable software flow control
			fmt.Fprint(conn, "LFIGNORE ON\r") // Ignore linefeed (\n)
			fmt.Fprint(conn, "AUTOLF OFF\r")  // Don't auto-insert linefeed
			fmt.Fprint(conn, "CR ON\r")
			fmt.Fprint(conn, "8BITCONV ON\r") // Use 8-bit characters

			// Return to command mode if station of current I/O stream disconnects.
			fmt.Fprint(conn, "NEWMODE ON\r")

			time.Sleep(500 * time.Millisecond)

			fmt.Fprintf(conn, "MYCALL %s\r", mycall)
			fmt.Fprintf(conn, "HBAUD %d\r", config.HBaud)
			fmt.Fprintf(conn, "PACLEN %d\r", config.PacketLength)
			fmt.Fprintf(conn, "TXDELAY %d\r", config.TXDelay/_CONFIG_TXDELAY_UNIT)
			fmt.Fprintf(conn, "PERSIST %d\r", config.Persist)
			time.Sleep(500 * time.Millisecond)

			fmt.Fprintf(conn, "SLOTTIME %d\r", config.SlotTime/_CONFIG_SLOT_TIME_UNIT)
			fmt.Fprint(conn, "FULLDUP OFF\r")
			fmt.Fprintf(conn, "MAXFRAME %d\r", config.MaxFrame)
			fmt.Fprintf(conn, "FRACK %d\r", config.FRACK/_CONFIG_FRACK_UNIT)
			fmt.Fprintf(conn, "RESPTIME %d\r", config.ResponseTime/_CONFIG_RESPONSE_TIME_UNIT)
			fmt.Fprintf(conn, "NOMODE ON\r")

			break
		}
	}
	time.Sleep(2 * time.Second)

	fmt.Fprintf(conn, "\rc %s\r", targetcall)
	for {
		line, _ := wl2k.ReadLine(conn)
		logger.Println(line)
		line = strings.TrimSpace(line)

		if strings.Contains(line, "*** CONNECTED to") {
			fmt.Fprint(conn, "TRANS\r\n")
			return conn, nil
		} else if strings.Contains(line, "*** DISCONNECTED") {
			logger.Fatal("got disconnect ", int(line[len(line)-1]))
		}
	}

	return conn, nil
}

func (c *KenwoodConn) Close() error {
	if !c.ok() {
		return syscall.EINVAL
	}

	// Exit TRANS mode
	time.Sleep(1 * time.Second)
	for i := 0; i < 3; i++ {
		c.Write([]byte{3}) // ETX
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for prompt
	time.Sleep(1 * time.Second)

	// Disconnect
	fmt.Fprint(c, "\r\nD\r\n")
	for {
		line, _ := wl2k.ReadLine(c)
		if strings.Contains(line, `DISCONN`) {
			log.Println(`Disconnected`)
			break
		}
	}
	return c.Conn.Close()
}
