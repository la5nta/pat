package forms

import (
	"fmt"
	"testing"
)

func TestReplaceSelect(t *testing.T) {
	tests := []struct {
		In, Expect string
		Answer     func(Select) Option
	}{
		{
			In:     "",
			Expect: "",
			Answer: nil,
		},
		{
			In:     "foobar",
			Expect: "foobar",
			Answer: nil,
		},
		{
			In:     `Subj: //WL2K <Select Prioritet:,Routine=R/,Priority=P/,Immediate=O/,Flash=Z/> <Callsign>/<SeqNum> - <Var Subject>`,
			Expect: `Subj: //WL2K R/ <Callsign>/<SeqNum> - <Var Subject>`,
			Answer: func(s Select) Option { return s.Options[0] },
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			got := promptSelects(tt.In, tt.Answer)
			if got != tt.Expect {
				t.Errorf("Expected %q, got %q", tt.Expect, got)
			}
		})
	}
}
