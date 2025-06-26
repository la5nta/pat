package cmsapi

import (
	"context"
	"net/url"
	"os"
	"strconv"
)

const (
	PathAccountPasswordRecoveryEmailGet = "/account/password/recovery/email/get"
	PathAccountPasswordRecoveryEmailSet = "/account/password/recovery/email/set"
)

func PasswordRecoveryEmailGet(ctx context.Context, callsign, password string) (string, error) {
	if t, _ := strconv.ParseBool(os.Getenv("PAT_CMSAPI_MOCK_NO_RECOVERY_EMAIL")); t {
		return "", nil
	}
	params := url.Values{"callsign": []string{callsign}, "password": []string{password}}
	var resp struct {
		RecoveryEmail  string
		ResponseStatus responseStatus
	}
	if err := getJSON(ctx, PathAccountPasswordRecoveryEmailGet, params, &resp); err != nil {
		return "", err
	}
	return resp.RecoveryEmail, resp.ResponseStatus.errorOrNil()
}

func PasswordRecoveryEmailSet(ctx context.Context, callsign, password, email string) error {
	params := url.Values{"callsign": []string{callsign}, "password": []string{password}}
	body := bodyJSON(struct{ RecoveryEmail string }{email})
	req := newJSONRequest("POST", PathAccountPasswordRecoveryEmailSet, params, body).
		WithContext(ctx)
	var resp struct{ ResponseStatus responseStatus }
	if err := doJSON(req, &resp); err != nil {
		return err
	}
	return resp.ResponseStatus.errorOrNil()
}
