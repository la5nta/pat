// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"bytes"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

// Body is a UTF-8 string representation of a Message's body (textual content).
type Body string

// ToBytes converts the Body into a slice of bytes with the given charset encoding.
//
// If crlf is true all newlines are re-written as CRLF.
func (b Body) ToBytes(encoding string, crlf bool) ([]byte, error) {
	utf8 := []byte(b)

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

// BodyFromBytes translated the data based on the given charset encoding into a proper Body.
func BodyFromBytes(data []byte, encoding string) (Body, error) {
	translator, err := charset.TranslatorFrom(encoding)
	if err != nil {
		return Body(data), err
	}

	_, utf8, err := translator.Translate(data, true)
	if err != nil {
		return Body(data), err
	}

	return Body(utf8), nil
}
