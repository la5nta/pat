package forms

import (
	"fmt"
	"testing"
)

func TestPlaceholderReplacer(t *testing.T) {
	tests := []struct {
		In       string
		Replacer func(string) string
		Expect   string
	}{
		{
			In:       "<MyKey>",
			Replacer: placeholderReplacer("<", ">", map[string]string{"mykey": "foobar"}),
			Expect:   "foobar",
		},
		{
			In:       "<mykey>",
			Replacer: placeholderReplacer("<", ">", map[string]string{"MyKey": "foobar"}),
			Expect:   "foobar",
		},
		{
			In:       "<   mykey   \t>",
			Replacer: placeholderReplacer("<", ">", map[string]string{"MyKey": "foobar"}),
			Expect:   "foobar",
		},
		{
			In:       "<var      MyKey>",
			Replacer: placeholderReplacer("<Var", ">", map[string]string{"mykey": "foobar"}),
			Expect:   "foobar",
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if got := tt.Replacer(tt.In); got != tt.Expect {
				t.Errorf("Expected %q, got %q", tt.Expect, got)
			}

		})
	}
}
