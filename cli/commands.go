package cli

import (
	"context"
	"log"

	"github.com/la5nta/pat/app"
)

var Commands = []app.Command{
	{
		Str:        "init",
		Desc:       "Initial configuration setup.",
		HandleFunc: InitHandle,
		Usage:      "Interactive basic setup with Winlink account verification.",
	},
	{
		Str:        "configure",
		Desc:       "Open configuration file for editing.",
		HandleFunc: ConfigureHandle,
	},
	{
		Str:        "connect",
		Desc:       "Connect to a remote station.",
		HandleFunc: ConnectHandle,
		Usage:      UsageConnect,
		Example:    ExampleConnect,
		MayConnect: true,
	},
	{
		Str:   "interactive",
		Desc:  "Run interactive mode.",
		Usage: "[options]",
		Options: map[string]string{
			"--http, -h": "Start http server for web UI in the background.",
		},
		HandleFunc: InteractiveHandle,
		MayConnect: true,
		LongLived:  true,
	},
	{
		Str:   "http",
		Desc:  "Run http server for web UI.",
		Usage: "[options]",
		Options: map[string]string{
			"--addr, -a": "Listen address. Default is :8080.",
		},
		HandleFunc: HTTPHandle,
		MayConnect: true,
		LongLived:  true,
	},
	{
		Str:  "compose",
		Desc: "Compose a new message.",
		Usage: "[options]\n" +
			"\tIf no options are passed, composes interactively.\n" +
			"\tIf options are passed, reads message from stdin similar to mail(1).",
		Options: map[string]string{
			"--from, -r":        "Address to send from. Default is your call from config or --mycall, but can be specified to use tactical addresses.",
			"--forward":         "Forward given message (full path or mid)",
			"--in-reply-to":     "Compose in reply to given message (full path or mid)",
			"--reply-all":       "Reply to all (only applicable in combination with --in-reply-to)",
			"--template":        "Compose using template file. Uses the --forms directory as root for relative paths.",
			"--subject, -s":     "Subject",
			"--attachment , -a": "Attachment path (may be repeated)",
			"--cc, -c":          "CC Address(es) (may be repeated)",
			"--p2p-only":        "Send over peer to peer links only (avoid CMS)",
			"":                  "Recipient address (may be repeated)",
		},
		HandleFunc: ComposeMessage,
	},
	{
		Str:        "read",
		Desc:       "Read messages.",
		HandleFunc: ReadHandle,
	},
	{
		Str:     "composeform",
		Aliases: []string{"formPath"},
		Desc:    "Post form-based report. (DEPRECATED)",
		Usage:   "[options]",
		Options: map[string]string{
			"--template": "path to the form template file. Uses the --forms directory as root. Defaults to 'ICS USA Forms/ICS213.txt'",
		},
		HandleFunc: func(ctx context.Context, app *app.App, args []string) {
			log.Println("DEPRECATED: Use `compose --template` instead")
			if len(args) == 0 || args[0] == "" {
				args = []string{"ICS USA Forms/ICS213.txt"}
			}
			ComposeMessage(ctx, app, append([]string{"--template"}, args...))
		},
	},
	{
		Str:     "position",
		Aliases: []string{"pos"},
		Desc:    "Post a position report (GPSd or manual entry).",
		Usage:   "[options]",
		Options: map[string]string{
			"--latlon":      "latitude,longitude in decimal degrees for manual entry. Will use GPSd if this is empty.",
			"--comment, -c": "Comment to be included in the position report.",
		},
		Example:    ExamplePosition,
		HandleFunc: PositionHandle,
	},
	{
		Str:        "extract",
		Desc:       "Extract attachments from a message file.",
		Usage:      "[full path or mid]",
		HandleFunc: ExtractMessageHandle,
	},
	{
		Str:   "rmslist",
		Desc:  "Print/search in list of RMS nodes.",
		Usage: "[options] [search term]",
		Options: map[string]string{
			"--mode, -m":              "Mode filter.",
			"--band, -b":              "Band filter (e.g. '80m').",
			"--force-download, -d":    "Force download of latest list from winlink.org.",
			"--sort-distance, -s":     "Sort by distance",
			"--sort-link-quality, -q": "Sort by predicted link quality (requires VOACAP)",
		},
		HandleFunc: RMSListHandle,
	},
	{
		Str:  "updateforms",
		Desc: "Download the latest form templates. (DEPRECATED)",
		HandleFunc: func(ctx context.Context, a *app.App, args []string) {
			log.Println("DEPRECATED: Use `templates update` instead")
			TemplatesHandle(ctx, a, []string{"update"})
		},
	},
	{
		Str:        "templates",
		Desc:       "Manage message templates and HTML forms.",
		Usage:      TemplatesUsage,
		Example:    TemplatesExample,
		HandleFunc: TemplatesHandle,
	},
	{
		Str:        "account",
		Desc:       "Get and set Winlink.org account settings.",
		Usage:      AccountUsage,
		Example:    AccountExample,
		HandleFunc: AccountHandle,
	},
	{
		Str:        "mps",
		Desc:       "Manage message pickup stations.",
		Usage:      MPSUsage,
		Example:    MPSExample,
		HandleFunc: MPSHandle,
	},
	{
		Str:   "version",
		Desc:  "Print the application version.",
		Usage: "[options]",
		Options: map[string]string{
			"--check, -c":   "Check if a new version is available",
			"--verbose, -v": "Show detailed build information",
		},
		HandleFunc: VersionHandle,
	},
	{
		Str:        "env",
		Desc:       "List environment variables.",
		HandleFunc: EnvHandle,
	},
	{
		Str:  "help",
		Desc: "Print detailed help for a given command.",
		// Avoid initialization loop by invoking helpHandler in main
	},
}

func FindCommand(args []string) (cmd app.Command, pre, post []string, err error) {
	cmdMap := make(map[string]app.Command, len(Commands))
	for _, c := range Commands {
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
	err = app.ErrNoCmd
	return
}
