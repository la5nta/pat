package cmsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/la5nta/pat/internal/buildinfo"
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

func getJSON(ctx context.Context, path string, queryParams url.Values, v interface{}) error {
	req := newJSONRequest("GET", path, queryParams, nil).WithContext(ctx)
	return doJSON(req, v)
}

func doJSON(req *http.Request, v interface{}) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code: %d (%s)", resp.StatusCode, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func bodyJSON(v interface{}) io.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(b)
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
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}
