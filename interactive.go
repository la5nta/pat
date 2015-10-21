// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/peterh/liner"
)

func Interactive() {
	line := liner.NewLiner()
	defer line.Close()

	for {
		str, _ := line.Prompt(getPrompt())
		if str == "" {
			continue
		}
		line.AppendHistory(str)

		if str[0] == '#' {
			continue
		}

		if quit := execCmd(str); quit {
			break
		}
	}
}

func execCmd(line string) (quit bool) {
	cmd, param := parseCommand(line)
	switch cmd {
	case "connect":
		if param == "" {
			printInteractiveUsage()
			return
		}

		Connect(param)
	case "listen":
		Listen(param)
	case "unlisten":
		Unlisten(param)
	case "heard":
		PrintHeard()
	case "freq":
		freq(param)
	case "qtc":
		PrintQTC()
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
		MethodArdop,
		MethodAX25,
		MethodTelnet,
		MethodSerialTNC,
	}
	fmt.Println("Methods:", strings.Join(methods, ", "))

	cmds := []string{
		"connect  METHOD:[URI] or alias  Connect to a remote station.",
		"listen   METHOD                 Listen for incoming connections.",
		"unlisten METHOD                 Unregister listener for incoming connections.",
		"freq     METHOD:FREQ            Change rig frequency.",
		"heard                           Display all stations heard over the air.",
		"qtc                             Print pending outbound messages.",
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

func PrintQTC() {
	msgs, err := mbox.Outbox()
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Printf("QTC: %d.\n", len(msgs))
	for _, msg := range msgs {
		fmt.Printf(`%-12.12s (%s): %s`, msg.MID(), msg.Subject(), fmt.Sprint(msg.To()))
		if msg.Header.Get("X-P2POnly") == "true" {
			fmt.Printf(" (P2P only)")
		}
		fmt.Println("")
	}
}

func parseCommand(str string) (mode, param string) {
	parts := strings.SplitN(str, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
