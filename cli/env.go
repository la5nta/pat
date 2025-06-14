package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/la5nta/pat/app"
)

func EnvHandle(_ context.Context, app *app.App, _ []string) {
	fmt.Println(strings.Join(app.Env(), "\n"))
}
