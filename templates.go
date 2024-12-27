package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
)

const (
	templatesUsage = `subcommand [option ...]

subcommands:
  update             Update standard Winlink form templates.
  seqset [number]    Set the template sequence value.
`
	templatesExample = `
  update             Download the latest form templates from winlink.org.
  seqset 0           Reset the current sequence value to 0.
`
)

func templatesHandle(ctx context.Context, args []string) {
	switch cmd, args := shiftArgs(args); cmd {
	case "update":
		if _, err := formsMgr.UpdateFormTemplates(ctx); err != nil {
			log.Printf("%v", err)
		}
	case "seqset":
		v, err := strconv.Atoi(args[0])
		if err != nil {
			log.Printf("invalid sequence number: %q", args[0])
			return
		}
		if err := formsMgr.SeqSet(v); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Missing argument, try 'templates help'.")
	}
}
