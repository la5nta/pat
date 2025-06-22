// Copyright 2022 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifySignals() <-chan os.Signal {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP)
	return sig
}

func isSIGHUP(s os.Signal) bool { return s == syscall.SIGHUP }
