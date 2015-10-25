// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

// CRC-16-CCITT (Reversed reciprocal, 0x8810 polynomial) with 0xffff initial seed

const polynomial = 0x8810

func crc16Sum(data []byte) (sum uint16) {
	sum = 0xffff // Initial seed

	for _, b := range data {
		// For each bit, processing most significant bit first
		for mask := uint16(0x80); mask > 0; mask >>= 1 {
			divisible := (sum & 0x8000) != 0 // Most significant bit is set

			// Shift left
			sum <<= 1

			// Bring current data bit onto least significant bit of sum
			dataBit := uint16(b) & mask
			if dataBit != 0 {
				sum += 1
			}

			// Divide
			if divisible {
				sum ^= polynomial
			}
		}
	}

	return sum
}
