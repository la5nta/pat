// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package fbb

import (
	"crypto/md5"
	"fmt"
	"strings"
)

// This salt was found in paclink-unix's source code.
var winlinkSecureSalt = []byte{
	77, 197, 101, 206, 190, 249,
	93, 200, 51, 243, 93, 237,
	71, 94, 239, 138, 68, 108,
	70, 185, 225, 137, 217, 16,
	51, 122, 193, 48, 194, 195,
	198, 175, 172, 169, 70, 84,
	61, 62, 104, 186, 114, 52,
	61, 168, 66, 129, 192, 208,
	187, 249, 232, 193, 41, 113,
	41, 45, 240, 16, 29, 228,
	208, 228, 61, 20}

// This algorithm for generating a secure login response token has been ported
// to Go from the paclink-unix implementation.
func secureLoginResponse(challenge, password string) string {
	payload := strings.ToUpper(challenge+password) + string(winlinkSecureSalt)

	sum := md5.Sum([]byte(payload))

	pr := int32(sum[3] & 0x3f)
	for i := 2; i >= 0; i-- {
		pr = (pr << 8) | int32(sum[i])
	}

	str := fmt.Sprintf("%08d", pr)

	return str[len(str)-8:]
}
