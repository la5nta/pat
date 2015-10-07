// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

var (
	ErrInvalidBandwidth     error = errors.New("Invalid bandwidth. Supported values are 500 or 1600.")
	ErrActiveListenerExists error = errors.New("An active listener is already registered with this TNC.")
)

type listener struct {
	incoming <-chan net.Conn
	quit     chan struct{}
	errors   <-chan error
	addr     Addr
}

func (l listener) Accept() (c net.Conn, err error) {
	select {
	case c, ok := <-l.incoming:
		if !ok {
			return nil, io.EOF
		}
		return c, nil
	case err = <-l.errors:
		return
	}
}

func (l listener) Addr() net.Addr {
	return l.addr
}

func (l listener) Close() error {
	close(l.quit)
	return nil
}

func (tnc *TNC) Listen(bandwidth int) (ln net.Listener, err error) {
	if tnc.listenerActive {
		return nil, ErrActiveListenerExists
	} else if bandwidth != 500 && bandwidth != 1600 {
		return nil, ErrInvalidBandwidth
	}
	tnc.listenerActive = true

	incoming := make(chan net.Conn)
	quit := make(chan struct{})
	errors := make(chan error)

	mycall, err := tnc.MyCall()
	if err != nil {
		return nil, fmt.Errorf("Unable to get mycall: %s", err)
	}

	if err := tnc.SetListenEnabled(true); err != nil {
		return nil, fmt.Errorf("TNC failed to enable listening: %s", err)
	}

	go func() {
		defer func() {
			close(incoming) // Important to close this first!
			close(errors)
			tnc.listenerActive = false
		}()

		msgListener := tnc.in.Listen()
		msgs := msgListener.Msgs()

		var remotecall, targetcall string
		for {
			select {
			case <-quit:
				tnc.SetListenEnabled(false) // Should return this in listener.Close()
				return
			case msg, ok := <-msgs:
				switch {
				case !ok:
					errors <- fmt.Errorf("Lost connection to the TNC")
					return
				case msg.cmd == cmdNewState && msg.State() == ConnectPending:
					remotecall, targetcall = "", ""
					if err := tnc.set(cmdBandwidth, bandwidth); err != nil {
						errors <- err
					}
				case msg.cmd == cmdConnected:
					remotecall = msg.String()
				case msg.cmd == cmdTarget:
					targetcall = msg.String()
				}

				if len(remotecall) > 0 && len(targetcall) > 0 {
					// Give TNC time to listen on data port
					time.Sleep(200 * time.Millisecond)

					dataConn, err := net.Dial("tcp", tnc.connAddr)
					if err != nil {
						errors <- err
						remotecall, targetcall = "", ""
						continue
					}

					dataConn.(*net.TCPConn).SetReadBuffer(0)
					dataConn.(*net.TCPConn).SetWriteBuffer(0)

					tnc.data = &tncConn{
						Conn:       dataConn,
						remoteAddr: Addr{remotecall},
						localAddr:  Addr{targetcall},
						ctrlOut:    tnc.out,
						ctrlIn:     tnc.in,
					}
					tnc.connected = true

					incoming <- tnc.data

					remotecall, targetcall = "", ""
				}
			}
		}
	}()

	return listener{incoming, quit, errors, Addr{mycall}}, nil
}
