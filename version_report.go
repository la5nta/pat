// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/la5nta/pat/internal/cmsapi"
)

func postVersionUpdate() error {
	var lastUpdated time.Time
	file, err := os.OpenFile(path.Join(appDir, "last_version_report.json"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	json.NewDecoder(file).Decode(&lastUpdated)

	if time.Since(lastUpdated) < 24*time.Hour {
		return nil
	}

	v := cmsapi.VersionAdd{
		Callsign: fOptions.MyCall,
		Program:  AppName,
		Version:  Version,
		Comments: fmt.Sprintf("%s - %s/%s", GitRev, runtime.GOOS, runtime.GOARCH),
	}

	if err := v.Post(); err != nil {
		return err
	}

	file.Truncate(0)
	file.Seek(0, 0)
	return json.NewEncoder(file).Encode(time.Now())
}
