package app

import (
	"net"
	"testing"
	"time"
)

type mockListener struct {
	closed    bool
	acceptErr error
}

func (m *mockListener) Accept() (net.Conn, error) {
	if m.acceptErr != nil {
		return nil, m.acceptErr
	}
	select {} // Block forever to simulate working listener
}

func (m *mockListener) Close() error   { m.closed = true; m.acceptErr = net.ErrClosed; return nil }
func (m *mockListener) Addr() net.Addr { return nil }

type mockTransportListener struct {
	name          string
	initErr       error
	initCallCount int
}

func (m *mockTransportListener) Init() (net.Listener, error) {
	m.initCallCount++
	if m.initErr != nil {
		return nil, m.initErr
	}
	return &mockListener{}, nil
}

func (m *mockTransportListener) Name() string                   { return m.name }
func (m *mockTransportListener) CurrentFreq() (Frequency, bool) { return 0, false }

func createTestApp() *App {
	return &App{
		websocketHub: noopWSSocket{},
		eventLog:     &EventLogger{},
	}
}

func TestListenerHub_EnableDisable(t *testing.T) {
	hub := NewListenerHub(createTestApp())
	defer hub.Close()

	t.Run("Enable", func(t *testing.T) {
		hub.Enable(&mockTransportListener{name: "test"})
		active := hub.Active()
		if len(active) != 1 {
			t.Errorf("Expected 1 active listener, got %d", len(active))
		}
	})
	t.Run("Disable", func(t *testing.T) {
		removed, err := hub.Disable("test")
		if err != nil {
			t.Fatalf("Disable should not return error: %v", err)
		}
		if !removed {
			t.Error("Disable should return true when listener was removed")
		}
		active := hub.Active()
		if len(active) != 0 {
			t.Errorf("Expected 0 active listeners after removal, got %d", len(active))
		}
	})
}

func TestListener_RetriesOnFailure(t *testing.T) {
	hub := NewListenerHub(createTestApp())
	defer hub.Close()

	transport := &mockTransportListener{
		name:    "test",
		initErr: net.ErrClosed,
	}
	hub.Enable(transport)

	// Wait for multiple retry attempts
	time.Sleep(2500 * time.Millisecond)

	if transport.initCallCount < 2 {
		t.Errorf("Expected at least 2 Init calls due to retries, got %d", transport.initCallCount)
	}
}
