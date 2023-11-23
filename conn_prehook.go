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
	g.Go(func() error { return p.forwardLines(ctx, cmdStdin) })
	g.Go(func() error { defer cancel(); return cmd.Wait() })
	return g.Wait()
}

// forwardLines forwards data from to the spawned process line by line.
//
// The line delimiter is CR or LF, but to facilitate scripting we append LF if
// it's missing.
//
// Wait one second after each line, to give the process time to terminate
// before delivering the next line.
func (p prehookConn) forwardLines(ctx context.Context, w io.Writer) error {
	// Copy the lines to stdout so the user can see what's going on.
	stdinBuffered := bufio.NewWriter(io.MultiWriter(w, os.Stdout))
	defer stdinBuffered.Flush()

	var isPrefix bool // true if we're in the middle of a line
	for {
		if !isPrefix {
			// A line was just terminated (or no data has been read yet).
			// Flush and wait one second to check if the process
			// exited. If not we assume it expects an upcoming line.
			if err := stdinBuffered.Flush(); err != nil {
				return fmt.Errorf("child process exited prematurely: %w", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}

		b, err := p.br.ReadByte()
		if err != nil {
			return err
		}
		stdinBuffered.WriteByte(b)
		isPrefix = !(b == '\n' || b == '\r')

		// Make sure CR is always followed by LF. It's easier to deal with in scripts.
		if b == '\r' {
			stdinBuffered.WriteByte('\n')
			// Peek to check if the next byte is the LF we just wrote, in which case discard it.
			if peek, _ := p.br.Peek(1); len(peek) > 0 && peek[0] == '\n' {
				p.br.Discard(1)
			}
		}
	}
}
