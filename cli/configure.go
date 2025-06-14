package cli

import (
	"context"
	"log"
	"os"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/editor"
)

func ConfigureHandle(ctx context.Context, a *app.App, args []string) {
	// Ensure config file has been written
	_, err := app.ReadConfig(a.Options().ConfigPath)
	if os.IsNotExist(err) {
		err = app.WriteConfig(cfg.DefaultConfig, a.Options().ConfigPath)
		if err != nil {
			log.Fatalf("Unable to write default config: %s", err)
		}
	}
	if err := editor.Open(a.Options().ConfigPath); err != nil {
		log.Fatalf("Unable to start editor: %s", err)
	}
}
