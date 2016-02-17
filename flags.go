// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

var ErrNoCmd = fmt.Errorf("No CMD")

type Command struct {
	Str        string
	Aliases    []string
	Desc       string
	HandleFunc func(args []string)
	Usage      string
	Options    map[string]string
	Example    string

	LongLived  bool
	MayConnect bool
}

func (cmd Command) PrintUsage() {
	fmt.Fprintf(os.Stderr, "%s - %s\n", cmd.Str, cmd.Desc)

	fmt.Fprintf(os.Stderr, "\nUsage:\n  %s %s\n", cmd.Str, strings.TrimSpace(cmd.Usage))

	if len(cmd.Options) > 0 {
		fmt.Fprint(os.Stderr, "\nOptions:\n")
		for f, desc := range cmd.Options {
			fmt.Fprintf(os.Stderr, "   %-17s %s\n", f, desc)
		}
	}

	if cmd.Example != "" {
		fmt.Fprintf(os.Stderr, "\nExample:\n  %s\n", strings.TrimSpace(cmd.Example))
	}

	fmt.Fprint(os.Stderr, "\n")
}

func parseFlags(args []string) (cmd Command, arguments []string) {
	var options []string
	var err error
	cmd, options, arguments, err = findCommand(args)
	if err != nil {
		pflag.Usage()
		os.Exit(1)
	}

	optionsSet().Parse(options)

	if len(arguments) == 0 {
		arguments = append(arguments, "")
	}

	switch arguments[0] {
	case "--help", "-help", "help", "-h":
		cmd.PrintUsage()
		os.Exit(1)
	}

	return
}

func findCommand(args []string) (cmd Command, pre, post []string, err error) {
	cmdMap := make(map[string]Command, len(commands))
	for _, c := range commands {
		cmdMap[c.Str] = c
		for _, alias := range c.Aliases {
			cmdMap[alias] = c
		}
	}

	for i, arg := range args {
		if cmd, ok := cmdMap[arg]; ok {
			return cmd, args[1:i], args[i+1:], nil
		}
	}
	err = ErrNoCmd
	return
}
