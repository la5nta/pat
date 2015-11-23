// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package fbb

import "testing"

func TestParseProposalAnswer(t *testing.T) {
	tests := map[string][]*Proposal{
		"FS YLA3350RH": []*Proposal{
			&Proposal{answer: Accept}, // Y
			&Proposal{answer: Defer},  // L
			&Proposal{
				answer: Accept,
				offset: 3350,
			}, // A3350
			&Proposal{answer: Reject}, // R
			&Proposal{answer: Accept}, // H
		},
		"FS +=!3350-+": []*Proposal{
			&Proposal{answer: Accept}, // +
			&Proposal{answer: Defer},  // =
			&Proposal{
				answer: Accept,
				offset: 3350,
			}, // !3350
			&Proposal{answer: Reject}, // -
			&Proposal{answer: Accept}, // +
		},
	}

	for input, expected := range tests {
		got := []*Proposal{&Proposal{}, &Proposal{}, &Proposal{}, &Proposal{}, &Proposal{}}

		if err := parseProposalAnswer(input, got, nil); err != nil {
			t.Fatalf("Got error from parser func: %s", err)
		}
		for i, exp := range expected {
			if exp.answer != got[i].answer {
				t.Errorf("Test %d: expected %c got %c", i, exp.answer, got[i].answer)
			}
		}
	}
}
