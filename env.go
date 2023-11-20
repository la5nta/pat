package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/la5nta/pat/internal/buildinfo"
)

func envHandle(_ context.Context, _ []string) {
	writeEnvAll(os.Stdout)
}

func writeEnvAll(w io.Writer) {
	fmt.Fprintln(w, strings.Join(envAll(), "\n"))
}

func envAll() []string {
	return []string{
		`PAT_MYCALL="` + fOptions.MyCall + `"`,
		`PAT_LOCATOR="` + config.Locator + `"`,
		`PAT_VERSION="` + buildinfo.Version + `"`,
		`PAT_ARCH="` + runtime.GOARCH + `"`,
		`PAT_OS="` + runtime.GOOS + `"`,
		`PAT_MAILBOX_PATH="` + fOptions.MailboxPath + `"`,
		`PAT_CONFIG_PATH="` + fOptions.ConfigPath + `"`,
		`PAT_LOG_PATH="` + fOptions.LogPath + `"`,
		`PAT_EVENTLOG_PATH="` + fOptions.EventLogPath + `"`,
		`PAT_FORMS_PATH="` + fOptions.FormsPath + `"`,
		`PAT_DEBUG="` + os.Getenv("PAT_DEBUG") + `"`,
		`PAT_WEB_DEV_ADDR="` + os.Getenv("PAT_WEB_DEV_ADDR") + `"`,

		`ARDOP_DEBUG="` + os.Getenv("ARDOP_DEBUG") + `"`,
		`PACTOR_DEBUG="` + os.Getenv("PACTOR_DEBUG") + `"`,
		`AGWPE_DEBUG="` + os.Getenv("AGWPE_DEBUG") + `"`,
		`VARA_DEBUG="` + os.Getenv("VARA_DEBUG") + `"`,

		`GZIP_EXPERIMENT="` + os.Getenv("GZIP_EXPERIMENT") + `"`,
		`ARDOP_FSKONLY_EXPERIMENT="` + os.Getenv("ARDOP_FSKONLY_EXPERIMENT") + `"`,
	}
}
