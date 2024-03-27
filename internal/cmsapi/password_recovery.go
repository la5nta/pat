package cmsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	PathAccountPasswordRecoveryEmailGet = "/account/password/recovery/email/get"
	PathAccountPasswordRecoveryEmailSet = "/account/password/recovery/email/set"
)

type responseStatus struct {
	ErrorCode string
	Message   string
}

func (r responseStatus) errorOrNil() error {
	if (r == responseStatus{}) {
		return nil
	}
	return &r
}

func (r *responseStatus) Error() string { return r.Message }

func PasswordRecoveryEmailGet(ctx context.Context, callsign, password string) (string, error) {
	req := newJSONRequest("GET", PathAccountPasswordRecoveryEmailGet,
		url.Values{"callsign": []string{callsign}, "password": []string{password}},
		nil).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Unexpected status code %d", resp.StatusCode)
	}
	var obj struct {
		RecoveryEmail  string
		ResponseStatus responseStatus
	}
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return "", err
	}
	return obj.RecoveryEmail, obj.ResponseStatus.errorOrNil()
}

func PasswordRecoveryEmailSet(ctx context.Context, callsign, password, email string) error {
	payload, err := json.Marshal(struct{ RecoveryEmail string }{email})
	if err != nil {
		panic(err)
	}
	req := newJSONRequest("POST", PathAccountPasswordRecoveryEmailSet,
		url.Values{"callsign": []string{callsign}, "password": []string{password}},
		bytes.NewReader(payload)).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Unexpected status code %d", resp.StatusCode)
	}
	var obj struct{ ResponseStatus responseStatus }
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return err
	}
	return obj.ResponseStatus.errorOrNil()
}

func newJSONRequest(method string, path string, queryParams url.Values, body io.Reader) *http.Request {
	url, err := url.JoinPath(RootURL, path)
	if err != nil {
		panic(err)
	}
	url += "?key=" + AccessKey
	if len(queryParams) > 0 {
		url += "&" + queryParams.Encode()
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}
