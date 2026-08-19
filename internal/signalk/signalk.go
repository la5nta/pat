// Copyright 2024. All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package signalk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/gorilla/websocket"
)

var (
	ErrNoSignalKServers = errors.New("no Signal K servers found via mDNS")
	ErrConnectionClosed = errors.New("connection closed")
)

// Position holds geographic positioning data.
type Position struct {
	Lat, Lon float64   // Latitude/longitude in degrees. +/- signifies north/south.
	Alt      float64   // Altitude in meters.
	Track    float64   // Course over ground, degrees from true north.
	Speed    float64   // Speed over ground, meters per second.
	Time     time.Time // Time as reported by the device.
}

// Conn represents a WebSocket connection to a Signal K server.
type Conn struct {
	mu            sync.Mutex
	wsConn        *websocket.Conn
	posChan       chan Position
	closed        bool
	useServerTime bool
	selfID        string // The vessel self identifier (e.g., "vessels.self")
}

// Dial establishes a WebSocket connection to a Signal K server via mDNS discovery.
func Dial(useServerTime bool) (*Conn, error) {
	return DialWithToken("", useServerTime)
}

// DialWithToken establishes a WebSocket connection to a specific Signal K server with optional auth.
// If serverURL is empty, mDNS discovery is used.
func DialWithToken(serverURL string, useServerTime bool) (*Conn, error) {
	wsURL, err := discoverServerURL(serverURL)
	if err != nil {
		return nil, fmt.Errorf("Signal K discovery failed: %w", err)
	}

	// Set up WebSocket dialer with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Extract self ID from URL fragment
	selfID := ""
	if idx := strings.Index(wsURL, "#self="); idx != -1 {
		selfID = wsURL[idx+6:]
		wsURL = wsURL[:idx]
	}

	wsConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("WebSocket dial failed: %w", err)
	}

	c := &Conn{
		wsConn:        wsConn,
		posChan:       make(chan Position, 10),
		useServerTime: useServerTime,
		selfID:        selfID,
	}

	// Subscribe to navigation position updates
	context := "vessels.self"
	if c.selfID != "" {
		context = c.selfID
	}

	subscribeMsg := map[string]interface{}{
		"context": context,
		"subscribe": []map[string]interface{}{
			{
				"path":   "navigation.position",
				"period": 1000, // 1 second
				"format": "delta",
				"policy": "instant",
			},
			{
				"path":   "navigation.speedOverGround",
				"period": 1000,
				"format": "delta",
				"policy": "instant",
			},
			{
				"path":   "navigation.courseOverGroundTrue",
				"period": 1000,
				"format": "delta",
				"policy": "instant",
			},
		},
	}

	if err := c.wsConn.WriteJSON(subscribeMsg); err != nil {
		c.Close()
		return nil, fmt.Errorf("subscribe failed: %w", err)
	}

	// Start reading messages in background
	go c.readMessages()

	return c, nil
}

// discoverServerURL discovers a Signal K server and returns WebSocket URL
func discoverServerURL(serverURL string) (string, error) {
	if serverURL != "" {
		return convertToWebSocketURL(serverURL)
	}

	// Discover via mDNS - look for _signalk-http service
	servers, err := DiscoverServers(context.Background(), 5*time.Second)
	if err != nil {
		return "", err
	}
	if len(servers) == 0 {
		return "", ErrNoSignalKServers
	}

	// Use the first server found (HTTP service)
	srv := servers[0]

	// Build HTTP URL to get endpoints
	httpURL := fmt.Sprintf("http://%s:%d/signalk", srv.Host, srv.Port)

	// GET /signalk to get endpoints
	resp, err := http.Get(httpURL)
	if err != nil {
		return "", fmt.Errorf("failed to get Signal K endpoints: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Signal K endpoints returned HTTP %d", resp.StatusCode)
	}

	// Parse endpoints response
	var endpoints map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&endpoints); err != nil {
		return "", fmt.Errorf("failed to parse Signal K endpoints: %w", err)
	}

	// Extract self ID and WebSocket URL from endpoints
	wsURL, selfID, err := extractWebSocketInfo(endpoints)
	if err != nil {
		return "", err
	}

	// Store self ID in URL fragment for later use
	if selfID != "" {
		wsURL += "#self=" + selfID
	}

	return wsURL, nil
}

// extractWebSocketInfo extracts WebSocket URL and self ID from Signal K endpoints
func extractWebSocketInfo(endpoints map[string]interface{}) (wsURL, selfID string, err error) {
	// Get self ID from endpoints
	if self, ok := endpoints["self"].(string); ok {
		selfID = self
	}

	// Get vessels to find the self vessel
	vessels, ok := endpoints["vessels"].(map[string]interface{})
	if !ok {
		return "", "", errors.New("no vessels in Signal K endpoints")
	}

	// Find the self vessel and get its endpoint
	for vesselID, vesselData := range vessels {
		vesselMap, ok := vesselData.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is the self vessel
		if vesselID == selfID || selfID == "" {
			// Look for endpoints in this vessel
			if vEndpoints, ok := vesselMap["endpoints"].(map[string]interface{}); ok {
				// Look for v1 WebSocket endpoint
				if v1, ok := vEndpoints["v1"].(map[string]interface{}); ok {
					if ws, ok := v1["signalk-stream-ws"].(string); ok {
						wsURL = ws
						break
					}
					// Try alternative keys
					if ws, ok := v1["signalk-ws"].(string); ok {
						wsURL = ws
						break
					}
				}
			}
		}
	}

	// If we didn't find a vessel-specific endpoint, try top-level endpoints
	if wsURL == "" {
		if topEndpoints, ok := endpoints["endpoints"].(map[string]interface{}); ok {
			if v1, ok := topEndpoints["v1"].(map[string]interface{}); ok {
				if ws, ok := v1["signalk-stream-ws"].(string); ok {
					wsURL = ws
				}
			}
		}
	}

	// Fallback: construct WebSocket URL from discovery info
	if wsURL == "" {
		return "", "", errors.New("no WebSocket endpoint found in Signal K response")
	}

	return wsURL, selfID, nil
}

// convertToWebSocketURL converts an HTTP URL to WebSocket URL
func convertToWebSocketURL(addr string) (string, error) {
	u, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	scheme := "ws://"
	if u.URL.Scheme == "https" {
		scheme = "wss://"
	} else if u.URL.Scheme != "http" {
		scheme = u.URL.Scheme + "://"
	}

	wsURL := scheme + u.URL.Host
	path := u.URL.Path
	if path == "" || path == "/" {
		path = "/signalk/v1/stream"
	} else if !strings.Contains(path, "stream") {
		if strings.HasSuffix(path, "/") {
			path += "signalk/v1/stream"
		} else {
			path += "/signalk/v1/stream"
		}
	}

	return wsURL + path + "?subscribe=none", nil
}

// readMessages reads WebSocket messages and parses position updates
func (c *Conn) readMessages() {
	defer close(c.posChan)

	for {
		_, data, err := c.wsConn.ReadMessage()
		if err != nil {
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// Handle delta messages
		updates, ok := msg["updates"].([]interface{})
		if !ok {
			continue
		}

		pos := Position{
			// Initialize with current values if we want to merge
		}
		var hasPos, hasSpeed, hasCourse bool

		for _, u := range updates {
			update, ok := u.(map[string]interface{})
			if !ok {
				continue
			}

			values, ok := update["values"].([]interface{})
			if !ok {
				continue
			}

			for _, v := range values {
				value, ok := v.(map[string]interface{})
				if !ok {
					continue
				}

				path, ok := value["path"].(string)
				if !ok {
					continue
				}

				posValue, ok := value["value"].(map[string]interface{})
				if !ok {
					continue
				}

				switch path {
				case "navigation.position":
					if lat, ok := posValue["latitude"].(float64); ok {
						pos.Lat = lat
						hasPos = true
					}
					if lon, ok := posValue["longitude"].(float64); ok {
						pos.Lon = lon
						hasPos = true
					}
					if alt, ok := posValue["altitude"].(float64); ok {
						pos.Alt = alt
					}
					if timestamp, ok := posValue["timestamp"].(string); ok && !c.useServerTime {
						if t, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
							pos.Time = t
						}
					}
				case "navigation.speedOverGround":
					if speed, ok := posValue["value"].(float64); ok {
						pos.Speed = speed
						hasSpeed = true
					}
				case "navigation.courseOverGroundTrue":
					if course, ok := posValue["value"].(float64); ok {
						pos.Track = course
						hasCourse = true
					}
				}
			}
		}

		if hasPos {
			if c.useServerTime || pos.Time.IsZero() {
				pos.Time = time.Now()
			}

			c.mu.Lock()
			if !c.closed {
				c.posChan <- pos
			}
			c.mu.Unlock()
		}
	}
}

// Close closes the Signal K connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	// Send unsubscribe message
	context := "vessels.self"
	if c.selfID != "" {
		context = c.selfID
	}

	unsubscribeMsg := map[string]interface{}{
		"context": context,
		"unsubscribe": []map[string]interface{}{
			{"path": "navigation.position"},
			{"path": "navigation.speedOverGround"},
			{"path": "navigation.courseOverGroundTrue"},
		},
	}
	_ = c.wsConn.WriteJSON(unsubscribeMsg)

	return c.wsConn.Close()
}

// NextPos returns the next reported position.
func (c *Conn) NextPos() (Position, error) {
	return c.NextPosTimeout(0)
}

// NextPosTimeout returns the next reported position, or an empty position on timeout.
func (c *Conn) NextPosTimeout(timeout time.Duration) (Position, error) {
	if timeout > 0 {
		select {
		case pos, ok := <-c.posChan:
			if !ok {
				return Position{}, ErrConnectionClosed
			}
			return pos, nil
		case <-time.After(timeout):
			return Position{}, errors.New("timeout")
		}
	}

	select {
	case pos, ok := <-c.posChan:
		if !ok {
			return Position{}, ErrConnectionClosed
		}
		return pos, nil
	}
}

// ServerInfo represents information about a Signal K server discovered via mDNS.
type ServerInfo struct {
	Name string
	Host string
	Port int
}

// DiscoverServers discovers Signal K servers on the local network using mDNS.
func DiscoverServers(ctx context.Context, timeout time.Duration) ([]ServerInfo, error) {
	var servers []ServerInfo

	// Discover _signalk-http service per Signal K spec
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		_ = zeroconf.Browse(ctx, "_signalk-http", "local.", entries)
	}()

	// Collect entries within timeout
	deadline := time.After(timeout)
	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				goto done
			}
			srv := parseZeroconfEntry(entry, false)
			if srv != nil {
				servers = append(servers, *srv)
			}
		case <-deadline:
			goto done
		case <-ctx.Done():
			goto done
		}
	}
done:

	if len(servers) == 0 {
		return nil, ErrNoSignalKServers
	}

	return servers, nil
}

// parseZeroconfEntry parses a zeroconf service entry into a ServerInfo
func parseZeroconfEntry(entry *zeroconf.ServiceEntry, isWS bool) *ServerInfo {
	if len(entry.AddrIPv4) == 0 && len(entry.AddrIPv6) == 0 {
		return nil
	}

	info := ServerInfo{
		Name: entry.ServiceInstance(),
		Port: entry.Port,
	}

	// Prefer IPv4 addresses
	if len(entry.AddrIPv4) > 0 {
		info.Host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		info.Host = entry.AddrIPv6[0].String()
	}

	return &info
}