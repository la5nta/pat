// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := map[string]ctrlMsg{
		"NEWSTATE DISC":                     ctrlMsg{cmdNewState, Disconnected},
		"PTT True":                          ctrlMsg{cmdPTT, true},
		"PTT False":                         ctrlMsg{cmdPTT, false},
		"PTT trUE":                          ctrlMsg{cmdPTT, true},
		"CODEC True":                        ctrlMsg{cmdCodec, true},
		"foobar baz":                        ctrlMsg{Command("FOOBAR"), nil},
		"RDY":                               ctrlMsg{cmdReady, nil},
		"DISCONNECTED":                      ctrlMsg{cmdDisconnected, nil},
		"FAULT 5/Error in the application.": ctrlMsg{cmdFault, "5/Error in the application."},
		"BUFFER 300":                        ctrlMsg{cmdBuffer, 300},
		"MYCALL LA5NTA":                     ctrlMsg{cmdMyCall, "LA5NTA"},
		"GRIDSQUARE JP20QH":                 ctrlMsg{cmdGridSquare, "JP20QH"},
		"MYAUX LA5NTA,LE3OF":                ctrlMsg{cmdMyAux, []string{"LA5NTA", "LE3OF"}},
		"MYAUX LA5NTA, LE3OF":               ctrlMsg{cmdMyAux, []string{"LA5NTA", "LE3OF"}},
		"VERSION 1.4.7.0":                   ctrlMsg{cmdVersion, "1.4.7.0"},
	}
	for input, expected := range tests {
		got := parseCtrlMsg(input)
		if got.cmd != expected.cmd {
			t.Errorf("Got %#v expected %#v when parsing '%s'", got.cmd, expected.cmd, input)
		}
		if !reflect.DeepEqual(got.value, expected.value) {
			t.Errorf("Got %#v expected %#v when parsing '%s'", got.value, expected.value, input)
		}
	}
}
