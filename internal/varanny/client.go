// Package varanny provides a client for discovering and controlling VARA modems via varanny services.
package varanny

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	defaultDiscoveryTimeout   = 5 * time.Second
	defaultPortBindingTimeout = 10 * time.Second
	defaultModemTTL           = 10 * time.Minute
)

var (
	ErrNoModemsFound   = errors.New("no varanny modems found")
	ErrModemNotFound   = errors.New("modem not found")
	ErrDiscoveryFailed = errors.New("varanny discovery failed")
)

// ModemInfo represents a discovered varanny modem.
type ModemInfo struct {
	Name       string    // Modem name (e.g., "IC705HF")
	Type       string    // "hf" or "fm"
	Host       string    // Hostname or IP
	CmdPort    int       // VARA command port
	DataPort   int       // VARA data port (CmdPort + 1)
	LaunchPort int       // Varanny control port
	CatPort    int       // CAT control port (0 if not configured)
	CatDialect string    // CAT protocol (e.g., "hamlib")
	FirstSeen  time.Time // When this modem was first discovered
	LastSeen   time.Time // When this modem was last refreshed
}

// IsExpired returns true if the modem hasn't been seen within TTL.
func (m *ModemInfo) IsExpired(ttl time.Duration) bool {
	return time.Since(m.LastSeen) > ttl
}

// DiscoverModems discovers varanny modems via mDNS.
// Returns error if mDNS browse itself fails (not just no modems found).
func DiscoverModems(ctx context.Context) ([]ModemInfo, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create zeroconf resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	errChan := make(chan error, 1)

	go func() {
		errChan <- resolver.Browse(ctx, "_vara-modem._tcp", "local.", entries)
	}()

	var modems []ModemInfo
	seen := make(map[string]bool) // Deduplicate by name

	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				// Channel closed, check for browse error
				if err := <-errChan; err != nil {
					return nil, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
				}
				return modems, nil
			}
			info, err := ParseZeroconfEntry(entry)
			if err != nil {
				continue // Log this? Skip bad entries
			}
			// Deduplicate - only update if newer or not seen
			if !seen[info.Name] {
				seen[info.Name] = true
				info.FirstSeen = time.Now()
				info.LastSeen = time.Now()
				modems = append(modems, info)
			}

		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				// Normal timeout
				return modems, nil
			}
			return nil, ctx.Err()
		}
	}
}

// ParseZeroconfEntry parses a zeroconf entry into ModemInfo.
func ParseZeroconfEntry(entry *zeroconf.ServiceEntry) (ModemInfo, error) {
	var info ModemInfo

	// Extract modem name from ServiceInstanceName() (e.g., "IC705HF._vara-modem._tcp.local.")
	info.Name = strings.TrimSuffix(entry.ServiceInstanceName(), "._vara-modem._tcp.local.")
	info.CmdPort = entry.Port
	info.DataPort = entry.Port + 1

	// Prefer addresses based on network context
	if len(entry.AddrIPv4) > 0 {
		info.Host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		info.Host = entry.AddrIPv6[0].String()
	} else {
		return info, fmt.Errorf("no addresses in entry")
	}

	// Parse TXT records
	for _, txt := range entry.Text {
		parts := strings.SplitN(txt, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "type":
			info.Type = strings.ToLower(value)
		case "launchport":
			fmt.Sscanf(value, "%d", &info.LaunchPort)
		case "catport":
			fmt.Sscanf(value, "%d", &info.CatPort)
		case "catdialect":
			info.CatDialect = value
		}
	}

	// Default launch port if not in TXT
	if info.LaunchPort == 0 {
		info.LaunchPort = 8273
	}

	return info, nil
}

// Session represents an active varanny connection session.
type Session struct {
	client    *Client
	modemInfo ModemInfo
	scheme    string
	createdAt time.Time
	catCloser io.Closer // Closer for varanny's CAT/PTT connection (if used)
}

// NewSession creates a new varanny session.
func NewSession(client *Client, modemInfo ModemInfo, scheme string) *Session {
	return &Session{
		client:    client,
		modemInfo: modemInfo,
		scheme:    scheme,
		createdAt: time.Now(),
	}
}

// SetCATCloser sets the closer for the varanny CAT/PTT connection.
// This will be closed when the session is closed.
func (s *Session) SetCATCloser(c io.Closer) {
	s.catCloser = c
}

// Close closes the session's varanny client and CAT connection.
func (s *Session) Close() error {
	var errs []error

	if s.client != nil {
		if err := s.client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("varanny client: %w", err))
		}
	}

	if s.catCloser != nil {
		if err := s.catCloser.Close(); err != nil {
			errs = append(errs, fmt.Errorf("varanny CAT: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("session close errors: %v", errs)
	}
	return nil
}

// Scheme returns the transport scheme for this session.
func (s *Session) Scheme() string {
	return s.scheme
}

// Client returns the varanny client for this session.
func (s *Session) Client() *Client {
	return s.client
}

// ModemInfo returns the modem info for this session.
func (s *Session) ModemInfo() ModemInfo {
	return s.modemInfo
}

// CreatedAt returns when this session was created.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// Client represents a varanny control connection.
type Client struct {
	conn     net.Conn
	reader   *bufio.Reader
	started  bool
	startedMu sync.RWMutex
}

// IsStarted returns whether the modem has been started.
func (c *Client) IsStarted() bool {
	c.startedMu.RLock()
	defer c.startedMu.RUnlock()
	return c.started
}

// Connect connects to a varanny control port with timeout.
func Connect(ctx context.Context, addr string, port int) (*Client, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to varanny: %w", err)
	}

	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// ListModems returns available modem names.
func (c *Client) ListModems(ctx context.Context) ([]string, error) {
	// Set read/write deadlines from context, with fallback
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	c.conn.SetDeadline(deadline)
	defer c.conn.SetDeadline(time.Time{})

	if _, err := c.conn.Write([]byte("list\n")); err != nil {
		return nil, fmt.Errorf("failed to send list command: %w", err)
	}

	var names []string
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				return nil, errors.New("list command timed out")
			}
			return nil, fmt.Errorf("failed to read list response: %w", err)
		}
		line = strings.TrimSpace(line)

		if line == "OK" {
			return names, nil
		}
		if strings.HasPrefix(line, "ERROR") {
			return nil, fmt.Errorf("list command failed: %s", line)
		}
		names = append(names, line)
	}
}

// StartModem starts the specified modem.
func (c *Client) StartModem(ctx context.Context, name string) error {
	// Set read/write deadlines from context, with fallback
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	c.conn.SetDeadline(deadline)
	defer c.conn.SetDeadline(time.Time{})

	if _, err := c.conn.Write([]byte(fmt.Sprintf("start %s\n", name))); err != nil {
		return fmt.Errorf("failed to send start command: %w", err)
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return errors.New("start command timed out")
		}
		return fmt.Errorf("failed to read start response: %w", err)
	}

	line = strings.TrimSpace(line)
	if line != "OK" {
		return fmt.Errorf("start failed: %s", line)
	}

	c.startedMu.Lock()
	c.started = true
	c.startedMu.Unlock()
	return nil
}

// StopModem stops the current modem session.
func (c *Client) StopModem(ctx context.Context) error {
	c.startedMu.RLock()
	if !c.started {
		c.startedMu.RUnlock()
		return nil
	}
	c.startedMu.RUnlock()

	// Set read/write deadlines from context, with fallback
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	c.conn.SetDeadline(deadline)
	defer c.conn.SetDeadline(time.Time{})

	if _, err := c.conn.Write([]byte("stop\n")); err != nil {
		return fmt.Errorf("failed to send stop command: %w", err)
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return errors.New("stop command timed out")
		}
		return fmt.Errorf("failed to read stop response: %w", err)
	}

	line = strings.TrimSpace(line)
	if line != "OK" {
		return fmt.Errorf("stop failed: %s", line)
	}

	c.startedMu.Lock()
	c.started = false
	c.startedMu.Unlock()
	return nil
}

// Close closes the varanny connection, stopping the modem if started.
func (c *Client) Close() error {
	// Use background context with timeout for cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.startedMu.RLock()
	shouldStop := c.started
	c.startedMu.RUnlock()

	if shouldStop {
		if err := c.StopModem(ctx); err != nil {
			// Log but still close connection
			// Caller should log this properly via application logger
			_ = err
		}
	}
	return c.conn.Close()
}

// WaitForPortBinding waits for VARA to bind to both cmd and data ports.
// The timeout is controlled by the passed context's deadline.
func WaitForPortBinding(ctx context.Context, host string, cmdPort, dataPort int) error {
	checkPort := func(port int) bool {
		dialer := &net.Dialer{Timeout: 500 * time.Millisecond}
		conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
		if err == nil {
			conn.Close()
			return true
		}
		return false
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if checkPort(cmdPort) && checkPort(dataPort) {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}