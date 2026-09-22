package app

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/wl2k-go/transport/ax25/agwpe"
)

func TestOpenAGWPE_AlreadyReachable_NoLaunch(t *testing.T) {
	resetLaunchTestState(t)

	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return &agwpe.TNCPort{}, nil
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{
		LaunchCmd: cfg.LaunchCmd{Path: "/should-not-be-exec'd"},
	}}}

	tp, err := a.openAGWPE()
	if err != nil {
		t.Fatalf("openAGWPE() error = %v, want nil", err)
	}
	if tp == nil {
		t.Fatal("openAGWPE() returned nil TNCPort on success")
	}
	if a.isLaunchInFlight(agwpeLaunchName) {
		t.Error("openAGWPE() spawned a launch when the TNC was already reachable")
	}
}

func TestOpenAGWPE_Unreachable_NoLaunchCmd_IsNoOp(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("connection refused")
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, wantErr
	}

	// LaunchCmd left unconfigured (zero value).
	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{}}}

	_, err := a.openAGWPE()
	if !errors.Is(err, wantErr) {
		t.Fatalf("openAGWPE() error = %v, want %v", err, wantErr)
	}
	if a.isLaunchInFlight(agwpeLaunchName) {
		t.Error("openAGWPE() spawned a launch when LaunchCmd was unconfigured")
	}
}

func TestInitAGWPE_WrapsDialError(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("boom")
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, wantErr
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{}}}

	err := a.initAGWPE()
	if err == nil {
		t.Fatal("initAGWPE() error = nil, want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("initAGWPE() error = %v, does not wrap %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "AGWPE TNC initialization failed") {
		t.Errorf("initAGWPE() error = %q, want it to contain %q", err.Error(), "AGWPE TNC initialization failed")
	}
}

// Deliberately tests waitForAGWPE directly rather than the full
// initAGWPEForConnect/initAGWPE chain: a successful outcome there would go
// on to call configureAGWPE, which invokes a real (*agwpe.TNCPort).Version()
// that needs a fully handshaked TNC and hangs forever on the bare
// &agwpe.TNCPort{} dialAGWPE returns in tests. That's exactly the
// AGWPE-protocol-simulation cost the seam decision ruled out; waitForAGWPE
// is the boundary that keeps this feature's own retry/launch logic testable
// without paying it.
func TestWaitForAGWPE_LaunchesAndBecomesReachableWithinBudget(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")

	var calls int
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		calls++
		if calls <= 2 {
			return nil, errors.New("connection refused")
		}
		return &agwpe.TNCPort{}, nil
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}

	tp, err := a.waitForAGWPE()
	if err != nil {
		t.Fatalf("waitForAGWPE() error = %v, want nil", err)
	}
	if tp == nil {
		t.Fatal("waitForAGWPE() returned nil TNCPort on success")
	}
	if calls < 2 {
		t.Errorf("expected at least one retry dial after the initial failure, got %d total calls", calls)
	}
	if strings.Contains(logs.String(), "Pat failed to launch") {
		t.Errorf("unexpected failure log on a successful launch: %q", logs.String())
	}
}

// A bare openAGWPE() call (as used directly by the Listen path via
// initAGWPE(), and thus AX25AGWPEListener.Init()) must NOT itself poll/wait
// for a freshly launched daemon — that would turn ListenerHub's 1s retry
// cadence into launchPollBudget per tick. Only initAGWPEForConnect (used by
// outgoing Connect) may wait.
func TestOpenAGWPE_DoesNotPoll(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")

	var calls int
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("connection refused")
		}
		return &agwpe.TNCPort{}, nil // would succeed on a second attempt, if one were made
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	start := time.Now()
	_, err := a.openAGWPE()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("openAGWPE() error = nil, want the single failed dial's error (it must not retry internally)")
	}
	if calls != 1 {
		t.Errorf("openAGWPE() made %d dial attempts, want exactly 1 (no internal polling)", calls)
	}
	if elapsed >= launchPollBudget {
		t.Errorf("openAGWPE() took %s, want well under launchPollBudget (%s) since it must not poll", elapsed, launchPollBudget)
	}
}

func TestOpenAGWPE_LaunchCmdFailsToStart(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("connection refused")
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{
		LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")},
	}}}

	_, err := a.openAGWPE()
	if !errors.Is(err, wantErr) {
		t.Fatalf("openAGWPE() error = %v, want the original dial error %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch agwpe"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch agwpe", got, logs.String())
	}
}

func TestInitAGWPEForConnect_NeverBecomesReachable_LogsOnce(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1") // stay alive for the whole poll budget

	wantErr := errors.New("connection refused")
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	err := a.initAGWPEForConnect()
	if !errors.Is(err, wantErr) {
		t.Fatalf("initAGWPEForConnect() error = %v, want %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch agwpe"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch agwpe", got, logs.String())
	}
}

// When launch_cmd fails to even start (as opposed to starting but never
// becoming reachable), nothing was ever created, so waitForAGWPE must give
// up immediately rather than polling out the budget — a definitive signal,
// unlike "our child process exited" (see the no-early-break test below).
func TestInitAGWPEForConnect_LaunchCmdFailsToStart_FailsFastAndLogsOnce(t *testing.T) {
	resetLaunchTestState(t)
	launchPollBudget = 2 * time.Second // generous, so a "burned the budget" bug is unambiguous

	wantErr := errors.New("connection refused")
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{
		LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")},
	}}}

	start := time.Now()
	err := a.initAGWPEForConnect()
	elapsed := time.Since(start)

	if !errors.Is(err, wantErr) {
		t.Fatalf("initAGWPEForConnect() error = %v, want %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch agwpe"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch agwpe", got, logs.String())
	}
	if elapsed >= launchPollBudget {
		t.Errorf("initAGWPEForConnect() took %s (the full budget), want it to fail immediately since cmd.Start() itself failed", elapsed)
	}
}

// waitForAGWPE must NOT give up just because the process it spawned has
// exited: some launch_cmd scripts daemonize (a short-lived parent forks the
// real daemon and exits), so an exited child is not reliable evidence the
// daemon isn't still coming up.
func TestWaitForAGWPE_DoesNotBreakEarlyWhenProcessExits(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	// No BLOCK/SLEEP env: the helper exits almost immediately.

	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	start := time.Now()
	if _, err := a.waitForAGWPE(); err == nil {
		t.Fatal("expected an error since dialAGWPE always fails")
	}
	elapsed := time.Since(start)

	if elapsed < launchPollBudget {
		t.Errorf("waitForAGWPE() took only %s, want it to poll the full launchPollBudget (%s) despite the child exiting quickly", elapsed, launchPollBudget)
	}
}

func TestOpenAGWPE_InFlight_NoDuplicateSpawn(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1") // stay alive across both calls

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	if _, err := a.openAGWPE(); err == nil {
		t.Fatal("expected an error since the daemon never becomes reachable")
	}
	if _, err := a.openAGWPE(); err == nil {
		t.Fatal("expected an error on the second call too")
	}

	waitForMarkerCount(t, markerFile, 1)
}

// The sequential test above can't catch a check-then-spawn race, since
// sequential calls trivially serialize regardless of whether startLaunch is
// atomic. This drives many concurrent openAGWPE() calls at once — the
// realistic case being a Listen retry tick overlapping a user-triggered
// Connect on the same App — to catch a duplicate spawn under -race.
func TestOpenAGWPE_ConcurrentCalls_NoDuplicateSpawn(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1")

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			a.openAGWPE()
		}()
	}
	wg.Wait()

	waitForMarkerCount(t, markerFile, 1)
}

func TestOpenAGWPE_RelaunchesAfterProcessExits(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	// No BLOCK env: the helper exits immediately, so it gets reaped quickly.

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, errors.New("connection refused") // never reachable, keep it simple
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{LaunchCmd: helperLaunchCmd()}}}
	t.Cleanup(a.terminateLaunchedProcesses)

	if _, err := a.openAGWPE(); err == nil {
		t.Fatal("expected an error since the daemon never becomes reachable")
	}
	waitUntilNotInFlight(t, a, agwpeLaunchName)

	if _, err := a.openAGWPE(); err == nil {
		t.Fatal("expected an error on the second call too")
	}

	waitForMarkerCount(t, markerFile, 2)
}

// AX25AGWPEListener.Init() has no AGWPE-specific logic of its own — it
// delegates directly to (*App).AGWPE(), which is exactly what initAGWPE()
// (exercised above) backs. This test only pins that delegation, confirming
// the listener surfaces initAGWPE()'s error unchanged rather than swallowing
// or wrapping it.
func TestAX25AGWPEListener_Init_DelegatesToInitAGWPE(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("connection refused")
	dialAGWPE = func(addr string, port int, callsign string) (*agwpe.TNCPort, error) {
		return nil, wantErr
	}

	a := &App{config: cfg.Config{AGWPE: cfg.AGWPEConfig{}}} // LaunchCmd unconfigured
	l := &AX25AGWPEListener{a: a}

	_, err := l.Init()
	if !errors.Is(err, wantErr) {
		t.Fatalf("AX25AGWPEListener.Init() error = %v, want it to wrap %v", err, wantErr)
	}
	if a.isLaunchInFlight(agwpeLaunchName) {
		t.Error("AX25AGWPEListener.Init() spawned a launch when LaunchCmd was unconfigured")
	}
}
