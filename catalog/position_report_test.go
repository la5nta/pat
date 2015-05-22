// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package catalog

import (
	"os"
	"testing"
	"time"
)

func TestDecToDM(t *testing.T) {
	latTests := map[float64]string{
		60.132: "60-7.9200N",
		-4.974: "04-58.4400S",
	}
	lonTests := map[float64]string{
		003.50: "003-30.0000E",
		153.50: "153-30.0000E",
		-60.50: "060-30.0000W",
	}

	for deg, expect := range latTests {
		if got := decToMinDec(deg, true); got != expect {
			t.Errorf("On input %f, expected %s got %s", deg, expect, got)
		}
	}
	for deg, expect := range lonTests {
		if got := decToMinDec(deg, false); got != expect {
			t.Errorf("On input %f, expected %s got %s", deg, expect, got)
		}
	}
}

func ExamplePosReport_Message() {
	lat := 60.18
	lon := 5.3972

	posRe := PosReport{
		Date:    time.Now(),
		Lat:     &lat,
		Lon:     &lon,
		Comment: "Hjemme QTH",
	}
	msg := posRe.Message("N0CALL")
	msg.WriteTo(os.Stdout)
}
