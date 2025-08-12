package voacap

import (
	"os"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	file, err := os.Open("testdata/output.out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	output, err := Parse(file)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}

	if !strings.Contains(output.Title, "IONOSPHERIC COMMUNICATIONS ANALYSIS AND PREDICTION PROGRAM") {
		t.Errorf("got %q, want %q", output.Title, "IONOSPHERIC COMMUNICATIONS ANALYSIS AND PREDICTION PROGRAM")
	}

	if output.Version != "16.1207W" {
		t.Errorf("got %q, want %q", output.Version, "16.1207W")
	}

	if output.Coeffs != "CCIR" {
		t.Errorf("got %q, want %q", output.Coeffs, "CCIR")
	}

	if output.Method != "30" {
		t.Errorf("got %q, want %q", output.Method, "30")
	}

	if output.SSN != 139.0 {
		t.Errorf("got %f, want %f", output.SSN, 139.0)
	}

	if output.MinAngle != 0.100 {
		t.Errorf("got %f, want %f", output.MinAngle, 0.100)
	}

	if output.Circuit.From.Lat != "60.31N" {
		t.Errorf("got %q, want %q", output.Circuit.From.Lat, "60.31N")
	}

	if output.Circuit.From.Lon != "5.37E" {
		t.Errorf("got %q, want %q", output.Circuit.From.Lon, "5.37E")
	}

	if output.Circuit.To.Lat != "59.77N" {
		t.Errorf("got %q, want %q", output.Circuit.To.Lat, "59.77N")
	}

	if output.Circuit.To.Lon != "10.78E" {
		t.Errorf("got %q, want %q", output.Circuit.To.Lon, "10.78E")
	}

	if output.Circuit.Azimuths[0] != 98.95 {
		t.Errorf("got %f, want %f", output.Circuit.Azimuths[0], 98.95)
	}

	if output.Circuit.Azimuths[1] != 283.64 {
		t.Errorf("got %f, want %f", output.Circuit.Azimuths[1], 283.64)
	}

	if output.Circuit.DistanceNM != 165.4 {
		t.Errorf("got %f, want %f", output.Circuit.DistanceNM, 165.4)
	}

	if output.Circuit.DistanceKM != 306.2 {
		t.Errorf("got %f, want %f", output.Circuit.DistanceKM, 306.2)
	}

	if output.Transmitter.Description != "samples/sample.23" {
		t.Errorf("got %q, want %q", output.Transmitter.Description, "samples/sample.23")
	}

	if output.Transmitter.Azimuth != 90.0 {
		t.Errorf("got %f, want %f", output.Transmitter.Azimuth, 90.0)
	}

	if output.Transmitter.OffAzimuth != 8.9 {
		t.Errorf("got %f, want %f", output.Transmitter.OffAzimuth, 8.9)
	}

	if output.Transmitter.PowerKW != 0.100 {
		t.Errorf("got %f, want %f", output.Transmitter.PowerKW, 0.100)
	}

	if output.Receiver.Description != "samples/sample.23" {
		t.Errorf("got %q, want %q", output.Receiver.Description, "samples/sample.23")
	}

	if output.Receiver.Azimuth != 270.0 {
		t.Errorf("got %f, want %f", output.Receiver.Azimuth, 270.0)
	}

	if output.Receiver.OffAzimuth != 13.6 {
		t.Errorf("got %f, want %f", output.Receiver.OffAzimuth, 13.6)
	}

	if output.Noise != -145.0 {
		t.Errorf("got %f, want %f", output.Noise, -145.0)
	}

	if output.RequiredRel != 90.0 {
		t.Errorf("got %f, want %f", output.RequiredRel, 90.0)
	}

	if output.RequiredSNR != -20.0 {
		t.Errorf("got %f, want %f", output.RequiredSNR, -20.0)
	}

	if output.PowerTol != 3.0 {
		t.Errorf("got %f, want %f", output.PowerTol, 3.0)
	}

	if output.DelayTol != 0.100 {
		t.Errorf("got %f, want %f", output.DelayTol, 0.100)
	}

	if output.Request.Hour != 20 {
		t.Errorf("got %d, want %d", output.Request.Hour, 20)
	}

	if output.Request.Frequency != 5.350 {
		t.Errorf("got %f, want %f", output.Request.Frequency, 5.350)
	}

	if len(output.Predictions) != 2 {
		t.Fatalf("got %d predictions, want %d", len(output.Predictions), 2)
	}

	prediction := output.Predictions[0]
	if prediction.Hour != 24.0 {
		t.Errorf("got %f, want %f", prediction.Hour, 24.0)
	}

	if len(prediction.BandPredictions) != 12 {
		t.Errorf("got %d freqs, want %d", len(prediction.BandPredictions), 12)
	}

	if prediction.BandPredictions[0].Mode != "1F2" {
		t.Errorf("got %s, want %s", prediction.BandPredictions[0].Mode, "1F2")
	}
}
