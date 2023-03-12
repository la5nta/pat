// Copyright 2017 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"log"
	"net"
	"sync"
	"time"
)

type TransportListener interface {
	Init() (net.Listener, error)
	Name() string
	CurrentFreq() (Frequency, bool)
}

type Beaconer interface {
	BeaconStop()
	BeaconStart() error
}

type Listener struct {
	t   TransportListener
	hub *ListenerHub

	mu       sync.Mutex
	isClosed bool
	err      error
	ln       net.Listener
}

func NewListener(t TransportListener) *Listener { return &Listener{t: t} }

func (l *Listener) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.isClosed {
		return l.err
	}
	l.isClosed = true

	// If l.err is not nil, then the last attempt to open the listener failed and we don't have anything to close
	if l.err != nil {
		return l.err
	}
	return l.ln.Close()
}

func (l *Listener) listenLoop() {
	var silenceErr bool
	for {
		l.mu.Lock()
		if l.isClosed {
			l.mu.Unlock()
			break
		}

		// Try to init the TNC
		l.ln, l.err = l.t.Init()
		if l.err != nil {
			l.mu.Unlock()
			if !silenceErr {
				log.Printf("Listener %s failed: %s", l.t.Name(), l.err)
				log.Printf("Will try to re-establish listener in the background...")
				silenceErr = true
				websocketHub.UpdateStatus()
			}
			time.Sleep(time.Second)
			continue
		}
		l.mu.Unlock()
		if silenceErr {
			log.Printf("Listener %s re-established", l.t.Name())
			silenceErr = false
			websocketHub.UpdateStatus()
		}

		if b, ok := l.t.(Beaconer); ok {
			b.BeaconStart()
		}

		// Run the accept loop until an error occurs
		if err := l.acceptLoop(); err != nil {
			log.Printf("Accept %s failed: %s", l.t.Name(), err)
		}

		if b, ok := l.t.(Beaconer); ok {
			b.BeaconStop()
		}
	}
}

type RemoteCaller interface {
	RemoteCall() string
}

func (l *Listener) acceptLoop() error {
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return err
		}

		remoteCall := conn.RemoteAddr().String()
		if c, ok := conn.(RemoteCaller); ok {
			remoteCall = c.RemoteCall()
		}

		freq, _ := l.t.CurrentFreq()

		eventLog.LogConn("accept", freq, conn, nil)
		log.Printf("Got connect (%s:%s)", l.t.Name(), remoteCall)

		err = exchange(conn, remoteCall, true)
		if err != nil {
			log.Printf("Exchange failed: %s", err)
		} else {
			log.Println("Disconnected.")
		}
	}
}

type ListenerHub struct {
	mu        sync.Mutex
	listeners map[string]*Listener
}

func NewListenerHub() *ListenerHub {
	return &ListenerHub{
		listeners: map[string]*Listener{},
	}
}

func (h *ListenerHub) Active() []TransportListener {
	h.mu.Lock()
	defer h.mu.Unlock()

	slice := make([]TransportListener, 0, len(h.listeners))
	for _, l := range h.listeners {
		if l.Err() != nil {
			continue
		}
		slice = append(slice, l.t)
	}
	return slice
}

func (h *ListenerHub) Enable(t TransportListener) {
	h.mu.Lock()
	defer func() {
		h.mu.Unlock()
		websocketHub.UpdateStatus()
	}()
	l := NewListener(t)
	if _, ok := h.listeners[t.Name()]; ok {
		return
	}
	h.listeners[t.Name()] = l
	go l.listenLoop()
}

func (h *ListenerHub) Disable(name string) (bool, error) {
	if name == MethodAX25 {
		name = defaultAX25Method()
	}
	h.mu.Lock()
	defer func() {
		h.mu.Unlock()
		websocketHub.UpdateStatus()
	}()
	l, ok := h.listeners[name]
	if !ok {
		return false, nil
	}
	delete(h.listeners, name)
	return true, l.Close()
}

func (h *ListenerHub) Close() {
	h.mu.Lock()
	defer func() {
		h.mu.Unlock()
		websocketHub.UpdateStatus()
	}()
	for k, l := range h.listeners {
		l.Close()
		delete(h.listeners, k)
	}
}
