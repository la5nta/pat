// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/cmsapi"
	"github.com/la5nta/pat/internal/directories"
)

func accountExistsCached(callsign string) (bool, error) {
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

	exists, err := cmsapi.AccountExists(callsign)
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

func postVersionUpdate() error {
	var lastUpdated time.Time
	filePath := filepath.Join(directories.StateDir(), "last_version_report.json")
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	json.NewDecoder(file).Decode(&lastUpdated)
	if time.Since(lastUpdated) < 24*time.Hour {
		return nil
	}

	// WDT do not want us to post version reports for callsigns without a registered account
	if exists, err := accountExistsCached(fOptions.MyCall); err != nil {
		return err
	} else if !exists {
		return nil
	}

	v := cmsapi.VersionAdd{
		Callsign: fOptions.MyCall,
		Program:  buildinfo.AppName,
		Version:  buildinfo.Version,
		Comments: fmt.Sprintf("%s - %s/%s", buildinfo.GitRev, runtime.GOOS, runtime.GOARCH),
	}

	if err := v.Post(); err != nil {
		return err
	}

	file.Truncate(0)
	file.Seek(0, 0)
	return json.NewEncoder(file).Encode(time.Now())
}
