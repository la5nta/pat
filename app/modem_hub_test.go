package app

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/la5nta/wl2k-go/transport"
)

type fakeModem struct {
	dead   atomic.Bool
	closed atomic.Bool
	hang   chan struct{} // if set, Ping blocks until closed
	block  chan struct{} // if set, Close blocks until closed
	dialed atomic.Pointer[transport.URL]
}

func (m *fakeModem) DialURLContext(_ context.Context, url *transport.URL) (net.Conn, error) {
	m.dialed.Store(url)
	return nil, errors.New("fake dial")
}

func (m *fakeModem) Close() error {
	m.closed.Store(true)
	if m.block != nil {
		<-m.block
	}
	return nil
}
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

// uncomparableModem is a value type with a func field, like statelessModem.
type uncomparableModem struct {
	*fakeModem
	_ func()
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

// hubTest runs f in a synctest bubble, then outlasts every hub deadline so
// abandoned opens, pings and closes must finish or fail the test as a leak.
func hubTest(t *testing.T, name string, f func(t *testing.T)) {
	t.Run(name, func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f(t)
			time.Sleep(modemOpenTimeout)
		})
	})
}

func newTestModemHub() (*ModemHub, *fakeOpener) {
	o := &fakeOpener{}
	return NewModemHub(map[string]ModemOpener{"test": o.open}), o
}

func TestModemHub(t *testing.T) {
	hubTest(t, "ConcurrentAcquireOpensOnce", func(t *testing.T) {
		h, o := newTestModemHub()
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

	hubTest(t, "ReplacesDeadModemAndIgnoresStaleRelease", func(t *testing.T) {
		h, o := newTestModemHub()
		defer h.Close()
		m1, release1, _ := h.Acquire(context.Background(), "test")
		m1.(*fakeModem).dead.Store(true)
		m2, _, _ := h.Acquire(context.Background(), "test")
		synctest.Wait()
		if m1 == m2 || !m1.(*fakeModem).closed.Load() || o.opens.Load() != 2 {
			t.Fatal("dead modem was not closed and replaced")
		}
		release1()
		time.Sleep(modemGracePeriod + time.Second)
		if m2.(*fakeModem).closed.Load() {
			t.Error("stale release tore down the replacement")
		}
	})

	hubTest(t, "UncomparableModem", func(t *testing.T) {
		h, _ := newTestModemHub()
		defer h.Close()
		inner := &fakeModem{}
		h.slots["test"].open = func() (Modem, error) { return uncomparableModem{fakeModem: inner}, nil }
		_, release, err := h.Acquire(context.Background(), "test")
		if err != nil {
			t.Fatal(err)
		}
		release()
		time.Sleep(modemGracePeriod + time.Second)
		if !inner.closed.Load() {
			t.Error("modem not closed after grace period")
		}
	})

	hubTest(t, "BlockingCloseDoesNotFreezeHub", func(t *testing.T) {
		h, _ := newTestModemHub()
		defer h.Close()
		m, release, _ := h.Acquire(context.Background(), "test")
		block := make(chan struct{})
		defer close(block)
		m.(*fakeModem).block = block
		release()
		time.Sleep(modemGracePeriod + time.Second)
		if !m.(*fakeModem).closed.Load() {
			t.Fatal("not closed after grace period")
		}
		h.Current("test")
		if _, release, err := h.Acquire(context.Background(), "test"); err != nil {
			t.Fatal(err)
		} else {
			release()
		}
	})

	hubTest(t, "GracePeriod", func(t *testing.T) {
		h, _ := newTestModemHub()
		defer h.Close()
		m, release, _ := h.Acquire(context.Background(), "test")
		release()
		release()
		_, release, _ = h.Acquire(context.Background(), "test")
		time.Sleep(modemGracePeriod + time.Second)
		if m.(*fakeModem).closed.Load() {
			t.Fatal("closed while held")
		}
		release()
		time.Sleep(modemGracePeriod - time.Second)
		if m.(*fakeModem).closed.Load() {
			t.Fatal("closed before grace period")
		}
		time.Sleep(2 * time.Second)
		if !m.(*fakeModem).closed.Load() || h.Current("test") != nil {
			t.Error("not closed after grace period")
		}
	})

	hubTest(t, "FailedOpenLeavesNoState", func(t *testing.T) {
		h, o := newTestModemHub()
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

	hubTest(t, "CancelledAcquireReturnsWhileOpenWedged", func(t *testing.T) {
		h, o := newTestModemHub()
		defer h.Close()
		o.gate = make(chan struct{})
		defer close(o.gate)
		go h.Acquire(context.Background(), "test")
		synctest.Wait()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := h.Acquire(ctx, "test"); err != context.Canceled {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	})

	hubTest(t, "LateOpenIsClosedNotInstalled", func(t *testing.T) {
		h, o := newTestModemHub()
		defer h.Close()
		o.gate = make(chan struct{})
		start := time.Now()
		if _, _, err := h.Acquire(context.Background(), "test"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want deadline exceeded", err)
		}
		if d := time.Since(start); d < modemOpenTimeout {
			t.Errorf("gave up after %v, want %v", d, modemOpenTimeout)
		}
		close(o.gate)
		synctest.Wait()
		if m := o.last.Load(); m == nil || !m.closed.Load() {
			t.Error("late modem not closed")
		}
		if h.Current("test") != nil {
			t.Error("late modem installed")
		}
	})

	hubTest(t, "PingTimeoutReplacesModem", func(t *testing.T) {
		h, o := newTestModemHub()
		defer h.Close()
		m1, _, _ := h.Acquire(context.Background(), "test")
		m1.(*fakeModem).hang = make(chan struct{})
		defer close(m1.(*fakeModem).hang)
		start := time.Now()
		m2, _, err := h.Acquire(context.Background(), "test")
		if err != nil || m2 == m1 || o.opens.Load() != 2 {
			t.Fatalf("hung modem not replaced: err=%v opens=%d", err, o.opens.Load())
		}
		if d := time.Since(start); d < modemPingTimeout {
			t.Errorf("replaced after %v, want ping timeout %v", d, modemPingTimeout)
		}
		synctest.Wait()
		if !m1.(*fakeModem).closed.Load() {
			t.Error("hung modem not closed")
		}
	})

	hubTest(t, "CloseIgnoresHolds", func(t *testing.T) {
		h, _ := newTestModemHub()
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

	hubTest(t, "CloseBoundsWedgedModem", func(t *testing.T) {
		h, _ := newTestModemHub()
		m, _, _ := h.Acquire(context.Background(), "test")
		block := make(chan struct{})
		defer close(block)
		m.(*fakeModem).block = block
		start := time.Now()
		h.Close()
		if d := time.Since(start); d != modemPingTimeout {
			t.Errorf("Close took %v, want %v", d, modemPingTimeout)
		}
	})
}

func TestConnectDialsAcquiredModem(t *testing.T) {
	ev, err := NewEventLogger(filepath.Join(t.TempDir(), "events.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Close()
	h, o := newTestModemHub()
	defer h.Close()
	a := &App{options: Options{MyCall: "N0CALL"}, websocketHub: noopWSSocket{}, eventLog: ev, modems: h}

	url, err := transport.ParseURL("test:///LA1B")
	if err != nil {
		t.Fatal(err)
	}
	if a.connectURL("test:///LA1B", url) {
		t.Fatal("connect succeeded despite failing dial")
	}
	m := o.last.Load()
	if m == nil {
		t.Fatal("modem never opened")
	}
	if got := m.dialed.Load(); got == nil || got.Target != "LA1B" {
		t.Fatalf("acquired modem dialed %v, want target LA1B", got)
	}
	if a.dialing != nil {
		t.Error("dialing state not cleared")
	}
}

type statusRecorder struct {
	noopWSSocket
	a       *App
	dialing bool // as of the last UpdateStatus
}

func (r *statusRecorder) UpdateStatus() { r.dialing = r.a.dialing != nil }

func TestConnectEarlyReturnClearsDialingStatus(t *testing.T) {
	h, o := newTestModemHub()
	defer h.Close()
	o.err = errors.New("no tnc")
	a := &App{options: Options{MyCall: "N0CALL"}, modems: h}
	ws := &statusRecorder{a: a}
	a.websocketHub = ws

	url, _ := transport.ParseURL("test:///LA1B")
	a.connectURL("test:///LA1B", url)
	if ws.dialing {
		t.Error("web gui last told dialing after failed acquire")
	}
}
