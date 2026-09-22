package app

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/la5nta/pat/cfg"
	"github.com/n8jja/Pat-Vara/vara"
)

// varaSchemes is used to table-drive tests across both VARA variants, which
// share openVaraModem/waitForVaraModem (an existing, pre-ticket-01
// abstraction — see initVARA — not a new one introduced for launch_cmd).
var varaSchemes = []string{MethodVaraHF, MethodVaraFM}

func TestOpenVaraModem_AlreadyReachable_NoLaunch(t *testing.T) {
	for _, scheme := range varaSchemes {
		t.Run(scheme, func(t *testing.T) {
			resetLaunchTestState(t)

			dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
				return &vara.Modem{}, nil
			}

			a := &App{}
			conf := cfg.VaraConfig{LaunchCmd: cfg.LaunchCmd{Path: "/should-not-be-exec'd"}}

			m, err := a.openVaraModem(scheme, conf)
			if err != nil {
				t.Fatalf("openVaraModem() error = %v, want nil", err)
			}
			if m == nil {
				t.Fatal("openVaraModem() returned nil modem on success")
			}
			if a.isLaunchInFlight(scheme) {
				t.Error("openVaraModem() spawned a launch when the modem was already reachable")
			}
		})
	}
}

func TestOpenVaraModem_Unreachable_NoLaunchCmd_IsNoOp(t *testing.T) {
	for _, scheme := range varaSchemes {
		t.Run(scheme, func(t *testing.T) {
			resetLaunchTestState(t)

			wantErr := errors.New("connection refused")
			dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
				return nil, wantErr
			}

			a := &App{}
			_, err := a.openVaraModem(scheme, cfg.VaraConfig{}) // LaunchCmd unconfigured
			if !errors.Is(err, wantErr) {
				t.Fatalf("openVaraModem() error = %v, want %v", err, wantErr)
			}
			if a.isLaunchInFlight(scheme) {
				t.Error("openVaraModem() spawned a launch when LaunchCmd was unconfigured")
			}
		})
	}
}

func TestInitVARA_WrapsDialError(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("boom")
	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, wantErr
	}

	a := &App{}
	_, err := a.initVARA(MethodVaraHF, cfg.VaraConfig{})
	if err == nil {
		t.Fatal("initVARA() error = nil, want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("initVARA() error = %v, does not wrap %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "vara initialization failed") {
		t.Errorf("initVARA() error = %q, want it to contain %q", err.Error(), "vara initialization failed")
	}
}

// Deliberately tests waitForVaraModem directly rather than the full
// initVARA*ForConnect chain: a successful outcome there would go on to call
// configureVaraModem, which invokes real *vara.Modem methods
// (SetBusyFunc/Version) that need a fully handshaked modem and would hang
// on the bare fake &vara.Modem{} dialVara returns in tests — the same
// protocol-simulation cost avoided for Ardop via waitForARDOP.
func TestWaitForVaraModem_LaunchesAndBecomesReachableWithinBudget(t *testing.T) {
	for _, scheme := range varaSchemes {
		t.Run(scheme, func(t *testing.T) {
			resetLaunchTestState(t)
			t.Setenv("PAT_TEST_HELPER_PROCESS", "1")

			var calls int
			dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
				calls++
				if calls <= 2 {
					return nil, errors.New("connection refused")
				}
				return &vara.Modem{}, nil
			}

			logs := captureLog(t)
			a := &App{}
			conf := cfg.VaraConfig{LaunchCmd: helperLaunchCmd()}

			m, err := a.waitForVaraModem(scheme, conf)
			if err != nil {
				t.Fatalf("waitForVaraModem() error = %v, want nil", err)
			}
			if m == nil {
				t.Fatal("waitForVaraModem() returned nil modem on success")
			}
			if calls < 2 {
				t.Errorf("expected at least one retry dial after the initial failure, got %d total calls", calls)
			}
			if strings.Contains(logs.String(), "Pat failed to launch") {
				t.Errorf("unexpected failure log on a successful launch: %q", logs.String())
			}
		})
	}
}

// A bare openVaraModem() call (as used directly by the Listen path via
// initVARA()) must NOT itself poll/wait for a freshly launched daemon —
// that would turn ListenerHub's 1s retry cadence into launchPollBudget per
// tick. Only waitForVaraModem (used by the *ForConnect wrappers) may wait.
func TestOpenVaraModem_DoesNotPoll(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")

	var calls int
	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("connection refused")
		}
		return &vara.Modem{}, nil // would succeed on a second attempt, if one were made
	}

	a := &App{}
	t.Cleanup(a.terminateLaunchedProcesses)

	start := time.Now()
	_, err := a.openVaraModem(MethodVaraHF, cfg.VaraConfig{LaunchCmd: helperLaunchCmd()})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("openVaraModem() error = nil, want the single failed dial's error (it must not retry internally)")
	}
	if calls != 1 {
		t.Errorf("openVaraModem() made %d dial attempts, want exactly 1 (no internal polling)", calls)
	}
	if elapsed >= launchPollBudget {
		t.Errorf("openVaraModem() took %s, want well under launchPollBudget (%s) since it must not poll", elapsed, launchPollBudget)
	}
}

func TestOpenVaraModem_LaunchCmdFailsToStart(t *testing.T) {
	resetLaunchTestState(t)

	wantErr := errors.New("connection refused")
	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")}}

	_, err := a.openVaraModem(MethodVaraFM, conf)
	if !errors.Is(err, wantErr) {
		t.Fatalf("openVaraModem() error = %v, want the original dial error %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch varafm"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch varafm", got, logs.String())
	}
}

// Confirms the two VARA variants produce distinctly named failure warnings,
// not a generic "vara" message.
func TestWaitForVaraModem_LogsAreTransportSpecific(t *testing.T) {
	cases := []struct {
		scheme string
		want   string
	}{
		{MethodVaraHF, "Pat failed to launch varahf"},
		{MethodVaraFM, "Pat failed to launch varafm"},
	}
	for _, c := range cases {
		t.Run(c.scheme, func(t *testing.T) {
			resetLaunchTestState(t)

			wantErr := errors.New("connection refused")
			dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
				return nil, wantErr
			}

			logs := captureLog(t)
			a := &App{}
			conf := cfg.VaraConfig{LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")}}

			if _, err := a.openVaraModem(c.scheme, conf); !errors.Is(err, wantErr) {
				t.Fatalf("openVaraModem() error = %v, want %v", err, wantErr)
			}
			if !strings.Contains(logs.String(), c.want) {
				t.Errorf("expected log to contain %q, got %q", c.want, logs.String())
			}
		})
	}
}

func TestWaitForVaraModem_NeverBecomesReachable_LogsOnce(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1") // stay alive for the whole poll budget

	wantErr := errors.New("connection refused")
	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: helperLaunchCmd()}
	t.Cleanup(a.terminateLaunchedProcesses)

	_, err := a.waitForVaraModem(MethodVaraHF, conf)
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitForVaraModem() error = %v, want %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch varahf"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch varahf", got, logs.String())
	}
}

// When launch_cmd fails to even start, nothing was ever created, so
// waitForVaraModem must give up immediately rather than polling out the
// budget — a definitive signal, unlike "our child process exited" (see the
// no-early-break test below).
func TestWaitForVaraModem_LaunchCmdFailsToStart_FailsFastAndLogsOnce(t *testing.T) {
	resetLaunchTestState(t)
	launchPollBudget = 2 * time.Second // generous, so a "burned the budget" bug is unambiguous

	wantErr := errors.New("connection refused")
	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, wantErr
	}

	logs := captureLog(t)
	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: cfg.LaunchCmd{Path: filepath.Join(t.TempDir(), "does-not-exist")}}

	start := time.Now()
	_, err := a.waitForVaraModem(MethodVaraFM, conf)
	elapsed := time.Since(start)

	if !errors.Is(err, wantErr) {
		t.Fatalf("waitForVaraModem() error = %v, want %v", err, wantErr)
	}
	if got := strings.Count(logs.String(), "Pat failed to launch varafm"); got != 1 {
		t.Errorf("expected exactly one %q log line, got %d: %q", "Pat failed to launch varafm", got, logs.String())
	}
	if elapsed >= launchPollBudget {
		t.Errorf("waitForVaraModem() took %s (the full budget), want it to fail immediately since cmd.Start() itself failed", elapsed)
	}
}

// waitForVaraModem must NOT give up just because the process it spawned has
// exited — see the identical concern (and regression story) documented on
// TestWaitForARDOP_DoesNotBreakEarlyWhenProcessExits.
func TestWaitForVaraModem_DoesNotBreakEarlyWhenProcessExits(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	// No BLOCK/SLEEP env: the helper exits almost immediately.

	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: helperLaunchCmd()}
	t.Cleanup(a.terminateLaunchedProcesses)

	start := time.Now()
	if _, err := a.waitForVaraModem(MethodVaraHF, conf); err == nil {
		t.Fatal("expected an error since dialVara always fails")
	}
	elapsed := time.Since(start)

	if elapsed < launchPollBudget {
		t.Errorf("waitForVaraModem() took only %s, want it to poll the full launchPollBudget (%s) despite the child exiting quickly", elapsed, launchPollBudget)
	}
}

func TestOpenVaraModem_InFlight_NoDuplicateSpawn(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1") // stay alive across both calls

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: helperLaunchCmd()}
	t.Cleanup(a.terminateLaunchedProcesses)

	if _, err := a.openVaraModem(MethodVaraHF, conf); err == nil {
		t.Fatal("expected an error since the daemon never becomes reachable")
	}
	if _, err := a.openVaraModem(MethodVaraHF, conf); err == nil {
		t.Fatal("expected an error on the second call too")
	}

	waitForMarkerCount(t, markerFile, 1)
}

// Sequential calls can't catch a check-then-spawn race, since sequential
// calls trivially serialize regardless of whether startLaunch is atomic —
// see the identical concern on TestOpenARDOP_ConcurrentCalls_NoDuplicateSpawn.
func TestOpenVaraModem_ConcurrentCalls_NoDuplicateSpawn(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	t.Setenv("PAT_TEST_HELPER_BLOCK", "1")

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, errors.New("connection refused") // never reachable
	}

	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: helperLaunchCmd()}
	t.Cleanup(a.terminateLaunchedProcesses)

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			a.openVaraModem(MethodVaraFM, conf)
		}()
	}
	wg.Wait()

	waitForMarkerCount(t, markerFile, 1)
}

func TestOpenVaraModem_RelaunchesAfterProcessExits(t *testing.T) {
	resetLaunchTestState(t)
	t.Setenv("PAT_TEST_HELPER_PROCESS", "1")
	// No BLOCK env: the helper exits immediately, so it gets reaped quickly.

	markerFile := filepath.Join(t.TempDir(), "spawned.log")
	t.Setenv("PAT_TEST_HELPER_MARKER_FILE", markerFile)

	dialVara = func(scheme, myCall string, conf vara.ModemConfig) (*vara.Modem, error) {
		return nil, errors.New("connection refused") // never reachable, keep it simple
	}

	a := &App{}
	conf := cfg.VaraConfig{LaunchCmd: helperLaunchCmd()}
	t.Cleanup(a.terminateLaunchedProcesses)

	if _, err := a.openVaraModem(MethodVaraHF, conf); err == nil {
		t.Fatal("expected an error since the daemon never becomes reachable")
	}
	waitUntilNotInFlight(t, a, MethodVaraHF)

	if _, err := a.openVaraModem(MethodVaraHF, conf); err == nil {
		t.Fatal("expected an error on the second call too")
	}

	waitForMarkerCount(t, markerFile, 2)
}
