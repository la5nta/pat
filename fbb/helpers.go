// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package fbb

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

type ByDate []*Message

func (d ByDate) Len() int           { return len(d) }
func (d ByDate) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d ByDate) Less(i, j int) bool { return d[i].Date().Before(d[j].Date()) }

func ReadLine(rd io.Reader) (string, error) {
	var lineBuffer bytes.Buffer

	for {
		buf := make([]byte, 1)
		n, err := rd.Read(buf)
		if err != nil {
			return ``, err
		} else if n < 1 {
			continue
		}

		if buf[0] == '\n' || buf[0] == '\r' {
			if lineBuffer.Len() > 0 {
				return cleanString(string(lineBuffer.Bytes())), nil
			}
			continue
		} else {
			lineBuffer.WriteByte(buf[0])
		}
	}
}

func (s *Session) nextLineRemoteErr(parseErr bool) (string, error) {
	line, err := s.rd.ReadString('\r')
	if err != nil {
		return line, err
	}

	line = cleanString(line)
	s.pLog.Println(line)

	if err := errLine(line); parseErr && err != nil {
		return "", err
	} else {
		return line, nil
	}
}

func (s *Session) nextLine() (string, error) {
	return s.nextLineRemoteErr(true)
}

func errLine(str string) error {
	if len(str) == 0 || str[0] != '*' {
		return nil
	}

	idx := strings.LastIndex(str, "*")
	if idx+1 >= len(str) {
		return nil
	}

	return fmt.Errorf(strings.TrimSpace(str[idx+1:]))
}

func cleanString(str string) string {
	str = strings.TrimSpace(str)
	if len(str) < 1 {
		return str
	}
	if str[0] == byte(0) {
		str = str[1:]
	}
	if str[len(str)-1] == byte(0) {
		str = str[0 : len(str)-2]
	}
	return str
}
