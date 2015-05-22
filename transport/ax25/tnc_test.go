// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ax25

import "testing"

func TestTncAddrFromString(t *testing.T) {
	tAddrParse(t, tncAddrFromString("LA5NTA-2 v LA1B-10"), "LA5NTA-2 via LA1B-10")
	tAddrParse(t, tncAddrFromString("LA5NTA"), "LA5NTA")
}

func tAddrParse(t *testing.T, a tncAddr, expect string) {
	ax25Addr := AX25Addr{a}
	if ax25Addr.String() != expect {
		t.Errorf("Expected '%s', got '%s'.", expect, ax25Addr)
	}
}
