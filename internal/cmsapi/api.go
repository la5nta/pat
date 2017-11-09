// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cmsapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	RootURL        = "http://cms.winlink.org:8085"
	PathVersionAdd = "/version/add"
)

type VersionAdd struct {
	Callsign string `json:"callsign"`
	Program  string `json:"program"`
	Version  string `json:"version"`
	Comments string `json:"comments,omitempty"`
}

func (v VersionAdd) Post() error {
	b, _ := json.Marshal(v)
	buf := bytes.NewBuffer(b)

	req, _ := http.NewRequest("POST", RootURL+PathVersionAdd, buf)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	if errMsg, ok := response["ErrorMessage"]; ok {
		return fmt.Errorf("Winlink CMS Web Services: %s", errMsg)
	}

	return nil
}
