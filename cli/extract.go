package cli

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/la5nta/pat/app"
)

func ExtractMessageHandle(_ context.Context, app *app.App, args []string) {
	if len(args) == 0 || args[0] == "" {
		fmt.Println("Missing argument, try 'extract help'.")
		os.Exit(1)
	}

	msg, err := openMessage(app, args[0])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(msg)
	for _, f := range msg.Files() {
		if err := os.WriteFile(f.Name(), f.Data(), 0o664); err != nil {
			log.Fatal(err)
		}
	}
}
