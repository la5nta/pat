// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"bufio"
	"bytes"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

// StringToBytes converts the body into a slice of bytes with the given charset encoding.
//
// CRLF line break is enforced.
// Line break are inserted if a line is longer than 1000 characters (including CRLF).
func StringToBody(str, encoding string) ([]byte, error) {
	in := bufio.NewScanner(bytes.NewBufferString(str))
	out := new(bytes.Buffer)

	var err error
	var line []byte
	for in.Scan() {
		line = in.Bytes()
		for {
			// Lines can not be longer that 1000 characters including CRLF.
			n := min(len(line), 1000-2)

			out.Write(line[:n])
			out.WriteString("\r\n")

			line = line[n:]
			if len(line) == 0 {
				break
			}
		}
	}

	return out.Bytes(), err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
