// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

//go:build libhamlib
// +build libhamlib

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
)

func init() {
	cmd := Command{
		Str:        "riglist",
		Usage:      "[search term]",
		Desc:       "Print/search a list of rigcontrol supported transceivers.",
		HandleFunc: riglistHandle,
	}

	commands = append(commands[:8], append([]Command{cmd}, commands[8:]...)...)
}

func riglistHandle(ctx context.Context, args []string) {
	if args[0] == "" {
		fmt.Println("Missing argument")
	}
	term := strings.ToLower(args[0])

	fmt.Print("id\ttransceiver\n")
	for m, str := range hamlib.Rigs() {
		if !strings.Contains(strings.ToLower(str), term) {
			continue
		}
		fmt.Printf("%d\t%s\n", m, str)
	}
}
