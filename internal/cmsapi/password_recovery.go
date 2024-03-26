package cmsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	PathAccountPasswordRecoveryEmailGet = "/account/password/recovery/email/get"
	PathAccountPasswordRecoveryEmailSet = "/account/password/recovery/email/set"
)

func PasswordRecoveryEmailGet(ctx context.Context, callsign, password string) (string, error) {
	url := RootURL + PathAccountPasswordRecoveryEmailGet +
		"?key=" + AccessKey +
		"&callsign=" + url.QueryEscape(callsign) +
		"&password=" + url.QueryEscape(password)
	req, _ := http.NewRequest("GET", url, nil)
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Unexpected status code %d", resp.StatusCode)
	}
	var obj struct{ RecoveryEmail string }
	return obj.RecoveryEmail, json.NewDecoder(resp.Body).Decode(&obj)
}

func PasswordRecoveryEmailSet(ctx context.Context, callsign, password, email string) error {
	payload, err := json.Marshal(struct{ RecoveryEmail string }{email})
	if err != nil {
		panic(err)
	}
	url := RootURL + PathAccountPasswordRecoveryEmailSet +
		"?key=" + AccessKey +
		"&callsign=" + url.QueryEscape(callsign) +
		"&password=" + url.QueryEscape(password)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Unexpected status code %d", resp.StatusCode)
	}
	return nil
}
