// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build libax25

package ax25

//#include <time.h>
//#include <netax25/axlib.h>
//#include <netax25/mheard.h>
import "C"

import (
	"io"
	"os"
	"strings"
	"time"
	"unsafe"
)

const MheardDataFile = "/var/ax25/mheard/mheard.dat"

// Heard returns all stations heard via the given axport.
//
// This function parses the content of MhardDataFile which
// is normally written by mheardd. The mheardd daemon must
// be running on the system to record heard stations.
func Heard(axPort string) (map[string]time.Time, error) {
	var mheard C.struct_mheard_struct

	f, err := os.Open(MheardDataFile)
	if err != nil {
		return nil, err
	}

	heard := make(map[string]time.Time)
	for {
		data := (*(*[999]byte)(unsafe.Pointer(&mheard)))[0:unsafe.Sizeof(mheard)]
		if _, err := f.Read(data); err == io.EOF {
			break
		} else if err != nil {
			return heard, err
		}

		port := C.GoString((*C.char)(unsafe.Pointer(&mheard.portname[0])))
		if !strings.EqualFold(port, axPort) {
			continue
		}

		from := (*C.ax25_address)(unsafe.Pointer(&mheard.from_call))
		fromAddr := AddressFromString(C.GoString(C.ax25_ntoa(from)))
		t := time.Unix(int64(mheard.last_heard), 0)

		heard[fromAddr.String()] = t
	}

	return heard, f.Close()
}
