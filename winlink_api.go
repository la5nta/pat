package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/cmsapi"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
)

// doIfElapsed implements a per-callsign rate limited function.
func doIfElapsed(name string, t time.Duration, fn func() error) error {
	filePath := filepath.Join(directories.StateDir(), "."+name+"_"+fOptions.MyCall+".json")
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	var lastUpdated time.Time
	json.NewDecoder(file).Decode(&lastUpdated)
	if since := time.Since(lastUpdated); since < t {
		debug.Printf("Skipping %q (last run: %s ago)", name, since.Truncate(time.Minute))
		return nil
	}

	if err := fn(); err != nil {
		return err
	}

	file.Truncate(0)
	file.Seek(0, 0)
	return json.NewEncoder(file).Encode(time.Now())
}

func postVersionUpdate() {
	const interval = 24 * time.Hour
	err := doIfElapsed("version_report", interval, func() error {
		debug.Printf("Posting version update...")
		// WDT do not want us to post version reports for callsigns without a registered account
		if exists, err := accountExists(fOptions.MyCall); err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("account does not exist")
		}
		return cmsapi.VersionAdd{
			Callsign: fOptions.MyCall,
			Program:  buildinfo.AppName,
			Version:  buildinfo.Version,
			Comments: fmt.Sprintf("%s - %s/%s", buildinfo.GitRev, runtime.GOOS, runtime.GOARCH),
		}.Post()
	})
	if err != nil {
		debug.Printf("Failed to post version update: %v", err)
	}
}

func checkPasswordRecoveryEmailIsSet(ctx context.Context) {
	const interval = 14 * 24 * time.Hour
	err := doIfElapsed("pw_recovery_email_check", interval, func() error {
		debug.Printf("Checking if winlink.org password recovery email is set...")
		set, err := passwordRecoveryEmailSet(ctx)
		if err != nil {
			return err
		}
		debug.Printf("Password recovery email set: %t", set)
		if set {
			return nil
		}
		fmt.Println("")
		fmt.Println("WINLINK NOTICE: Password recovery email is not set for your Winlink account. It is highly recommended to do so.")
		fmt.Println("Run `" + os.Args[0] + " account --help` for help setting your recovery address. You can also manage your account settings at https://winlink.org/.")
		fmt.Println("")
		return nil
	})
	if err != nil {
		debug.Printf("Failed to check if password recovery email is set: %v", err)
	}
}

func passwordRecoveryEmailSet(ctx context.Context) (bool, error) {
	if config.SecureLoginPassword == "" {
		return false, fmt.Errorf("missing password")
	}
	switch exists, err := accountExists(fOptions.MyCall); {
	case err != nil:
		return false, fmt.Errorf("error checking if account exist: %w", err)
	case !exists:
		return false, fmt.Errorf("account does not exist")
	}
	email, err := cmsapi.PasswordRecoveryEmailGet(ctx, fOptions.MyCall, config.SecureLoginPassword)
	return email != "", err
}

func accountExists(callsign string) (bool, error) {
	var cache struct {
		Expires       time.Time
		AccountExists bool
	}

	fileName := fmt.Sprintf(".cached_account_check_%s.json", callsign)
	filePath := filepath.Join(directories.StateDir(), fileName)
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return false, err
	}
	json.NewDecoder(f).Decode(&cache)
	if time.Since(cache.Expires) < 0 {
		return cache.AccountExists, nil
	}
	defer func() {
		f.Truncate(0)
		f.Seek(0, 0)
		json.NewEncoder(f).Encode(cache)
	}()

	debug.Printf("Checking if account exists...")
	exists, err := cmsapi.AccountExists(callsign)
	debug.Printf("Account exists: %t (%v)", exists, err)
	if !exists || err != nil {
		// Let's try again in 48 hours
		cache.Expires = time.Now().Add(48 * time.Hour)
		return false, err
	}

	// Keep this response for a month. It will probably not change.
	cache.Expires = time.Now().Add(30 * 24 * time.Hour)
	cache.AccountExists = exists
	return exists, err
}
