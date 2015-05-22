// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fiorix/go-readline"
	"github.com/la5nta/wl2k-go/transport/ax25"
)

func Interactive() {
	for {
		prompt := getPrompt()
		line := readline.Readline(&prompt)
		if line == nil {
			break
		}

		if quit := execCmd(*line); quit {
			break
		}
		readline.AddHistory(*line)
	}
}

func execCmd(line string) (quit bool) {
	cmd, param := parseCommand(line)
	switch cmd {
	case "connect":
		Connect(param)
	case "listen":
		Listen(param)
	case "unlisten":
		Unlisten(param)
	case "heard":
		PrintHeard()
	case "freq":
		freq(param)
	case "q", "quit":
		return true
	case "":
		return
	default:
		printInteractiveUsage()
	}
	return
}

func printInteractiveUsage() {
	fmt.Println("Uri examples: 'LA3F@5350', 'LA1B-10 v LA5NTA-1', 'LA5NTA:secret@192.168.1.1:54321'")

	methods := []string{
		MethodWinmor,
		MethodAX25,
		MethodTelnet,
		MethodSerialTNC,
	}
	fmt.Println("Methods:", strings.Join(methods, ", "))

	cmds := []string{
		"connect  [METHOD]:[URI] Connect to a remote station.",
		"listen   METHOD         Listen for incoming connections.",
		"unlisten METHOD         Unregister listener for incoming connections.",
		"freq     METHOD:FREQ    Change rig frequency.",
		"heard                   Display all stations heard over the air.",
	}
	fmt.Println("Commands: ")
	for _, cmd := range cmds {
		fmt.Printf(" %s\n", cmd)
	}
}

func getPrompt() string {
	var buf bytes.Buffer

	methods := make([]string, 0, len(listeners))
	for method, _ := range listeners {
		methods = append(methods, method)
	}

	if len(listeners) > 0 {
		sort.Strings(methods)
		fmt.Fprintf(&buf, "L%v", methods)
	}

	fmt.Fprint(&buf, "> ")
	return buf.String()
}

func PrintHeard() {
	pf := func(call string, t time.Time) {
		fmt.Printf("  %-10s (%s)\n", call, t.Format(time.RFC1123))
	}

	fmt.Println("winmor:")
	if wmTNC == nil {
		fmt.Println("  (not initialized)")
	} else if heard := wmTNC.Heard(); len(heard) == 0 {
		fmt.Println("  (none)")
	} else {
		for call, t := range heard {
			pf(call, t)
		}
	}

	fmt.Println("ax25:")
	if heard, err := ax25.Heard(config.AX25.Port); err != nil {
		fmt.Printf("  (%s)\n", err)
	} else if len(heard) == 0 {
		fmt.Println("  (none)")
	} else {
		for call, t := range heard {
			pf(call, t)
		}
	}
}

func parseCommand(str string) (mode, param string) {
	parts := strings.SplitN(str, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
