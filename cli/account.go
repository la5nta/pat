package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/internal/cmsapi"
)

const (
	AccountUsage = `property [value]

properties:
  password.recovery.email
`
	AccountExample = `
  account password.recovery.email                 Get your current password recovery email for winlink.org.
  account password.recovery.email me@example.com  Set your password recovery email to for winlink.org to "me@example.com".
`
)

func AccountHandle(ctx context.Context, app *app.App, args []string) {
	switch cmd, args := shiftArgs(args); cmd {
	case "password.recovery.email":
		if err := passwordRecoveryEmailHandle(ctx, app, args); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Missing argument, try 'account help'.")
	}
}

// getPasswordForCallsign gets the password for the specified callsign
// It tries the configured SecureLoginPassword first, then prompts if not available
func getPasswordForCallsign(ctx context.Context, a *app.App, callsign string) string {
	password := a.Config().SecureLoginPassword
	if password != "" {
		return password
	}

	select {
	case <-ctx.Done():
		return ""
	case resp := <-a.PromptHub().Prompt(ctx, app.PromptKindPassword, "Enter account password for "+callsign):
		if resp.Err != nil {
			log.Printf("Password prompt error: %v", resp.Err)
			return ""
		}
		return resp.Value
	}
}

func passwordRecoveryEmailHandle(ctx context.Context, a *app.App, args []string) error {
	mycall := a.Options().MyCall
	password := getPasswordForCallsign(ctx, a, mycall)

	arg, _ := shiftArgs(args)
	if arg != "" {
		if err := cmsapi.PasswordRecoveryEmailSet(ctx, mycall, password, arg); err != nil {
			return fmt.Errorf("failed to set value: %w", err)
		}
	}
	email, err := cmsapi.PasswordRecoveryEmailGet(ctx, mycall, password)
	switch {
	case err != nil:
		return fmt.Errorf("failed to get value: %w", err)
	case strings.TrimSpace(email) == "":
		email = "[not set]"
	}
	fmt.Println(email)
	return nil
}
