package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/la5nta/pat/internal/buildinfo"
)

func envHandle(_ context.Context, _ []string) {
	writeEnvAll(os.Stdout)
}

func writeEnvAll(w io.Writer) {
	writeEnv(w, "PAT_MYCALL", fOptions.MyCall)
	writeEnv(w, "PAT_LOCATOR", config.Locator)
	writeEnv(w, "PAT_VERSION", buildinfo.Version)
	writeEnv(w, "PAT_ARCH", runtime.GOARCH)
	writeEnv(w, "PAT_OS", runtime.GOOS)
	writeEnv(w, "PAT_MAILBOX_PATH", fOptions.MailboxPath)
	writeEnv(w, "PAT_CONFIG_PATH", fOptions.ConfigPath)
	writeEnv(w, "PAT_LOG_PATH", fOptions.LogPath)
	writeEnv(w, "PAT_EVENTLOG_PATH", fOptions.EventLogPath)
	writeEnv(w, "PAT_FORMS_PATH", fOptions.FormsPath)
	writeEnv(w, "PAT_DEBUG", os.Getenv("PAT_DEBUG"))
	writeEnv(w, "PAT_WEB_DEV_ADDR", os.Getenv("PAT_WEB_DEV_ADDR"))
	writeEnv(w, "ARDOP_DEBUG", os.Getenv("ARDOP_DEBUG"))
	writeEnv(w, "PACTOR_DEBUG", os.Getenv("PACTOR_DEBUG"))
	writeEnv(w, "AGWPE_DEBUG", os.Getenv("AGWPE_DEBUG"))
}

func writeEnv(w io.Writer, k, v string) {
	fmt.Fprintf(w, "%s=\"%s\"\n", k, v)
}
