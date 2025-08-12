// Copyright 2023 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cmsapi

import (
	"encoding/json"
	"testing"
)

func TestGetGatewayStatusEmbedded(t *testing.T) {
	// Get the embedded gateway status
	reader, err := getGatewayStatusEmbedded()
	if err != nil {
		t.Fatalf("Failed to get embedded gateway status: %v", err)
	}
	defer reader.Close()

	// Unmarshal into GatewayStatus struct
	var status GatewayStatus
	if err := json.NewDecoder(reader).Decode(&status); err != nil {
		t.Fatalf("Failed to unmarshal gateway status data: %v", err)
	}

	// Check that Gateways slice is not empty
	if len(status.Gateways) == 0 {
		t.Error("Gateway status contains empty Gateways slice")
	}

	// Test at least one gateway has valid data
	if len(status.Gateways) > 0 {
		gateway := status.Gateways[0]
		if gateway.Callsign == "" {
			t.Error("First gateway has empty Callsign")
		}
		if gateway.Latitude == 0 && gateway.Longitude == 0 {
			t.Error("First gateway has invalid coordinates (0,0)")
		}
	}
}
