// Copyright 2022 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

//go:build windows

package main

import (
	"os"
	"os/signal"
)

func notifySignals(sig chan<- os.Signal) {
	signal.Notify(sig, os.Interrupt)
}

func isSIGHUP(s os.Signal) bool { return false }
