// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"fmt"
	"net"
)

func (tnc *TNC) Dial(targetcall string) (net.Conn, error) {
	if err := tnc.arqCall(targetcall, 10); err != nil {
		return nil, err
	}

	mycall, err := tnc.MyCall()
	if err != nil {
		return nil, fmt.Errorf("Error when getting mycall: %s", err)
	}

	tnc.data = &tncConn{
		remoteAddr: Addr{targetcall},
		localAddr:  Addr{mycall},
		ctrlOut:    tnc.out,
		dataOut:    tnc.dataOut,
		ctrlIn:     tnc.in,
		dataIn:     tnc.dataIn,
		eofChan:    make(chan struct{}),
	}

	return tnc.data, nil
}
