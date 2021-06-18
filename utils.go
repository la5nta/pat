// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"runtime"
	"unicode"
)

func SplitFunc(c rune) bool {
	return unicode.IsSpace(c) || c == ',' || c == ';'
}

var PatUserAgent = fmt.Sprintf("%v/%v (%v) %v (%v; %v)",
	AppName, Version, GitRev, runtime.Version(), runtime.GOOS, runtime.GOARCH)
