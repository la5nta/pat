// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"crypto/md5"
	"encoding/base32"
	"fmt"
	"time"
)

const MaxMIDLength = 12

// Generates a unique message ID in the format specified by the protocol.
func GenerateMid(callsign string) string {
	sum := md5.Sum(midPayload(callsign, time.Now()))
	return base32.StdEncoding.EncodeToString(sum[0:])[0:MaxMIDLength]
}

func midPayload(callsign string, t time.Time) []byte {
	return []byte(fmt.Sprintf("%s-%s", time.Now(), callsign))
}
