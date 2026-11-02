package app

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeModem struct {
	dead   atomic.Bool
	closed atomic.Bool
}

func (m *fakeModem) Close() error                  { m.closed.Store(true); return nil }
func (m *fakeModem) Listen() (net.Listener, error) { return newMockListener(), nil }
func (m *fakeModem) Ping() error {
	if m.dead.Load() {
		return errors.New("dead")
	}
	return nil
}

type fakeOpener struct {
	opens atomic.Int32
	err   error
}

func (o *fakeOpener) open() (Modem, error) {
	o.opens.Add(1)
	time.Sleep(10 * time.Millisecond)
	if o.err != nil {
		return nil, o.err
	}
	return &fakeModem{}, nil
}

func newTestModemHub(t *testing.T) (*ModemHub, *fakeOpener) {
	old := modemGracePeriod
	modemGracePeriod = 50 * time.Millisecond
	t.Cleanup(func() { modemGracePeriod = old })
	o := &fakeOpener{}
	return NewModemHub(map[string]ModemOpener{"test": o.open}), o
}

func TestModemHub(t *testing.T) {
	t.Run("ConcurrentAcquireOpensOnce", func(t *testing.T) {
		h, o := newTestModemHub(t)
		defer h.Close()
		var wg sync.WaitGroup
		for range 5 {
			wg.Go(func() {
				if _, _, err := h.Acquire("test"); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if n := o.opens.Load(); n != 1 {
			t.Errorf("opens = %d, want 1", n)
		}
	})

	t.Run("ReplacesDeadModemAndIgnoresStaleRelease", func(t *testing.T) {
		h, o := newTestModemHub(t)
		defer h.Close()
		m1, release1, _ := h.Acquire("test")
		m1.(*fakeModem).dead.Store(true)
		m2, _, _ := h.Acquire("test")
		if m1 == m2 || !m1.(*fakeModem).closed.Load() || o.opens.Load() != 2 {
			t.Fatal("dead modem was not closed and replaced")
		}
		release1()
		time.Sleep(2 * modemGracePeriod)
		if m2.(*fakeModem).closed.Load() {
			t.Error("stale release tore down the replacement")
		}
	})

	t.Run("GracePeriod", func(t *testing.T) {
		h, _ := newTestModemHub(t)
		defer h.Close()
		m, release, _ := h.Acquire("test")
		release()
		release()
		_, release, _ = h.Acquire("test")
		time.Sleep(2 * modemGracePeriod)
		if m.(*fakeModem).closed.Load() {
			t.Fatal("closed while held")
		}
		release()
		time.Sleep(2 * modemGracePeriod)
		if !m.(*fakeModem).closed.Load() || h.Current("test") != nil {
			t.Error("not closed after grace period")
		}
	})

	t.Run("FailedOpenLeavesNoState", func(t *testing.T) {
		h, o := newTestModemHub(t)
		defer h.Close()
		o.err = errors.New("unreachable")
		if _, _, err := h.Acquire("test"); err == nil {
			t.Fatal("expected error")
		}
		o.err = nil
		if _, _, err := h.Acquire("test"); err != nil || o.opens.Load() != 2 {
			t.Errorf("retry after failure: err=%v opens=%d", err, o.opens.Load())
		}
	})

	t.Run("CloseIgnoresHolds", func(t *testing.T) {
		h, _ := newTestModemHub(t)
		m, release, _ := h.Acquire("test")
		h.Close()
		release()
		if !m.(*fakeModem).closed.Load() {
			t.Error("Close left a held modem open")
		}
		if _, _, err := h.Acquire("test"); err != errModemHubClosed {
			t.Errorf("Acquire after Close: %v", err)
		}
	})
}
