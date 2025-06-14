// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"os"
	"strings"
)

var ErrNoCmd = fmt.Errorf("no cmd")

type Command struct {
	Str        string
	Aliases    []string
	Desc       string
	HandleFunc func(ctx context.Context, app *App, args []string)
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
