package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/la5nta/wl2k-go/transport"
)

type Modem interface {
	io.Closer
	Listen() (net.Listener, error)
	Ping() error
	DialURLContext(context.Context, *transport.URL) (net.Conn, error)
}

type ModemOpener func() (Modem, error)

var (
	modemGracePeriod = 10 * time.Second
	modemOpenTimeout = 15 * time.Second
	modemPingTimeout = 3 * time.Second
)

var errModemHubClosed = errors.New("modem hub closed")

type ModemHub struct {
	mu     sync.Mutex
	slots  map[string]*modemSlot
	closed bool
}

type modemSlot struct {
	open  ModemOpener
	busy  chan struct{} // 1-buffered semaphore serializing open/ping per scheme
	m     Modem
	gen   uint64 // bumped on drop; identifies m without comparing interfaces
	holds int
	timer *time.Timer
}

func NewModemHub(openers map[string]ModemOpener) *ModemHub {
	slots := make(map[string]*modemSlot, len(openers))
	for scheme, open := range openers {
		slots[scheme] = &modemSlot{open: open, busy: make(chan struct{}, 1)}
	}
	return &ModemHub{slots: slots}
}

func (h *ModemHub) Acquire(ctx context.Context, scheme string) (Modem, func(), error) {
	s, ok := h.slots[scheme]
	if !ok {
		return nil, nil, errors.New("no modem for scheme " + scheme)
	}
	select {
	case s.busy <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	defer func() { <-s.busy }()

	h.mu.Lock()
	m, closed := s.m, h.closed
	h.mu.Unlock()
	if closed {
		return nil, nil, errModemHubClosed
	}

	if m != nil {
		pinged := m // m is reassigned below.
		ping := func() (struct{}, error) { return struct{}{}, pinged.Ping() }
		if _, err := within(ctx, modemPingTimeout, ping, func(struct{}, error) {}); err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err() // Caller gave up; the modem may be fine.
			}
			log.Printf("%s modem unresponsive, reopening: %s", scheme, err)
			h.mu.Lock()
			h.drop(s)
			h.mu.Unlock()
			go m.Close() // May hang on a wedged TNC.
			m = nil
		}
	}
	if m == nil {
		var err error
		m, err = within(ctx, modemOpenTimeout, s.open, func(m Modem, err error) {
			if err == nil {
				m.Close()
			}
		})
		if err != nil {
			return nil, nil, fmt.Errorf("open %s modem: %w", scheme, err)
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
	gen := s.gen
	return m, sync.OnceFunc(func() { h.release(s, m, gen) }), nil
}

// within runs f bounded by ctx and d. If abandoned, late gets f's eventual result.
func within[T any](ctx context.Context, d time.Duration, f func() (T, error), late func(T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	type result struct {
		v   T
		err error
	}
	ch := make(chan result)
	go func() {
		v, err := f()
		select {
		case ch <- result{v, err}:
		case <-ctx.Done():
			late(v, err)
		}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func (h *ModemHub) release(s *modemSlot, m Modem, gen uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.gen != gen || s.m == nil {
		return
	}
	if s.holds--; s.holds > 0 {
		return
	}
	var t *time.Timer
	t = time.AfterFunc(modemGracePeriod, func() {
		s.busy <- struct{}{} // Wait out in-flight Acquire.
		defer func() { <-s.busy }()
		h.mu.Lock()
		defer h.mu.Unlock()
		if s.timer == t { // Not cancelled or superseded meanwhile.
			h.drop(s)
			go m.Close()
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
	s.gen++
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
	h.closed = true
	open := make(map[string]Modem)
	for scheme, s := range h.slots {
		if s.m != nil {
			open[scheme] = s.m
			h.drop(s)
		}
	}
	h.mu.Unlock()
	for scheme, m := range open {
		if err := m.Close(); err != nil {
			log.Printf("Failure to close %s modem: %s", scheme, err)
		}
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
	m, release, err := a.modems.Acquire(context.Background(), scheme)
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
