// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

type frame interface{}

type dFrame struct {
	dataType string
	data     []byte
}

func (f dFrame) ARQFrame() bool { return f.dataType == "ARQ" }
func (f dFrame) FECFrame() bool { return f.dataType == "FEC" }
func (f dFrame) ErrFrame() bool { return f.dataType == "ERR" }
func (f dFrame) IDFFrame() bool { return f.dataType == "IDF" }

type cmdFrame string

func (f cmdFrame) Parsed() ctrlMsg { return parseCtrlMsg(string(f)) }

func writeCtrlFrame(w io.Writer, format string, params ...interface{}) error {
	payload := fmt.Sprintf(format+"\r", params...)
	if _, err := fmt.Fprint(w, "C:"+payload); err != nil {
		return err
	}

	sum := crc16Sum([]byte(payload))
	return binary.Write(w, binary.BigEndian, sum)
}

func readFrame(reader *bufio.Reader) (frame, error) {
	fType, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	reader.Discard(1) // :

	var data []byte
	switch fType {
	case 'c':
		data, err = reader.ReadBytes('\r')
	case 'd':
		// Peek length
		peeked, err := reader.Peek(2)
		if err != nil {
			return nil, err
		}
		length := binary.BigEndian.Uint16(peeked) + 2 // +2 to include the length bytes

		// actual data
		data = make([]byte, length)
		var n int
		for read := 0; read < int(length) && err == nil; {
			n, err = reader.Read(data[read:])
			read += n
		}
	default:
		return nil, fmt.Errorf("Unexpected frame type %c", fType)
	}

	if err != nil {
		return nil, err
	}

	// Verify CRC sums
	sumBytes := make([]byte, 2)
	reader.Read(sumBytes)
	crc := binary.BigEndian.Uint16(sumBytes)
	if crc16Sum(data) != crc {
		return nil, ErrChecksumMismatch
	}

	switch fType {
	case 'c':
		data = data[:len(data)-1] // Trim \r
		return cmdFrame(string(data)), nil
	case 'd':
		return dFrame{dataType: string(data[2:5]), data: data[5:]}, nil
	default:
		panic("not possible")
	}
}
