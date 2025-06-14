package cli

import (
	"context"
	"log"
	"os"

	"github.com/la5nta/pat/api"
	"github.com/la5nta/pat/app"

	"github.com/spf13/pflag"
)

func HTTPHandle(ctx context.Context, a *app.App, args []string) {
	addr := a.Config().HTTPAddr
	if addr == "" {
		addr = ":8080" // For backwards compatibility (remove in future)
	}

	set := pflag.NewFlagSet("http", pflag.ExitOnError)
	set.StringVarP(&addr, "addr", "a", addr, "Listen address.")
	set.Parse(args)

	if addr == "" {
		set.Usage()
		os.Exit(1)
	}

	scheduleLoop(ctx, a)

	if err := api.ListenAndServe(ctx, a, addr); err != nil {
		log.Println(err)
	}
}
