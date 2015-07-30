// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"bytes"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

// ToBytes converts the Body into a slice of bytes with the given charset encoding.
//
// If crlf is true all newlines are re-written as CRLF.
// bug(martinhpedersen): We should limit line length to 1000 characters (including CRLF).
func StringToBody(str, encoding string, crlf bool) ([]byte, error) {
	utf8 := []byte(str)

	if crlf {
		var buf bytes.Buffer
		for i, b := range utf8 {
			if b == '\n' && (i == 0 || utf8[i-1] != '\r') {
				buf.WriteByte('\r')
			}
			buf.WriteByte(b)
		}
		utf8 = buf.Bytes()
	}

	translator, err := charset.TranslatorTo(encoding)
	if err != nil {
		return utf8, err
	}

	_, data, err := translator.Translate(utf8, true)
	if err != nil {
		return utf8, err
	}

	return data, nil
}

// BodyFromBytes translated the data based on the given charset encoding into a proper utf-8 string.
func BodyFromBytes(data []byte, encoding string) (string, error) {
	translator, err := charset.TranslatorFrom(encoding)
	if err != nil {
		return string(data), err
	}

	_, utf8, err := translator.Translate(data, true)
	return string(utf8), err
}
