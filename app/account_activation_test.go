// Copyright 2021 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import "testing"

func TestIsAccountActivationMessage(t *testing.T) {
	msg := mockNewAccountMsg()
	isActivation, password := isAccountActivationMessage(msg)
	if !isActivation {
		t.Errorf("Expected isActivation to be true, but was false")
	}
	if password != "K1CHN7" {
		t.Errorf("Expected password to be 'K1CHN7', but was '%s'", password)
	}
}
