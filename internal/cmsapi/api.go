// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cmsapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	RootURL           = "https://api.winlink.org"
	PathVersionAdd    = "/version/add"
	PathGatewayStatus = "/gateway/status.json"
	PathAccountExists = "/account/exists"

	// AccessKey issued December 2017 by the WDT for use with Pat
	AccessKey = "1880278F11684B358F36845615BD039A"
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

	versionURL := RootURL + PathVersionAdd + "?key=" + AccessKey
	req, _ := http.NewRequest("POST", versionURL, buf)
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

func AccountExists(callsign string) (bool, error) {
	accountURL := RootURL + PathAccountExists + "?key=" + AccessKey + "&callsign=" + url.QueryEscape(callsign)
	req, _ := http.NewRequest("GET", accountURL, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var obj struct{ CallsignExists bool }
	return obj.CallsignExists, json.NewDecoder(resp.Body).Decode(&obj)
}

type GatewayStatus struct {
	ServerName string    `json:"ServerName"`
	ErrorCode  int       `json:"ErrorCode"`
	Gateways   []Gateway `json:"Gateways"`
}

type Gateway struct {
	Callsign      string
	BaseCallsign  string
	RequestedMode string
	Comments      string
	LastStatus    RFC1123Time
	Latitude      float64
	Longitude     float64

	Channels []GatewayChannel `json:"GatewayChannels"`
}

type GatewayChannel struct {
	OperatingHours string
	SupportedModes string
	Frequency      float64
	ServiceCode    string
	Baud           string
	RadioRange     string
	Mode           int
	Gridsquare     string
	Antenna        string
}

type RFC1123Time struct{ time.Time }

// GetGatewayStatus fetches the gateway status list returned by GatewayStatusUrl
//
// mode can be any of [packet, pactor, winmor, robustpacket, allhf or anyall]. Empty is AnyAll.
// historyHours is the number of hours of history to include (maximum: 48). If < 1, then API default is used.
// serviceCodes defaults to "PUBLIC".
func GetGatewayStatus(mode string, historyHours int, serviceCodes ...string) (io.ReadCloser, error) {
	switch {
	case mode == "":
		mode = "AnyAll"
	case historyHours > 48:
		historyHours = 48
	case len(serviceCodes) == 0:
		serviceCodes = []string{"PUBLIC"}
	}

	params := url.Values{"Mode": {mode}}
	params.Set("key", AccessKey)
	if historyHours >= 0 {
		params.Add("HistoryHours", fmt.Sprintf("%d", historyHours))
	}
	for _, str := range serviceCodes {
		params.Add("ServiceCodes", str)
	}

	resp, err := http.PostForm(RootURL+PathGatewayStatus, params)
	switch {
	case err != nil:
		return nil, err
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("unexpected http status '%v'", resp.Status)
	}

	return resp.Body, err
}

func GetGatewayStatusCached(cacheFile string, forceDownload bool, serviceCodes ...string) (io.ReadCloser, error) {
	if !forceDownload {
		file, err := os.Open(cacheFile)
		if err == nil {
			return file, nil
		}
	}

	log.Println("Downloading latest gateway status information...")
	fresh, err := GetGatewayStatus("", 48, serviceCodes...)
	if err != nil {
		return nil, err
	}

	file, err := os.Create(cacheFile)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(file, fresh)
	file.Seek(0, 0)
	if err == nil {
		log.Println("download succeeded.")
	}

	return file, err
}

func (t *RFC1123Time) UnmarshalJSON(b []byte) (err error) {
	var str string
	if err = json.Unmarshal(b, &str); err != nil {
		return err
	}
	t.Time, err = time.Parse(time.RFC1123, str)
	return err
}
