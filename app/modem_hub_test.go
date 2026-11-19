package app

import (
	"context"
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
	hang   chan struct{} // if set, Ping blocks until closed
}

func (m *fakeModem) Close() error                  { m.closed.Store(true); return nil }
func (m *fakeModem) Listen() (net.Listener, error) { return newMockListener(), nil }
func (m *fakeModem) Ping() error {
	if m.hang != nil {
		<-m.hang
	}
	if m.dead.Load() {
		return errors.New("dead")
	}
	return nil
}

type fakeOpener struct {
	opens atomic.Int32
	err   error
	gate  chan struct{} // if set, open blocks until closed
	last  atomic.Pointer[fakeModem]
}

func (o *fakeOpener) open() (Modem, error) {
	o.opens.Add(1)
	time.Sleep(10 * time.Millisecond)
	if o.gate != nil {
		<-o.gate
	}
	if o.err != nil {
		return nil, o.err
	}
	m := &fakeModem{}
	o.last.Store(m)
	return m, nil
}

func eventually(t *testing.T, cond func() bool) bool {
	t.Helper()
	for range 100 {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func newTestModemHub(t *testing.T) (*ModemHub, *fakeOpener) {
	oldGrace, oldOpen, oldPing := modemGracePeriod, modemOpenTimeout, modemPingTimeout
	modemGracePeriod, modemOpenTimeout, modemPingTimeout = 50*time.Millisecond, 100*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { modemGracePeriod, modemOpenTimeout, modemPingTimeout = oldGrace, oldOpen, oldPing })
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
				if _, _, err := h.Acquire(context.Background(), "test"); err != nil {
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
		m1, release1, _ := h.Acquire(context.Background(), "test")
		m1.(*fakeModem).dead.Store(true)
		m2, _, _ := h.Acquire(context.Background(), "test")
		if m1 == m2 || !eventually(t, m1.(*fakeModem).closed.Load) || o.opens.Load() != 2 {
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
		m, release, _ := h.Acquire(context.Background(), "test")
		release()
		release()
		_, release, _ = h.Acquire(context.Background(), "test")
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
		if _, _, err := h.Acquire(context.Background(), "test"); err == nil {
			t.Fatal("expected error")
		}
		o.err = nil
		if _, _, err := h.Acquire(context.Background(), "test"); err != nil || o.opens.Load() != 2 {
			t.Errorf("retry after failure: err=%v opens=%d", err, o.opens.Load())
		}
	})

	t.Run("CancelledAcquireReturnsWhileOpenWedged", func(t *testing.T) {
		h, o := newTestModemHub(t)
		defer h.Close()
		o.gate = make(chan struct{})
		defer close(o.gate)
		go h.Acquire(context.Background(), "test")
		eventually(t, func() bool { return o.opens.Load() == 1 })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := h.Acquire(ctx, "test"); err != context.Canceled {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	})

	t.Run("LateOpenIsClosedNotInstalled", func(t *testing.T) {
		h, o := newTestModemHub(t)
		defer h.Close()
		o.gate = make(chan struct{})
		if _, _, err := h.Acquire(context.Background(), "test"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want deadline exceeded", err)
		}
		close(o.gate)
		if !eventually(t, func() bool { m := o.last.Load(); return m != nil && m.closed.Load() }) {
			t.Error("late modem not closed")
		}
		if h.Current("test") != nil {
			t.Error("late modem installed")
		}
	})

	t.Run("PingTimeoutReplacesModem", func(t *testing.T) {
		h, o := newTestModemHub(t)
		defer h.Close()
		m1, _, _ := h.Acquire(context.Background(), "test")
		m1.(*fakeModem).hang = make(chan struct{})
		defer close(m1.(*fakeModem).hang)
		m2, _, err := h.Acquire(context.Background(), "test")
		if err != nil || m2 == m1 || o.opens.Load() != 2 {
			t.Fatalf("hung modem not replaced: err=%v opens=%d", err, o.opens.Load())
		}
		if !eventually(t, m1.(*fakeModem).closed.Load) {
			t.Error("hung modem not closed")
		}
	})

	t.Run("CloseIgnoresHolds", func(t *testing.T) {
		h, _ := newTestModemHub(t)
		m, release, _ := h.Acquire(context.Background(), "test")
		h.Close()
		release()
		if !m.(*fakeModem).closed.Load() {
			t.Error("Close left a held modem open")
		}
		if h.Current("test") != nil {
			t.Error("Current after Close")
		}
		if _, _, err := h.Acquire(context.Background(), "test"); err != errModemHubClosed {
			t.Errorf("Acquire after Close: %v", err)
		}
	})
}
