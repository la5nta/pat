package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/editor"
)

func ConfigureHandle(ctx context.Context, a *app.App, args []string) {
	cancel := exitOnContextCancellation(ctx)
	defer cancel()

	// Ensure config file has been written
	config, err := app.ReadConfig(a.Options().ConfigPath)
	if os.IsNotExist(err) {
		err = app.WriteConfig(cfg.DefaultConfig, a.Options().ConfigPath)
		if err != nil {
			log.Fatalf("Unable to write default config: %s", err)
		}
	}

	if err != nil || config.MyCall == "" || config.Locator == "" {
		fmt.Println("Hello there! Do you want to be guided through the basic setup?")
		ans := strings.ToLower(prompt(fmt.Sprintf("Run '%s init'?", os.Args[0]), "Y", "n"))
		if ans == "y" || ans == "yes" {
			InitHandle(ctx, a, args)
			return
		}
	}

	if err := editor.Open(a.Options().ConfigPath); err != nil {
		log.Fatalf("Unable to start editor: %s", err)
	}
}
