// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build libax25

package ax25

/*
#cgo LDFLAGS: -lax25
#include <sys/socket.h>
#include <netax25/ax25.h>
#include <netax25/axlib.h>
#include <netax25/axconfig.h>
#include <fcntl.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"
)

type ax25Addr C.struct_full_sockaddr_ax25

var numAXPorts int

// bug(martinhpedersen): The AX.25 stack does not support SOCK_STREAM, so any write to the connection
// that is larger than maximum packet length will fail. The b2f impl. requires 125 bytes long packets.
var ErrMessageTooLong = errors.New("Write: Message too long. Consider increasing maximum packet length to >= 125.")

type fd uintptr

type ax25Listener struct {
	sock      fd
	localAddr AX25Addr
}

func loadPorts() (int, error) {
	if numAXPorts > 0 {
		return numAXPorts, nil
	}

	n, err := C.ax25_config_load_ports()
	if err != nil {
		return int(n), err
	} else if n == 0 {
		return 0, fmt.Errorf("No AX.25 ports configured")
	}

	numAXPorts = int(n)
	return numAXPorts, err
}

// Addr returns the listener's network address, an AX25Addr.
func (ln ax25Listener) Addr() net.Addr { return ln.localAddr }

// Close stops listening on the AX.25 port. Already Accepted connections are not closed.
func (ln ax25Listener) Close() error { return ln.sock.close() } //TODO: Should make sure any Accept() calls returns with an error!

// Accept waits for the next call and returns a generic Conn.
//
// See net.Listener for more information.
func (ln ax25Listener) Accept() (net.Conn, error) {
	nfd, addr, err := ln.sock.accept()
	if err != nil {
		return nil, err
	}

	conn := &Conn{
		localAddr:       ln.localAddr,
		remoteAddr:      AX25Addr{addr},
		ReadWriteCloser: os.NewFile(uintptr(nfd), ""),
	}

	return conn, nil
}

// ListenAX25 announces on the local port axPort using mycall as the local address.
func ListenAX25(axPort, mycall string) (net.Listener, error) {
	if _, err := loadPorts(); err != nil {
		return nil, err
	}

	// Setup local address (via callsign of supplied axPort)
	localAddr := newAX25Addr(mycall)
	if err := localAddr.setPort(axPort); err != nil {
		return nil, err
	}

	// Create file descriptor
	var socket fd
	if f, err := syscall.Socket(syscall.AF_AX25, syscall.SOCK_SEQPACKET, 0); err != nil {
		return nil, err
	} else {
		socket = fd(f)
	}

	if err := socket.bind(localAddr); err != nil {
		return nil, err
	}
	if err := syscall.Listen(int(socket), syscall.SOMAXCONN); err != nil {
		return nil, err
	}

	return ax25Listener{
		sock:      fd(socket),
		localAddr: AX25Addr{localAddr},
	}, nil
}

// DialAX25Timeout acts like DialAX25 but takes a timeout.
func DialAX25Timeout(axPort, mycall, targetcall string, timeout time.Duration) (*Conn, error) {
	if _, err := loadPorts(); err != nil {
		return nil, err
	}

	// Setup local address (via callsign of supplied axPort)
	localAddr := newAX25Addr(mycall)
	if err := localAddr.setPort(axPort); err != nil {
		return nil, err
	}
	remoteAddr := newAX25Addr(targetcall)

	// Create file descriptor
	var socket fd
	if f, err := syscall.Socket(syscall.AF_AX25, syscall.SOCK_SEQPACKET, 0); err != nil {
		return nil, err
	} else {
		socket = fd(f)
	}

	// Bind
	if err := socket.bind(localAddr); err != nil {
		return nil, err
	}

	// Connect
	err := socket.connectTimeout(remoteAddr, timeout)
	if err != nil {
		socket.close()
		return nil, err
	}

	return &Conn{
		ReadWriteCloser: os.NewFile(uintptr(socket), axPort),
		localAddr:       AX25Addr{localAddr},
		remoteAddr:      AX25Addr{remoteAddr},
	}, nil
}

func (c *Conn) Close() error {
	if !c.ok() {
		return syscall.EINVAL
	}

	return c.ReadWriteCloser.Close()
}

func (c *Conn) Write(p []byte) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}

	n, err = c.ReadWriteCloser.Write(p)
	perr, ok := err.(*os.PathError)
	if !ok {
		return
	}

	switch perr.Err.Error() {
	case "message too long":
		return n, ErrMessageTooLong
	default:
		return
	}
}

func (c *Conn) Read(p []byte) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}

	n, err = c.ReadWriteCloser.Read(p)
	perr, ok := err.(*os.PathError)
	if !ok {
		return
	}

	//TODO: These errors should not be checked using string comparison!
	// The weird error handling here is needed because of how the *os.File treats
	// the underlying fd. This should be fixed the same way as net.FileConn does.
	switch perr.Err.Error() {
	case "transport endpoint is not connected": // We get this error when the remote hangs up
		return n, io.EOF
	default:
		return
	}
}

// DialAX25 connects to the remote station targetcall using the named axport and mycall.
func DialAX25(axPort, mycall, targetcall string) (*Conn, error) {
	return DialAX25Timeout(axPort, mycall, targetcall, 0)
}

func (sock fd) connectTimeout(addr ax25Addr, timeout time.Duration) (err error) {
	if timeout == 0 {
		return sock.connect(addr)
	}
	if err = syscall.SetNonblock(int(sock), true); err != nil {
		return err
	}

	err = sock.connect(addr)
	if err == nil {
		return nil // Connected
	} else if err != syscall.EINPROGRESS {
		return fmt.Errorf("Unable to connect: %s", err)
	}

	// Shamelessly stolen from src/pkg/exp/inotify/inotify_linux.go:
	//
	// Create fdSet, taking into consideration that
	// 64-bit OS uses Bits: [16]int64, while 32-bit OS uses Bits: [32]int32.
	// This only support File Descriptors up to 1024
	//
	if sock > 1024 {
		panic(fmt.Errorf("connectTimeout: File Descriptor >= 1024: %v", sock))
	}
	fdset := new(syscall.FdSet)
	fElemSize := 32 * 32 / len(fdset.Bits)
	fdset.Bits[int(sock)/fElemSize] |= 1 << uint(int(sock)%fElemSize)
	//
	// Thanks!
	//

	// Wait or timeout
	var n int
	var tv syscall.Timeval
	for {
		tv = syscall.NsecToTimeval(int64(timeout))
		n, err = syscall.Select(int(sock)+1, nil, fdset, nil, &tv)
		if n < 0 && err != syscall.EINTR {
			return fmt.Errorf("Unable to connect: %s", err)
		} else if n > 0 {
			/* TODO: verify that connection is OK
			 * lon = sizeof(int);
			 * if (getsockopt(soc, SOL_SOCKET, SO_ERROR, (void*)(&valopt), &lon) < 0) {
			 *   fprintf(stderr, "Error in getsockopt() %d - %s\n", errno, strerror(errno));
			 *   exit(0);
			 * }
			 * // Check the value returned...
			 * if (valopt) {
			 *   fprintf(stderr, "Error in delayed connection() %d - %s\n", valopt, strerror(valopt));
			 *   exit(0);
			 * }
			 */
			break
		} else {
			return fmt.Errorf("Unable to connect: timeout")
		}
	}

	syscall.SetNonblock(int(sock), false)
	return
}

func (sock fd) close() error {
	return syscall.Close(int(sock))
}

func (sock fd) accept() (nfd fd, addr ax25Addr, err error) {
	addrLen := C.socklen_t(unsafe.Sizeof(addr))

	n, err := C.accept(
		C.int(sock),
		(*C.struct_sockaddr)(unsafe.Pointer(&addr)),
		&addrLen)

	if addrLen != C.socklen_t(unsafe.Sizeof(addr)) {
		panic("unexpected socklet_t")
	}

	return fd(n), addr, err
}

func (sock fd) connect(addr ax25Addr) (err error) {
	_, err = C.connect(
		C.int(sock),
		(*C.struct_sockaddr)(unsafe.Pointer(&addr)),
		C.socklen_t(unsafe.Sizeof(addr)))

	return
}

func (sock fd) bind(addr ax25Addr) (err error) {
	_, err = C.bind(
		C.int(sock),
		(*C.struct_sockaddr)(unsafe.Pointer(&addr)),
		C.socklen_t(unsafe.Sizeof(addr)))

	return
}

type ax25_address *C.ax25_address

func (a ax25Addr) Address() Address {
	return AddressFromString(
		C.GoString(C.ax25_ntoa(a.ax25_address())),
	)
}

func (a ax25Addr) Digis() []Address {
	digis := make([]Address, a.numDigis())
	for i, digi := range a.digis() {
		digis[i] = AddressFromString(C.GoString(C.ax25_ntoa(digi)))
	}
	return digis
}

func (a *ax25Addr) numDigis() int {
	return int(a.fsa_ax25.sax25_ndigis)
}

func (a *ax25Addr) digis() []ax25_address {
	digis := make([]ax25_address, a.numDigis())
	for i, _ := range digis {
		digis[i] = (*C.ax25_address)(unsafe.Pointer(&a.fsa_digipeater[i]))
	}
	return digis
}

func (a *ax25Addr) ax25_address() ax25_address {
	return (*C.ax25_address)(unsafe.Pointer(&a.fsa_ax25.sax25_call.ax25_call))
}

func (a *ax25Addr) setPort(port string) (err error) {
	C.ax25_aton_entry(
		C.ax25_config_get_addr(C.CString(port)),
		&a.fsa_digipeater[0].ax25_call[0],
	)
	a.fsa_ax25.sax25_ndigis = 1
	return
}

func newAX25Addr(address string) ax25Addr {
	var addr C.struct_full_sockaddr_ax25

	if C.ax25_aton(C.CString(address), &addr) < 0 {
		panic("ax25_aton")
	}
	addr.fsa_ax25.sax25_family = syscall.AF_AX25

	return ax25Addr(addr)
}
