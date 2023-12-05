package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sync/errgroup"
)

type prehookConn struct {
	net.Conn
	br *bufio.Reader

	executable string
	args       []string
}

func NewPrehookConn(conn net.Conn, executable string, args ...string) prehookConn {
	return prehookConn{
		Conn:       conn,
		br:         bufio.NewReader(conn),
		executable: executable,
		args:       args,
	}
}

func (p prehookConn) Read(b []byte) (int, error) { return p.br.Read(b) }

// Wait waits for the prehook process to exit, returning nil if the process
// terminated successfully (exit code 0).
func (p prehookConn) Wait(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, p.executable, p.args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = p.Conn
	cmdStdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	// Copy environment to the child process. Also include additional
	// relevant variables: REMOTE_ADDR, LOCAL_ADDR and the output of the
	// env command.
	cmd.Env = append(append(os.Environ(),
		"PAT_REMOTE_ADDR="+p.RemoteAddr().String(),
		"PAT_LOCAL_ADDR="+p.LocalAddr().String(),
	), envAll()...)

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
			switch peek, err := r.Peek(1); {
			case err != nil:
				// Connection lost.
				return err
			case len(peek) > 0 && isDelimiter(peek[0]):
				r.Discard(1)
				continue
			case ctx.Err() != nil:
				// Child process exited before the next line
				// arrived. We're done.
				return nil
			}
		}

		// Read and forward the byte.
		// Replace CR with LF for convenience.
		b, err := r.ReadByte()
		if err != nil {
			// Connection lost.
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
