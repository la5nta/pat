// Copyright 2017 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

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
	*App
	t TransportListener

	done chan struct{}

	mu  sync.Mutex
	err error
	ln  net.Listener
}

func (h *ListenerHub) NewListener(t TransportListener) *Listener {
	return &Listener{
		App:  h.App,
		t:    t,
		done: make(chan struct{}),
	}
}

func (l *Listener) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	select {
	case <-l.done:
		return l.err
	default:
		close(l.done)
		if l.ln != nil {
			return l.ln.Close()
		}
		return l.err
	}
}

func (l *Listener) listenLoop(h *ListenerHub) {
	var silenceErr bool
	for {
		select {
		case <-l.done:
			return
		default:
			ln, err := l.t.Init()
			l.mu.Lock()
			l.ln, l.err = ln, err
			l.mu.Unlock()
			if err != nil {
				if !silenceErr {
					log.Printf("Listener %s failed: %s", l.t.Name(), err)
					log.Printf("Will try to re-establish listener in the background...")
					silenceErr = true
					h.websocketHub.UpdateStatus()
				}
				time.Sleep(time.Second)
				continue
			}
			if silenceErr {
				log.Printf("Listener %s re-established", l.t.Name())
				silenceErr = false
				h.websocketHub.UpdateStatus()
			}

			if b, ok := l.t.(Beaconer); ok {
				b.BeaconStart()
			}

			// Run the accept loop until an error occurs
			if err := l.acceptLoop(); err != nil {
				select {
				case <-l.done:
					// Ignore errors during shutdown
				default:
					log.Printf("Accept %s failed: %s", l.t.Name(), err)
				}
			}

			if b, ok := l.t.(Beaconer); ok {
				b.BeaconStop()
			}
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

		l.eventLog.LogConn("accept", freq, conn, nil)
		log.Printf("Got connect (%s:%s)", l.t.Name(), remoteCall)

		err = l.exchange(conn, remoteCall, true)
		if err != nil {
			log.Printf("Exchange failed: %s", err)
		} else {
			log.Println("Disconnected.")
		}
	}
}

type ListenerHub struct {
	*App

	mu        sync.Mutex
	listeners map[string]*Listener
}

func NewListenerHub(a *App) *ListenerHub {
	return &ListenerHub{
		App:       a,
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
		h.websocketHub.UpdateStatus()
	}()
	l := h.NewListener(t)
	if _, ok := h.listeners[t.Name()]; ok {
		return
	}
	h.listeners[t.Name()] = l
	go l.listenLoop(h)
}

func (h *ListenerHub) Disable(name string) (bool, error) {
	if name == MethodAX25 {
		name = h.defaultAX25Method()
	}
	h.mu.Lock()
	defer func() {
		h.mu.Unlock()
		h.websocketHub.UpdateStatus()
	}()
	l, ok := h.listeners[name]
	if !ok {
		return false, nil
	}
	delete(h.listeners, name)
	return true, l.Close()
}

func (h *ListenerHub) Close() error {
	h.mu.Lock()
	defer func() {
		h.mu.Unlock()
		h.websocketHub.UpdateStatus()
	}()
	for k, l := range h.listeners {
		l.Close()
		delete(h.listeners, k)
	}
	return nil
}
