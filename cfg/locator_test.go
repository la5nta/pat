package cfg

import (
	"testing"
)

func TestMaidenheadLocatorValid(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"JP20", false},
		{"jp20", false},
		{"JP20qe", true},
		{"jp20qh", true},
		{"JP20QE", true},
		{"AA00", false},
		{"RR99", false},
		{"AA00aa", true},
		{"RR99xx", true},
		{"XX99xx", false}, // X is not valid for first pair (A-R only)
		{"", false},
		{"JP2", false},
		{"JP20q", false},
		{"JP20qex", false},
		{"JP20qe12", false},
		{"S", false},
		{"JP90qe", true},  // J and P are valid (A-R), 9 and 0 are digits
		{"SP20qe", false}, // S is not valid (A-R only)
		{"JP20ry", false}, // y is not valid (A-X only)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := MaidenheadLocator(tt.input).Valid(); got != tt.valid {
				t.Errorf("Valid(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestMaidenheadLocatorString(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{"JP20", "JP20"},
		{"jp20", "JP20"},
		{"jp20qh", "JP20QH"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := MaidenheadLocator(tt.input).String(); got != tt.output {
				t.Errorf("String(%q) = %q, want %q", tt.input, got, tt.output)
			}
		})
	}
}
