package voacap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncode(t *testing.T) {
	params := EncodeParams{
		From:               "JP20QH", // LA5NTA
		To:                 "JO59JS", // LA3F
		Frequency:          5.350,
		SSN:                139,
		TransmitPower:      100,
		DateTime:           time.Date(2025, time.July, 1, 20, 0, 0, 0, time.UTC),
		MinSNR:             -20,
		MinTakeoffAngle:    15.0,
		MultipathDelay:     0.1,
		MultipathTolerance: 0.5,
		LocalNoise:         145.0,
	}

	var gotBuf bytes.Buffer
	if err := Encode(&gotBuf, params); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got := gotBuf.String()

	expectedBytes, err := os.ReadFile(filepath.Join("testdata", "input.dat"))
	if err != nil {
		t.Fatalf("Failed to read expected input: %v", err)
	}
	expected := string(expectedBytes)

	if strings.TrimSpace(expected) != strings.TrimSpace(got) {
		t.Errorf("Encode() mismatch, want:\n%s\ngot:\n%s", expected, got)
	}
}
