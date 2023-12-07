// Package prehook implements a connection prehook mechanism, to handle any
// pre-negotiation required by a remote node before the B2F protocol can
// commence (e.g. packet node traversal).
package prehook

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"golang.org/x/sync/errgroup"
)

var ErrConnNotWrapped = errors.New("connection not wrapped for prehook")

type Script struct {
	File string
	Args []string
	Env  []string
}

// Execute executes the prehook script on a wrapped connection.
//
// ErrConnNotWrapped is returned if conn is not wrapped.
func (s Script) Execute(ctx context.Context, conn net.Conn) error {
	if conn, ok := conn.(*Conn); ok {
		return conn.Execute(ctx, s)
	}
	return ErrConnNotWrapped
}

type Conn struct {
	net.Conn
	br *bufio.Reader
}

// Verify returns nil if the given script file is found and valid.
func Verify(file string) error { _, err := lookPath(file); return err }

func lookPath(file string) (string, error) {
	// Look in our custom location first
	if p, err := exec.LookPath(filepath.Join(directories.ConfigDir(), "prehooks", file)); err == nil {
		return p, nil
	}
	p, err := exec.LookPath(file)
	if errors.Is(err, exec.ErrDot) {
		return file, nil
	}
	return p, err
}

// Wrap returns a wrapped connection with the ability to execute a prehook.
//
// The returned Conn implements the net.Conn interface, and should be used in
// place of the original throughout the lifetime of the connection once the
// prehook script is executed.
func Wrap(conn net.Conn) *Conn {
	return &Conn{
		Conn: conn,
		br:   bufio.NewReader(conn),
	}
}

func (p *Conn) Read(b []byte) (int, error) { return p.br.Read(b) }

// Execute executes the prehook script, returning nil if the process
// terminated successfully (exit code 0).
func (p *Conn) Execute(ctx context.Context, script Script) error {
	execPath, err := lookPath(script.File)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, execPath, script.Args...)
	cmd.Env = script.Env
	cmd.Stderr = os.Stderr
	cmd.Stdout = p.Conn
	cmdStdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	debugf("start cmd: %s", cmd)
	if err := cmd.Start(); err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g.Go(func() error { return forwardLines(ctx, cmdStdin, p.br) })
	g.Go(func() error { defer cancel(); return cmd.Wait() })
	return g.Wait()
}

// forwardLines forwards data from to the spawned process line by line.
//
// The line delimiter is CR or LF, but to facilitate scripting we forward
// each line with LF ending only.
func forwardLines(ctx context.Context, w io.Writer, r *bufio.Reader) error {
	// Copy the lines to stdout so the user can see what's going on.
	stdinBuffered := bufio.NewWriter(io.MultiWriter(w, os.Stdout))
	defer stdinBuffered.Flush()

	isDelimiter := func(b byte) bool { return b == '\n' || b == '\r' }

	var isPrefix bool // true if we're in the middle of a line
	for {
		if !isPrefix {
			// Peek until the next new line (discard empty lines).
			debugf("wait next line")
			switch peek, err := r.Peek(1); {
			case err != nil:
				// Connection lost.
				debugf("connection lost while waiting for next line")
				return err
			case len(peek) > 0 && isDelimiter(peek[0]):
				debugf("discard %q", peek)
				r.Discard(1)
				continue
			case ctx.Err() != nil:
				// Child process exited before the next line
				// arrived. We're done.
				debugf("cmd exited while waiting for next line")
				return nil
			default:
				debugf("at next line")
			}
		}

		// Read and forward the byte.
		// Replace CR with LF for convenience.
		b, err := r.ReadByte()
		if err != nil {
			// Connection lost.
			debugf("connection lost while reading next byte")
			return err
		}
		if b == '\r' {
			b = '\n'
		}
		stdinBuffered.WriteByte(b)

		isPrefix = !isDelimiter(b)
		if isPrefix {
			// Keep going. We're in the middle of a line.
			continue
		}

		// A line was just terminated.
		// Flush and wait a bit to check if the process exits.
		if err := stdinBuffered.Flush(); err != nil {
			return fmt.Errorf("child process exited prematurely: %w", err)
		}
		select {
		case <-time.After(100 * time.Millisecond):
			// Child process is still alive. Keep going.
		case <-ctx.Done():
			// Child process exited. We're done.
			return nil
		}
	}
}

func debugf(format string, args ...interface{}) {
	debug.Printf("prehook: "+format, args...)
}
