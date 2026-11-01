package app

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/wl2k-go/transport/ardop"
)

// TestHelperProcess is not a real test. It's re-exec'd (via os.Args[0]) as a
// stand-in binary for a transport's LaunchCmd, following the standard Go
// self-exec pattern used by os/exec's own tests. It only does anything when
// PAT_TEST_HELPER_PROCESS=1 is set in its environment, so normal `go test`
// runs treat it as a no-op.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("PAT_TEST_HELPER_PROCESS") != "1" {
		return
	}
	if marker := os.Getenv("PAT_TEST_HELPER_MARKER_FILE"); marker != "" {
		if f, err := os.OpenFile(marker, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.WriteString("spawned\n")
			f.Close()
		}
	}
	if os.Getenv("PAT_TEST_HELPER_BLOCK") == "1" {
		select {} // Simulate a long-running daemon until killed.
	}
	if ms, err := strconv.Atoi(os.Getenv("PAT_TEST_HELPER_SLEEP_MS")); err == nil && ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return
	}
	// Otherwise: simulate a daemon that starts and exits immediately.
}

// helperLaunchCmd returns a LaunchCmd that re-execs the test binary into
// TestHelperProcess above, standing in for a real transport daemon. The
// caller must set PAT_TEST_HELPER_PROCESS=1 (e.g. via t.Setenv) for the
// re-exec'd process to do anything.
func helperLaunchCmd() cfg.LaunchCmd {
	return cfg.LaunchCmd{Path: os.Args[0], Args: []string{"-test.run=^TestHelperProcess$", "--"}}
}

// resetLaunchTestState shrinks the launch-poll timing for fast tests and
// restores package state (including dialARDOP, dialVara, and dialAGWPE)
// after each test. Shared across the Ardop, VARA, and AGWPE test files: this
// is test scaffolding, not the production "shared launch/poll/reachability
// helper" the spec ruled out between transports.
func resetLaunchTestState(t *testing.T) {
	t.Helper()
	origInterval, origBudget := launchPollInterval, launchPollBudget
	origDialARDOP, origDialVara, origDialAGWPE := dialARDOP, dialVara, dialAGWPE
	launchPollInterval = 10 * time.Millisecond
	launchPollBudget = 100 * time.Millisecond
	t.Cleanup(func() {
		launchPollInterval, launchPollBudget = origInterval, origBudget
		dialARDOP, dialVara, dialAGWPE = origDialARDOP, origDialVara, origDialAGWPE
	})
}

// captureLog redirects the standard logger's output for the duration of the
// test and returns the buffer it's writing to.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// helperStartupTimeout bounds how long tests wait for a re-exec'd test
// binary (standing in for a transport daemon) to actually run and produce
// an observable side effect. This is longer than a dedicated helper binary
// would need, since re-exec'ing the whole test binary (especially built
// with -race) carries real Go-runtime/testing-framework startup latency.
const helperStartupTimeout = 5 * time.Second

func waitUntilNotInFlight(t *testing.T, a *App, name string) {
	t.Helper()
	deadline := time.Now().Add(helperStartupTimeout)
	for a.isLaunchInFlight(name) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.isLaunchInFlight(name) {
		t.Fatalf("timed out waiting for the launched %q process to be reaped", name)
	}
}

// waitForMarkerCount polls markerFile until it contains exactly want
// occurrences of "spawned\n", or fails the test if that doesn't happen
// within helperStartupTimeout. Needed because cmd.Start() returns as soon
// as the OS accepts the fork+exec — it gives no guarantee the child has
// run far enough to have written anything yet.
func waitForMarkerCount(t *testing.T, markerFile string, want int) {
	t.Helper()
	deadline := time.Now().Add(helperStartupTimeout)
	var data []byte
	for time.Now().Before(deadline) {
		data, _ = os.ReadFile(markerFile)
		if strings.Count(string(data), "spawned\n") >= want {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := strings.Count(string(data), "spawned\n"); got != want {
		t.Errorf("expected exactly %d spawn(s) recorded in the marker file, got %d: %q", want, got, string(data))
	}
}

func TestOpenARDOP_AlreadyReachable_NoLaunch(t *testing.T) {
	resetLaunchTestState(t)

	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return &ardop.TNC{}, nil
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{
		LaunchCmd: cfg.LaunchCmd{Path: os.Args[0]}, // would fail loudly if exec'd unexpectedly
	}}}

	tnc, err := a.openARDOP()
	if err != nil {
		t.Fatalf("openARDOP() error = %v, want nil", err)
	}
	if tnc == nil {
		t.Fatal("openARDOP() returned nil TNC on success")
	}
	if a.isLaunchInFlight(MethodArdop) {
		t.Error("openARDOP() spawned a launch when the TNC was already reachable")
	}
}

func TestOpenARDOP_Unreachable_NoLaunchCmd_IsNoOp(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("connection refused")
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, wantErr
	}

	// LaunchCmd left unconfigured (zero value).
	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{}}}

	_, err := a.openARDOP()
	if !errors.Is(err, wantErr) {
		t.Fatalf("openARDOP() error = %v, want %v", err, wantErr)
	}
	if a.isLaunchInFlight(MethodArdop) {
		t.Error("openARDOP() spawned a launch when LaunchCmd was unconfigured")
	}
}

func TestInitARDOP_WrapsDialError(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("boom")
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, wantErr
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{}}}

	err := a.initARDOP()
	if err == nil {
		t.Fatal("initARDOP() error = nil, want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("initARDOP() error = %v, does not wrap %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "ARDOP TNC initialization failed") {
		t.Errorf("initARDOP() error = %q, want it to contain %q", err.Error(), "ARDOP TNC initialization failed")
	}
}

// Deliberately tests waitForARDOP directly rather than the full
// initARDOPForConnect/initARDOP chain: a successful outcome there would go
// on to call configureARDOP, which invokes real *ardop.TNC methods
// (SetBusyFunc/SetCWID/Version) that need a fully handshaked TNC and hang
// forever on the bare fake &ardop.TNC{} dialARDOP returns in tests. That's
// exactly the ARDOP-protocol-simulation cost the seam discussion ruled out;
// waitForARDOP is the boundary that keeps this feature's own retry/launch
// logic testable without paying it.
func TestWaitForARDOP_LaunchesAndBecomesReachableWithinBudget(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")

	var calls int
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		calls++
		if calls <= 2 {
			return nil, errors.New("connection refused")
		}
		return &ardop.TNC{}, nil
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}

	tnc, err := a.waitForARDOP()
	if err != nil {
		t.Fatalf("waitForARDOP() error = %v, want nil", err)
	}
	if tnc == nil {
		t.Fatal("waitForARDOP() returned nil TNC on success")
	}
	if calls < 2 {
		t.Errorf("expected at least one retry dial after the initial failure, got %d total calls", calls)
	}
	if strings.Contains(logs.String(), "Pat failed to launch") {
		t.Errorf("unexpected failure log on a successful launch: %q", logs.String())
	}
}

// A bare openARDOP() call (as used directly by the Listen path via
// initARDOP()) must NOT itself poll/wait for a freshly launched daemon —
// that would turn ListenerHub's 1s retry cadence into launchPollBudget per
// tick. Only initARDOPForConnect (used by outgoing Connect) may wait.
func TestOpenARDOP_DoesNotPoll(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")

	var calls int
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("connection refused")
		}
		return &ardop.TNC{}, nil // would succeed on a second attempt, if one were made
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	start := time.Now()
	_, err := a.openARDOP()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("openARDOP() error = nil, want the single failed dial's error (it must not retry internally)")
	}
	if calls != 1 {
		t.Errorf("openARDOP() made %d dial attempts, want exactly 1 (no internal polling)", calls)
	}
	if elapsed >= launchPollBudget {
		t.Errorf("openARDOP() took %s, want well under launchPollBudget (%s) since it must not poll", elapsed, launchPollBudget)
	}
}

func TestOpenARDOP_LaunchCmdFailsToStart(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("connection refused")
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{
		LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")},
	}}}

	_, err := a.openARDOP()
	if !errors.Is(err, wantErr) {
		t.Fatalf("openARDOP() error = %v, want the original dial error %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch ardop"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch ardop", got, logs.String())
	}
}

func TestInitARDOPForConnect_NeverBecomesReachable_LogsOnce(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1") // stay alive for the whole poll budget

	wantErr := errors.New("connection refused")
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	err := a.initARDOPForConnect()
	if !errors.Is(err, wantErr) {
		t.Fatalf("initARDOPForConnect() error = %v, want %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch ardop"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch ardop", got, logs.String())
	}
}

// When launch_cmd fails to even start (as opposed to starting but never
// becoming reachable), nothing was ever created, so waitForARDOP must give
// up immediately rather than polling out the budget — a definitive signal,
// unlike "our child process exited" (see the no-early-break test below).
func TestInitARDOPForConnect_LaunchCmdFailsToStart_FailsFastAndLogsOnce(t *testing.T) {
	resetLaunchTestState(t)
	launchPollBudget = 2 * time.Second // generous, so a "burned the budget" bug is unambiguous

	wantErr := errors.New("connection refused")
	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{
		LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")},
	}}}

	start := time.Now()
	err := a.initARDOPForConnect()
	elapsed := time.Since(start)

	if !errors.Is(err, wantErr) {
		t.Fatalf("initARDOPForConnect() error = %v, want %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch ardop"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch ardop", got, logs.String())
	}
	if elapsed >= launchPollBudget {
		t.Errorf("initARDOPForConnect() took %s (the full budget), want it to fail immediately since cmd.Start() itself failed", elapsed)
	}
}

// waitForARDOP must NOT give up just because the process it spawned has
// exited: some launch_cmd scripts daemonize (a short-lived parent forks the
// real daemon and exits), so an exited child is not reliable evidence the
// daemon isn't still coming up. This is a regression test for a bug where
// an earlier "give up if our child exited" heuristic caused a flaky false
// failure on any launch_cmd that happens to exit quickly.
func TestWaitForARDOP_DoesNotBreakEarlyWhenProcessExits(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	// No BLOCK/SLEEP env: the helper exits almost immediately.

	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	start := time.Now()
	if _, err := a.waitForARDOP(); err == nil {
		t.Fatal("expected an error since dialARDOP always fails")
	}
	elapsed := time.Since(start)

	if elapsed < launchPollBudget {
		t.Errorf("waitForARDOP() took only %s, want it to poll the full launchPollBudget (%s) despite the child exiting quickly", elapsed, launchPollBudget)
	}
}

func TestOpenARDOP_InFlight_NoDuplicateSpawn(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1") // stay alive across both calls

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	if _, err := a.openARDOP(); err == nil {
		t.Fatal("expected an error since the daemon never becomes reachable")
	}
	if _, err := a.openARDOP(); err == nil {
		t.Fatal("expected an error on the second call too")
	}

	waitForMarkerCount(t, markerFile, 1)
}

// The sequential test above can't catch a check-then-spawn race, since
// sequential calls trivially serialize regardless of whether startLaunch is
// atomic. This drives many concurrent openARDOP() calls at once — the
// realistic case being a Listen retry tick overlapping a user-triggered
// Connect on the same App — to catch a duplicate spawn under -race.
func TestOpenARDOP_ConcurrentCalls_NoDuplicateSpawn(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1")

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			a.openARDOP()
		}()
	}
	wg.Wait()

	waitForMarkerCount(t, markerFile, 1)
}

func TestOpenARDOP_RelaunchesAfterProcessExits(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	// No BLOCK env: the helper exits immediately, so it gets reaped quickly.

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, errors.New("connection refused") // never reachable, keep it simple
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	if _, err := a.openARDOP(); err == nil {
		t.Fatal("expected an error since the daemon never becomes reachable")
	}
	waitUntilNotInFlight(t, a, MethodArdop)

	if _, err := a.openARDOP(); err == nil {
		t.Fatal("expected an error on the second call too")
	}

	waitForMarkerCount(t, markerFile, 2)
}

func TestTerminateLaunchedProcesses_KillsTrackedProcess(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1")

	dialARDOP = func(addr, mycall, gridSquare string) (*ardop.TNC, error) {
		return nil, errors.New("connection refused")
	}

	a := &App{config: cfg.Config{Ardop: cfg.ArdopConfig{LaunchCmd: helperLaunchCmd()}}}

	if _, err := a.openARDOP(); err == nil {
		t.Fatal("expected an error since dialARDOP always fails")
	}
	if !a.isLaunchInFlight(MethodArdop) {
		t.Fatal("expected the blocking helper process to still be tracked as in flight")
	}

	a.terminateLaunchedProcesses()

	waitUntilNotInFlight(t, a, MethodArdop)
}
