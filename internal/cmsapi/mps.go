package cmsapi

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

const (
	PathMPSAdd    = "/mps/add"
	PathMPSDelete = "/mps/delete"
	PathMPSGet    = "/mps/get"
	PathMPSList   = "/mps/list"
)

// MessagePickupStationRecord represents an MPS record
type MessagePickupStationRecord struct {
	Callsign    string     `json:"callsign"`
	MpsCallsign string     `json:"mpsCallsign"`
	Timestamp   DotNetTime `json:"timestamp"`
}

// DotNetTime handles .NET-style JSON date serialization
type DotNetTime struct{ time.Time }

// UnmarshalJSON implements custom JSON unmarshaling for .NET date format
func (t *DotNetTime) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}

	// Handle .NET date format: \/Date(milliseconds)\/
	re := regexp.MustCompile(`\/Date\((-?\d+)\)\/`)
	matches := re.FindStringSubmatch(str)
	if len(matches) == 2 {
		millis, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return err
		}
		t.Time = time.Unix(millis/1000, (millis%1000)*1000000)
		return nil
	}

	// Fall back to RFC3339 format
	parsedTime, err := time.Parse(time.RFC3339, str)
	if err == nil {
		t.Time = parsedTime
		return nil
	}

	// Fall back to RFC1123 format
	parsedTime, err = time.Parse(time.RFC1123, str)
	if err == nil {
		t.Time = parsedTime
		return nil
	}

	return err
}

// MPSAdd adds an entry to the MPS table
func MPSAdd(ctx context.Context, requester, callsign, password, mpsCallsign string) error {
	params := url.Values{
		"requester":   []string{requester},
		"callsign":    []string{callsign},
		"password":    []string{password},
		"mpsCallsign": []string{mpsCallsign},
	}
	var resp struct{ ResponseStatus responseStatus }
	if err := getJSON(ctx, PathMPSAdd, params, &resp); err != nil {
		return err
	}
	return resp.ResponseStatus.errorOrNil()
}

// MPSDelete deletes all MPS records for the specified callsign
func MPSDelete(ctx context.Context, requester, callsign, password string) error {
	params := url.Values{
		"requester": []string{requester},
		"callsign":  []string{callsign},
		"password":  []string{password},
	}
	var resp struct{ ResponseStatus responseStatus }
	if err := getJSON(ctx, PathMPSDelete, params, &resp); err != nil {
		return err
	}
	return resp.ResponseStatus.errorOrNil()
}

// MPSGet returns all MPS records for the specified callsign
func MPSGet(ctx context.Context, requester, callsign string) ([]MessagePickupStationRecord, error) {
	params := url.Values{
		"requester": []string{requester},
		"callsign":  []string{callsign},
	}
	var resp struct {
		MpsList        []MessagePickupStationRecord `json:"mpsList"`
		ResponseStatus responseStatus
	}
	if err := getJSON(ctx, PathMPSGet, params, &resp); err != nil {
		return nil, err
	}
	return resp.MpsList, resp.ResponseStatus.errorOrNil()
}
