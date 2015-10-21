// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"errors"
	"fmt"
	"io"
	"net"
)

var ErrActiveListenerExists error = errors.New("An active listener is already registered with this TNC.")

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
		return nil, err
	}
}

func (l listener) Addr() net.Addr {
	return l.addr
}

func (l listener) Close() error {
	close(l.quit)
	return nil
}

func (tnc *TNC) Listen() (ln net.Listener, err error) {
	if tnc.listenerActive {
		return nil, ErrActiveListenerExists
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
				case msg.cmd == cmdCancelPending:
					remotecall, targetcall = "", ""
				case msg.cmd == cmdConnected:
					remotecall = msg.value.([]string)[0]
				case msg.cmd == cmdTarget:
					targetcall = msg.String()
				}

				if len(remotecall) > 0 && len(targetcall) > 0 {
					tnc.data = &tncConn{
						remoteAddr: Addr{remotecall},
						localAddr:  Addr{targetcall},
						ctrlOut:    tnc.out,
						dataOut:    tnc.dataOut,
						ctrlIn:     tnc.in,
						dataIn:     tnc.dataIn,
						eofChan:    make(chan struct{}),
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
