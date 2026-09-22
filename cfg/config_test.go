package cfg

import (
	"encoding/json"
	"testing"
)

func TestArdopConfig_LaunchCmd_JSONRoundTrip(t *testing.T) {
	want := ArdopConfig{
		Addr:      "localhost:8515",
		LaunchCmd: LaunchCmd{Path: "ardopcf", Args: []string{"--webgui", "8514"}},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got ArdopConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.LaunchCmd.Path != want.LaunchCmd.Path {
		t.Errorf("LaunchCmd.Path = %q, want %q", got.LaunchCmd.Path, want.LaunchCmd.Path)
	}
	if len(got.LaunchCmd.Args) != len(want.LaunchCmd.Args) {
		t.Fatalf("LaunchCmd.Args = %v, want %v", got.LaunchCmd.Args, want.LaunchCmd.Args)
	}
	for i, arg := range want.LaunchCmd.Args {
		if got.LaunchCmd.Args[i] != arg {
			t.Errorf("LaunchCmd.Args[%d] = %q, want %q", i, got.LaunchCmd.Args[i], arg)
		}
	}
}

func TestArdopConfig_LaunchCmd_ZeroValueIsNoOp(t *testing.T) {
	var got ArdopConfig
	if err := json.Unmarshal([]byte(`{"addr":"localhost:8515"}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.LaunchCmd.Path != "" {
		t.Errorf("LaunchCmd.Path = %q, want empty string when launch_cmd is omitted from config", got.LaunchCmd.Path)
	}
}

func TestAGWPEConfig_LaunchCmd_JSONRoundTrip(t *testing.T) {
	want := AGWPEConfig{
		Addr:      "localhost:8000",
		LaunchCmd: LaunchCmd{Path: "direwolf", Args: []string{"-c", "direwolf.conf"}},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got AGWPEConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.LaunchCmd.Path != want.LaunchCmd.Path {
		t.Errorf("LaunchCmd.Path = %q, want %q", got.LaunchCmd.Path, want.LaunchCmd.Path)
	}
	if len(got.LaunchCmd.Args) != len(want.LaunchCmd.Args) {
		t.Fatalf("LaunchCmd.Args = %v, want %v", got.LaunchCmd.Args, want.LaunchCmd.Args)
	}
	for i, arg := range want.LaunchCmd.Args {
		if got.LaunchCmd.Args[i] != arg {
			t.Errorf("LaunchCmd.Args[%d] = %q, want %q", i, got.LaunchCmd.Args[i], arg)
		}
	}
}

func TestAGWPEConfig_LaunchCmd_ZeroValueIsNoOp(t *testing.T) {
	var got AGWPEConfig
	if err := json.Unmarshal([]byte(`{"addr":"localhost:8000"}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.LaunchCmd.Path != "" {
		t.Errorf("LaunchCmd.Path = %q, want empty string when launch_cmd is omitted from config", got.LaunchCmd.Path)
	}
}

func TestVaraConfig_LaunchCmd_JSONRoundTrip(t *testing.T) {
	want := VaraConfig{
		Addr:      "localhost:8300",
		LaunchCmd: LaunchCmd{Path: "vara", Args: []string{"-p", "8300"}},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got VaraConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.LaunchCmd.Path != want.LaunchCmd.Path {
		t.Errorf("LaunchCmd.Path = %q, want %q", got.LaunchCmd.Path, want.LaunchCmd.Path)
	}
	if len(got.LaunchCmd.Args) != len(want.LaunchCmd.Args) {
		t.Fatalf("LaunchCmd.Args = %v, want %v", got.LaunchCmd.Args, want.LaunchCmd.Args)
	}
	for i, arg := range want.LaunchCmd.Args {
		if got.LaunchCmd.Args[i] != arg {
			t.Errorf("LaunchCmd.Args[%d] = %q, want %q", i, got.LaunchCmd.Args[i], arg)
		}
	}
}

func TestVaraConfig_LaunchCmd_ZeroValueIsNoOp(t *testing.T) {
	var got VaraConfig
	if err := json.Unmarshal([]byte(`{"addr":"localhost:8300"}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.LaunchCmd.Path != "" {
		t.Errorf("LaunchCmd.Path = %q, want empty string when launch_cmd is omitted from config", got.LaunchCmd.Path)
	}
}

// Regression test: VaraConfig.IsZero() used to compare with == , which
// would fail to compile once LaunchCmd (containing a slice) was added.
// Confirms the reflect.DeepEqual replacement still has correct semantics.
func TestVaraConfig_IsZero(t *testing.T) {
	if !(VaraConfig{}).IsZero() {
		t.Error("VaraConfig{}.IsZero() = false, want true")
	}
	if (VaraConfig{LaunchCmd: LaunchCmd{Path: "vara"}}).IsZero() {
		t.Error("VaraConfig with only LaunchCmd set .IsZero() = true, want false")
	}
	if (VaraConfig{Addr: "localhost:8300"}).IsZero() {
		t.Error("VaraConfig with only Addr set .IsZero() = true, want false")
	}
}

// Regression test: UnmarshalJSON used to reject a config that sets only
// launch_cmd (no addr) with "invalid addr format", because its validation
// guard used IsZero() as a proxy for "was addr given" — but IsZero() is
// false as soon as any field is set, including LaunchCmd.
func TestVaraConfig_UnmarshalJSON_LaunchCmdOnly_NoAddrRequired(t *testing.T) {
	var got VaraConfig
	err := json.Unmarshal([]byte(`{"launch_cmd":{"path":"vara","args":["-p","8300"]}}`), &got)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil for a launch_cmd-only config", err)
	}
	if got.LaunchCmd.Path != "vara" {
		t.Errorf("LaunchCmd.Path = %q, want %q", got.LaunchCmd.Path, "vara")
	}
	if got.Addr != "" {
		t.Errorf("Addr = %q, want empty (not provided)", got.Addr)
	}
}

// Malformed addr must still be rejected.
func TestVaraConfig_UnmarshalJSON_InvalidAddr(t *testing.T) {
	var got VaraConfig
	err := json.Unmarshal([]byte(`{"addr":"not-a-valid-addr"}`), &got)
	if err == nil {
		t.Fatal("Unmarshal() error = nil, want an error for a malformed addr")
	}
}
