package app

import (
	"os"
	"runtime"

	"github.com/la5nta/pat/internal/buildinfo"
)

func (app *App) Env() []string {
	return []string{
		`PAT_MYCALL="` + app.options.MyCall + `"`,
		`PAT_LOCATOR="` + app.config.Locator + `"`,
		`PAT_VERSION="` + buildinfo.Version + `"`,
		`PAT_ARCH="` + runtime.GOARCH + `"`,
		`PAT_OS="` + runtime.GOOS + `"`,
		`PAT_MAILBOX_PATH="` + app.options.MailboxPath + `"`,
		`PAT_CONFIG_PATH="` + app.options.ConfigPath + `"`,
		`PAT_LOG_PATH="` + app.options.LogPath + `"`,
		`PAT_EVENTLOG_PATH="` + app.options.EventLogPath + `"`,
		`PAT_FORMS_PATH="` + app.options.FormsPath + `"`,
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
