package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/la5nta/pat/internal/cmsapi"
)

func shiftArgs(s []string) (string, []string) {
	if len(s) == 0 {
		return "", nil
	}
	return strings.TrimSpace(s[0]), s[1:]
}

const (
	accountUsage = `property [value]

properties:
  password.recovery.email
`
	accountExample = `
  account password.recovery.email                 Get your current password recovery email for winlink.org.
  account password.recovery.email me@example.com  Set your password recovery email to for winlink.org to "me@example.com".
`
)

func accountHandle(ctx context.Context, args []string) {
	switch cmd, args := shiftArgs(args); cmd {
	case "password.recovery.email":
		if err := passwordRecoveryEmailHandle(ctx, args); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	default:
		fmt.Println("Missing argument, try 'account help'.")
	}
}

func passwordRecoveryEmailHandle(ctx context.Context, args []string) error {
	mycall, password := fOptions.MyCall, config.SecureLoginPassword
	if password == "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp := <-promptHub.Prompt(PromptKindPassword, "Enter account password for "+mycall):
			if resp.Err != nil {
				return resp.Err
			}
			password = resp.Value
		}
	}
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
