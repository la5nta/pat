package app

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Modem interface {
	io.Closer
	Listen() (net.Listener, error)
	Ping() error
}

type ModemOpener func() (Modem, error)

var modemGracePeriod = 10 * time.Second

var errModemHubClosed = errors.New("modem hub closed")

type ModemHub struct {
	mu     sync.Mutex
	slots  map[string]*modemSlot
	closed bool
}

type modemSlot struct {
	open  ModemOpener
	init  sync.Mutex // serializes open/ping per scheme, so only one open runs at a time
	m     Modem
	holds int
	timer *time.Timer
}

func NewModemHub(openers map[string]ModemOpener) *ModemHub {
	slots := make(map[string]*modemSlot, len(openers))
	for scheme, open := range openers {
		slots[scheme] = &modemSlot{open: open}
	}
	return &ModemHub{slots: slots}
}

func (h *ModemHub) Acquire(scheme string) (Modem, func(), error) {
	s, ok := h.slots[scheme]
	if !ok {
		return nil, nil, errors.New("no modem for scheme " + scheme)
	}
	s.init.Lock()
	defer s.init.Unlock()

	h.mu.Lock()
	m, closed := s.m, h.closed
	h.mu.Unlock()
	if closed {
		return nil, nil, errModemHubClosed
	}

	if m != nil {
		if err := m.Ping(); err != nil {
			log.Printf("%s modem unresponsive, reopening: %s", scheme, err)
			h.mu.Lock()
			h.drop(s)
			h.mu.Unlock()
			m.Close()
			m = nil
		}
	}
	if m == nil {
		var err error
		if m, err = s.open(); err != nil {
			return nil, nil, err
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		m.Close()
		return nil, nil, errModemHubClosed
	}
	s.m = m
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.holds++
	return m, sync.OnceFunc(func() { h.release(s, m) }), nil
}

func (h *ModemHub) release(s *modemSlot, m Modem) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.m != m {
		return
	}
	if s.holds--; s.holds > 0 || h.closed {
		return
	}
	var t *time.Timer
	t = time.AfterFunc(modemGracePeriod, func() {
		s.init.Lock() // Wait out any in-flight Acquire, which may re-hold m.
		defer s.init.Unlock()
		h.mu.Lock()
		defer h.mu.Unlock()
		if s.timer == t { // Not cancelled or superseded meanwhile.
			h.drop(s)
			m.Close()
		}
	})
	s.timer = t
}

func (h *ModemHub) drop(s *modemSlot) {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.m, s.holds = nil, 0
}

func (h *ModemHub) Current(scheme string) Modem {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.slots[scheme]; ok {
		return s.m
	}
	return nil
}

func (h *ModemHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for scheme, s := range h.slots {
		if s.m == nil {
			continue
		}
		if err := s.m.Close(); err != nil {
			log.Printf("Failure to close %s modem: %s", scheme, err)
		}
		h.drop(s)
	}
}

type releasingListener struct {
	net.Listener
	release func()
}

func (l releasingListener) Close() error {
	defer l.release()
	return l.Listener.Close()
}

func (a *App) listenModem(scheme string) (Modem, net.Listener, error) {
	m, release, err := a.modems.Acquire(scheme)
	if err != nil {
		return nil, nil, err
	}
	ln, err := m.Listen()
	if err != nil {
		release()
		return nil, nil, err
	}
	return m, releasingListener{ln, release}, nil
}
