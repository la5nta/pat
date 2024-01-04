package forms

import (
	"testing"

	"github.com/la5nta/pat/cfg"
)

func TestInsertionTagReplacer(t *testing.T) {
	m := &Manager{config: Config{
		MyCall: "LA5NTA",
		GPSd:   cfg.GPSdConfig{Addr: gpsMockAddr},
	}}
	tests := map[string]string{
		"<GridSquare>": "JO29PJ",
	}
	for in, expect := range tests {
		if out := insertionTagReplacer(m, "<", ">")(in); out != expect {
			t.Errorf("%s: Expected %q, got %q", in, expect, out)
		}

	}
}
