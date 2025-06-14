package app

import (
	"os"
	"strings"
	"testing"

	"github.com/la5nta/pat/cfg"
)

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
