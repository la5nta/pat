// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build libax25

package ax25

//#include <sys/socket.h>
import "C"

import (
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"
)

func NewAX25Beacon(axPort, mycall, dest, message string) (Beacon, error) {
	if _, err := loadPorts(); err != nil {
		return nil, err
	}

	localAddr := newAX25Addr(mycall)
	if err := localAddr.setPort(axPort); err != nil {
		return nil, err
	}
	remoteAddr := newAX25Addr(dest)

	return &ax25Beacon{localAddr, remoteAddr, message}, nil
}

type ax25Beacon struct {
	localAddr  ax25Addr
	remoteAddr ax25Addr
	message    string
}

func (b *ax25Beacon) Message() string      { return b.message }
func (b *ax25Beacon) LocalAddr() net.Addr  { return AX25Addr{b.localAddr} }
func (b *ax25Beacon) RemoteAddr() net.Addr { return AX25Addr{b.remoteAddr} }

func (b *ax25Beacon) Every(d time.Duration) error {
	for {
		if err := b.Now(); err != nil {
			return err
		}
		time.Sleep(d)
	}
}

func (b *ax25Beacon) Now() error {
	// Create file descriptor
	//REVIEW: Should we keep it for next beacon?
	var socket fd
	if f, err := syscall.Socket(syscall.AF_AX25, syscall.SOCK_DGRAM, 0); err != nil {
		return err
	} else {
		socket = fd(f)
	}
	defer socket.close()

	if err := socket.bind(b.localAddr); err != nil {
		return fmt.Errorf("bind: %s", err)
	}

	msg := C.CString(b.message)
	_, err := C.sendto(
		C.int(socket),
		unsafe.Pointer(msg),
		C.size_t(len(b.message)),
		0,
		(*C.struct_sockaddr)(unsafe.Pointer(&b.remoteAddr)),
		C.socklen_t(unsafe.Sizeof(b.remoteAddr)),
	)

	return err
}
