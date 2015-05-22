// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package lzhuf implements the lzhuf compression used by the binary FBB protocols B, B1 and B2.
//
// The compression is LZHUF with a CRC16 checksum of the compressed data prepended (as expected in FBB).
//
package lzhuf

// #cgo CFLAGS: -DLZHUF=1 -DB2F=1
// #include "lzhuf.h"
import "C"

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
)

//TODO:
// * Modify lzhuf.c's Encode() so we don't have to use temp files.
// * Handle go errors.
func encDec(data []byte, encode bool) []byte {
	// Create temp files
	outf, _ := ioutil.TempFile("", "lzhuf")
	inf, _ := ioutil.TempFile("", "lzhuf")
	defer func() {
		outf.Close()
		os.Remove(outf.Name())
		os.Remove(inf.Name())
	}()

	// Copy data to in file
	io.Copy(inf, bytes.NewBuffer(data))
	inf.Sync()
	inf.Close()

	// Encode/Decode the inf to outf
	lzs := C.AllocStruct()
	var retval C.int
	if encode {
		retval = C.Encode(0, C.CString(inf.Name()), C.CString(outf.Name()), lzs, 1)
	} else {
		retval = C.Decode(0, C.CString(inf.Name()), C.CString(outf.Name()), lzs, 1)
	}
	C.FreeStruct(lzs)

	if retval < 0 {
		panic("lzhuf encode/decode error")
	}

	// Read the compressed/decompressed data from outf
	b, _ := ioutil.ReadAll(outf)

	return b
}

// Function for decoding a slice of lzhuf-compressed
// bytes.
//
// Returns the decoded data.
//
func Decode(data []byte) []byte {
	return encDec(data, false)
}

// Function for encoding arbitrary data with
// the lzhuf compression.
//
// Returns the encoded data
//
func Encode(data []byte) []byte {
	return encDec(data, true)
}
