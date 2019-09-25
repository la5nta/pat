package gpsd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type NMEAMode int

const (
	ModeUnknown NMEAMode = iota
	ModeNoFix
	Mode2D
	Mode3D
)

var ErrUnsupportedProtocolVersion = errors.New("Unsupported protocol version")

// Objects implementing the Positioner interface provides geographic positioning data.
//
// This is particularly useful for testing if an object returned by Next can be used to determine the device position.
type Positioner interface {
	Position() Position
	HasFix() bool
}

// Position holds geopgraphic positioning data.
type Position struct {
	Lat, Lon float64   // Latitude/longitude in degrees. +/- signifies north/south.
	Alt      float64   // Altitude in meters.
	Track    float64   // Course over ground, degrees from true north.
	Speed    float64   // Speed over ground, meters per second.
	Time     time.Time // Time as reported by the device.
}

// Conn represents a socket connection to an GPSd daemon.
type Conn struct {
	Version Version

	mu           sync.Mutex
	tcpConn      net.Conn
	rd           *bufio.Reader
	watchEnabled bool
	closed       bool
}

// Dial establishes a socket connection to the GPSd daemon.
func Dial(addr string) (*Conn, error) {
	tcpConn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		tcpConn: tcpConn,
		rd:      bufio.NewReader(tcpConn),
	}

	err = json.NewDecoder(c.rd).Decode(&c.Version)
	if err != nil || c.Version.Release == "" {
		tcpConn.Close()
		return nil, errors.New("Unexpected server response")
	}

	if c.Version.ProtoMajor < 3 {
		tcpConn.Close()
		return nil, ErrUnsupportedProtocolVersion
	}

	return c, nil
}

// Watch enables or disables the watcher mode.
//
// In watcher mode, GPS reports are dumped as TPV and SKY objects. These objects are available through the Next method.
func (c *Conn) Watch(enable bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	if enable == c.watchEnabled {
		return enable
	}

	c.tcpConn.SetDeadline(time.Now().Add(30 * time.Second))
	defer c.tcpConn.SetDeadline(time.Time{})

	param, _ := json.Marshal(
		map[string]interface{}{
			"class":  "WATCH",
			"enable": enable,
			"json":   true,
		})
	c.send("?WATCH=%s", param)

	for {
		obj, err := c.next()
		if err != nil {
			return false
		}

		if watch, ok := obj.(watch); ok {
			c.watchEnabled = watch.Enable
			break
		}
	}

	return c.watchEnabled
}

// Close closes the GPSd daemon connection.
func (c *Conn) Close() error {
	c.Watch(false)
	c.closed = true
	return c.tcpConn.Close()
}

// Next returns the next object sent from the daemon, or an error.
//
// The empty interface returned can be any of the following types:
//   * Sky: A Sky object reports a sky view of the GPS satellite positions.
//   * TPV: A TPV object is a time-position-velocity report.
func (c *Conn) Next() (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		obj, err := c.next()
		if err != nil {
			return nil, err
		}

		switch obj.(type) {
		case TPV, Sky:
			return obj, nil
		default:
			// Ignore other objects for now.
		}
	}
}

func (c *Conn) next() (interface{}, error) {
	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	return parseJSONObject(line)
}

var (
	ErrTimeout          = errors.New("Timeout")
	ErrWatchModeEnabled = errors.New("Operation not available while in watch mode")
)

// NextPos returns the next reported position.
func (c *Conn) NextPos() (Position, error) {
	return c.NextPosTimeout(0)
}

// NextPosTimeout returns the next reported position, or an empty position on timeout.
func (c *Conn) NextPosTimeout(timeout time.Duration) (Position, error) {
	var deadline time.Time

	if timeout > 0 {
		deadline = time.Now().Add(timeout)
		c.tcpConn.SetDeadline(deadline)
		defer c.tcpConn.SetDeadline(time.Time{})
	}

	for {
		obj, err := c.Next()
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return Position{}, ErrTimeout
		} else if err != nil {
			return Position{}, err
		}

		if pos, ok := obj.(Positioner); ok && pos.HasFix() {
			return pos.Position(), nil
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			return Position{}, ErrTimeout
		}
	}
}

// Devices returns a list of all devices GPSd is aware of.
//
// ErrWatchModeEnabled will be returned if the connection is in watch mode.
// A nil-slice will be returned if the connection has been closed.
func (c *Conn) Devices() ([]Device, error) {
	if c.closed {
		return nil, nil
	} else if c.watchEnabled {
		return nil, ErrWatchModeEnabled
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.send("?DEVICES;")

	for {
		obj, err := c.next()
		if err != nil {
			return nil, errUnexpected(err)
		}

		if devs, ok := obj.([]Device); ok {
			return devs, nil
		}
	}
}

func (c *Conn) send(s string, params ...interface{}) error {
	_, err := fmt.Fprintf(c.tcpConn, s, params...)
	return errUnexpected(err)
}

func errUnexpected(err error) error {
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}
