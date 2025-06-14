package cli

import (
	"github.com/spf13/pflag"
)

func HelpHandle(args []string) {
	// Print usage for the specified command
	arg := args[0]
	for _, cmd := range Commands {
		if cmd.Str == arg {
			cmd.PrintUsage()
			return
		}
	}

	// Fallback to main help text
	pflag.Usage()
}
