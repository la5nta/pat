// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cmsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/internal/buildinfo"
)

const (
	RootURL              = "https://api.winlink.org"
	PathVersionAdd       = "/version/add"
	PathGatewayStatus    = "/gateway/status.json"
	PathAccountExists    = "/account/exists"
	PathPasswordValidate = "/account/password/validate"
	PathAccountAdd       = "/account/add"

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
	req := newJSONRequest("POST", PathVersionAdd, nil, bodyJSON(v))
	var resp struct{ ResponseStatus responseStatus }
	if err := doJSON(req, &resp); err != nil {
		return err
	}
	return resp.ResponseStatus.errorOrNil()
}

func AccountExists(ctx context.Context, callsign string) (bool, error) {
	params := url.Values{"callsign": []string{callsign}}
	var resp struct {
		CallsignExists bool
		ResponseStatus responseStatus
	}
	if err := getJSON(ctx, PathAccountExists, params, &resp); err != nil {
		return false, err
	}
	return resp.CallsignExists, resp.ResponseStatus.errorOrNil()
}

type PasswordValidateRequest struct {
	Callsign string `json:"Callsign"`
	Password string `json:"Password"`
}

type PasswordValidateResponse struct {
	IsValid        bool           `json:"IsValid"`
	ResponseStatus responseStatus `json:"ResponseStatus"`
}

func ValidatePassword(ctx context.Context, callsign, password string) (bool, error) {
	req := PasswordValidateRequest{
		Callsign: callsign,
		Password: password,
	}
	httpReq := newJSONRequest("POST", PathPasswordValidate, nil, bodyJSON(req))
	httpReq = httpReq.WithContext(ctx)
	var resp PasswordValidateResponse
	if err := doJSON(httpReq, &resp); err != nil {
		return false, err
	}
	return resp.IsValid, resp.ResponseStatus.errorOrNil()
}

type AccountAddRequest struct {
	Callsign      string `json:"Callsign"`
	Password      string `json:"Password"`
	RecoveryEmail string `json:"RecoveryEmail,omitempty"`
}

type AccountAddResponse struct {
	ResponseStatus responseStatus `json:"ResponseStatus"`
}

func AccountAdd(ctx context.Context, callsign, password, recoveryEmail string) error {
	if t, _ := strconv.ParseBool(os.Getenv("PAT_CMSAPI_MOCK_ACCOUNT_ADD")); t {
		return nil
	}
	req := AccountAddRequest{
		Callsign:      callsign,
		Password:      password,
		RecoveryEmail: recoveryEmail,
	}

	httpReq := newJSONRequest("POST", PathAccountAdd, nil, bodyJSON(req))
	httpReq = httpReq.WithContext(ctx)
	var resp AccountAddResponse
	if err := doJSON(httpReq, &resp); err != nil {
		return err
	}
	return resp.ResponseStatus.errorOrNil()
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
// mode can be any of [packet, pactor, robustpacket, allhf or anyall]. Empty is AnyAll.
// historyHours is the number of hours of history to include (maximum: 48). If < 1, then API default is used.
// serviceCodes defaults to "PUBLIC".
func GetGatewayStatus(ctx context.Context, mode string, historyHours int, serviceCodes ...string) (io.ReadCloser, error) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", RootURL+PathGatewayStatus, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	resp, err := http.DefaultClient.Do(req)
	switch {
	case err != nil:
		return nil, err
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("unexpected http status '%v'", resp.Status)
	}

	return resp.Body, err
}

func GetGatewayStatusCached(ctx context.Context, cacheFile string, forceDownload bool, serviceCodes ...string) (io.ReadCloser, error) {
	if !forceDownload {
		file, err := os.Open(cacheFile)
		if err == nil {
			return file, nil
		}
	}

	log.Println("Downloading latest gateway status information...")
	fresh, err := GetGatewayStatus(ctx, "", 48, serviceCodes...)
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
