// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package fbb

import "testing"

func TestSecureLoginResponse(t *testing.T) {
	var (
		challenge = "23753528"
		password  = "foobar"
		expect    = "72768415"
	)

	if got := secureLoginResponse(challenge, password); got != expect {
		t.Errorf("Got unexpected login response, expected '%s' got '%s'.", expect, got)
	}
}

func BenchmarkSecureLoginResponse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		secureLoginResponse("23753528", "foobar")
	}
}
