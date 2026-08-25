package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/la5nta/pat/cfg"
)

// A config that only sets launch_cmd (no addr) is legal per
// cfg.VaraConfig.UnmarshalJSON and must still receive its default Addr, not
// be treated as "fully configured, leave it alone" — otherwise the daemon
// launch_cmd starts up successfully but Pat dials an empty address and can
// never connect to it.
func TestLoadConfig_LaunchCmdOnly_StillGetsDefaultAddr(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{
		"agwpe": {"launch_cmd": {"path": "direwolf"}},
		"varahf": {"launch_cmd": {"path": "vara"}},
		"varafm": {"launch_cmd": {"path": "vara"}}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(cfgPath, cfg.DefaultConfig)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config.AGWPE.Addr != cfg.DefaultConfig.AGWPE.Addr {
		t.Errorf("AGWPE.Addr = %q, want default %q", config.AGWPE.Addr, cfg.DefaultConfig.AGWPE.Addr)
	}
	if config.AGWPE.LaunchCmd.Path != "direwolf" {
		t.Errorf("AGWPE.LaunchCmd.Path = %q, want %q", config.AGWPE.LaunchCmd.Path, "direwolf")
	}

	if config.VaraHF.Addr != cfg.DefaultConfig.VaraHF.Addr {
		t.Errorf("VaraHF.Addr = %q, want default %q", config.VaraHF.Addr, cfg.DefaultConfig.VaraHF.Addr)
	}
	if config.VaraHF.LaunchCmd.Path != "vara" {
		t.Errorf("VaraHF.LaunchCmd.Path = %q, want %q", config.VaraHF.LaunchCmd.Path, "vara")
	}

	if config.VaraFM.Addr != cfg.DefaultConfig.VaraFM.Addr {
		t.Errorf("VaraFM.Addr = %q, want default %q", config.VaraFM.Addr, cfg.DefaultConfig.VaraFM.Addr)
	}
	if config.VaraFM.LaunchCmd.Path != "vara" {
		t.Errorf("VaraFM.LaunchCmd.Path = %q, want %q", config.VaraFM.LaunchCmd.Path, "vara")
	}
}

func TestReadRigsFromEnv(t *testing.T) {
	const prefix = "PAT_HAMLIB_RIGS"
	unset := func() {
		for _, env := range os.Environ() {
			key, _, _ := strings.Cut(env, "=")
			if strings.HasPrefix(key, prefix) {
				os.Unsetenv(key)
			}
		}
	}
	t.Run("simple", func(t *testing.T) {
		defer unset()
		var rigs map[string]cfg.HamlibConfig
		os.Setenv(prefix+"_rig", "localhost:4532")
		if err := readRigsFromEnv(&rigs); err != nil {
			t.Fatal(err)
		}
		if got := rigs["rig"]; (got != cfg.HamlibConfig{Address: "localhost:4532"}) {
			t.Fatalf("Got unexpected config: %#v", got)
		}
	})
	t.Run("with VFO", func(t *testing.T) {
		defer unset()
		var rigs map[string]cfg.HamlibConfig
		os.Setenv(prefix+"_rig", "localhost:4532")
		os.Setenv(prefix+"_rig_VFO", "A")
		if err := readRigsFromEnv(&rigs); err != nil {
			t.Fatal(err)
		}
		if got := rigs["rig"]; (got != cfg.HamlibConfig{Address: "localhost:4532", VFO: "A"}) {
			t.Fatalf("Got unexpected config: %#v", got)
		}
	})
	t.Run("full", func(t *testing.T) {
		defer unset()
		var rigs map[string]cfg.HamlibConfig
		os.Setenv(prefix+"_rig_ADDRESS", "/dev/ttyS0")
		os.Setenv(prefix+"_rig_NETWORK", "serial")
		os.Setenv(prefix+"_rig_VFO", "B")
		if err := readRigsFromEnv(&rigs); err != nil {
			t.Fatal(err)
		}
		expect := cfg.HamlibConfig{
			Address: "/dev/ttyS0",
			Network: "serial",
			VFO:     "B",
		}
		if got := rigs["rig"]; got != expect {
			t.Fatalf("Got unexpected config: %#v", got)
		}
	})
}
