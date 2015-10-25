// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import "testing"

func TestCRC16Sum(t *testing.T) {
	tests := map[string]uint16{
		"RDY\r":                  55805,
		"voluptatem accusantium": 24749,
		"hagavik":                44843,

		"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor": 50066,
	}

	for data, expected := range tests {
		got := crc16Sum([]byte(data))
		if got != expected {
			t.Errorf("'%s' crc16 checksum failed.", data)
		}
	}
}
