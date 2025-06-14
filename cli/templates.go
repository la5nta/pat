package cli

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/la5nta/pat/app"
)

const (
	TemplatesUsage = `subcommand [option ...]

subcommands:
  update             Update standard Winlink form templates.
  seqset [number]    Set the template sequence value.
`
	TemplatesExample = `
  update             Download the latest form templates from winlink.org.
  seqset 0           Reset the current sequence value to 0.
`
)

func shiftArgs(s []string) (string, []string) {
	if len(s) == 0 {
		return "", nil
	}
	return strings.TrimSpace(s[0]), s[1:]
}

func TemplatesHandle(ctx context.Context, app *app.App, args []string) {
	switch cmd, args := shiftArgs(args); cmd {
	case "update":
		if _, err := app.FormsManager().UpdateFormTemplates(ctx); err != nil {
			log.Printf("%v", err)
		}
	case "seqset":
		v, err := strconv.Atoi(args[0])
		if err != nil {
			log.Printf("invalid sequence number: %q", args[0])
			return
		}
		if err := app.FormsManager().SeqSet(v); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Missing argument, try 'templates help'.")
	}
}
