package cmsapi

import (
	"context"
	"net/url"
)

const PathHybridStationList = "/hybridStation/list"

type HybridStation struct {
	Callsign            string
	AutomaticForwarding bool
	ManualForwarding    bool
}

func HybridStationList(ctx context.Context) ([]HybridStation, error) {
	var resp struct {
		HybridList     []HybridStation
		ResponseStatus responseStatus
	}

	if err := getJSON(ctx, PathHybridStationList, url.Values{}, &resp); err != nil {
		return nil, err
	}

	return resp.HybridList, resp.ResponseStatus.errorOrNil()
}
