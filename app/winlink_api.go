package app

import (
	"context"
	"encoding/json"
	"errors"
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

var ErrRateLimited error = errors.New("call was rate-limited")

// DoIfElapsed implements a per-callsign rate limited function.
func DoIfElapsed(callsign, name string, t time.Duration, fn func() error) error {
	filePath := filepath.Join(directories.StateDir(), "."+name+"_"+callsign+".json")
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	var lastUpdated time.Time
	json.NewDecoder(file).Decode(&lastUpdated)
	if since := time.Since(lastUpdated); since < t {
		debug.Printf("Skipping %q (last run: %s ago)", name, since.Truncate(time.Minute))
		return ErrRateLimited
	}

	if err := fn(); err != nil {
		return err
	}

	file.Truncate(0)
	file.Seek(0, 0)
	return json.NewEncoder(file).Encode(time.Now())
}

func (a *App) postVersionUpdate() {
	const interval = 24 * time.Hour
	err := DoIfElapsed(a.Options().MyCall, "version_report", interval, func() error {
		debug.Printf("Posting version update...")
		// WDT do not want us to post version reports for callsigns without a registered account
		if exists, err := accountExists(a.Options().MyCall); err != nil {
			return err
		} else if !exists {
			return fmt.Errorf("account does not exist")
		}
		return cmsapi.VersionAdd{
			Callsign: a.Options().MyCall,
			Program:  buildinfo.AppName,
			Version:  buildinfo.Version,
			Comments: fmt.Sprintf("%s - %s/%s", buildinfo.GitRev, runtime.GOOS, runtime.GOARCH),
		}.Post()
	})
	if err != nil && !errors.Is(err, ErrRateLimited) {
		debug.Printf("Failed to post version update: %v", err)
	}
}

func (a *App) checkPasswordRecoveryEmailIsSet(ctx context.Context) {
	const interval = 14 * 24 * time.Hour
	err := DoIfElapsed(a.Options().MyCall, "pw_recovery_email_check", interval, func() error {
		debug.Printf("Checking if winlink.org password recovery email is set...")
		set, err := passwordRecoveryEmailSet(ctx, a.Options().MyCall, a.Config().SecureLoginPassword)
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
	if err != nil && !errors.Is(err, ErrRateLimited) {
		debug.Printf("Failed to check if password recovery email is set: %v", err)
	}
}

func passwordRecoveryEmailSet(ctx context.Context, callsign, password string) (bool, error) {
	if password == "" {
		return false, fmt.Errorf("missing password")
	}
	switch exists, err := accountExists(callsign); {
	case err != nil:
		return false, fmt.Errorf("error checking if account exist: %w", err)
	case !exists:
		return false, fmt.Errorf("account does not exist")
	}
	email, err := cmsapi.PasswordRecoveryEmailGet(ctx, callsign, password)
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
	exists, err := cmsapi.AccountExists(context.Background(), callsign)
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
