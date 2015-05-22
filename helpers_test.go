// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import "testing"

func TestErrLine(t *testing.T) {
	err := errLine("*** Unable to decompress received binary compressed message - Disconnecting (88.89.220.254)")
	if err == nil {
		t.Errorf("Expected error, got nil")
	} else if err.Error() != "Unable to decompress received binary compressed message - Disconnecting (88.89.220.254)" {
		t.Errorf("Unexpected error message, got '%s'", err)
	}

	err = errLine("FF ***")
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err)
	}

	err = errLine("* foobar")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}

	err = errLine("*")
	if err != nil {
		t.Errorf("Expected no error, got non nil")
	}
}
