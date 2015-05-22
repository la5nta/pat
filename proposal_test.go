// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"reflect"
	"testing"
)

func TestParseProposal(t *testing.T) {
	tests := map[string]Proposal{
		"FC EM TJKYEIMMHSRB 527 123 0": Proposal{
			code:           Wl2kProposal,
			msgType:        "EM",
			mid:            "TJKYEIMMHSRB",
			offset:         0,
			size:           527,
			compressedSize: 123,
		},
	}

	for input, expected := range tests {
		got := Proposal{}
		err := parseProposal(input, &got)
		if err != nil {
			t.Errorf("Got unexpected error while parsing proposal '%s': %s", input, err)
		} else if !reflect.DeepEqual(got, expected) {
			t.Errorf("Got %#v, expected %#v while parsing '%s'", got, expected, input)
		}
	}
}
