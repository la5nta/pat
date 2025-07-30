package noaa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGetPredictedSSN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/solar-cycle/predicted-solar-cycle.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join("testdata", "predicted-solar-cycle.json"))
		if err != nil {
			t.Fatalf("failed to read testdata: %v", err)
		}
		w.Write(b)
	}))
	defer server.Close()

	SolarCyclePredictionURL = server.URL + "/json/solar-cycle/predicted-solar-cycle.json"

	predictions, err := GetPredictedSSN(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(predictions) != 72 {
		t.Fatalf("expected 72 predictions, got %d", len(predictions))
	}

	if predictions[0].TimeTag != "2025-01" {
		t.Errorf("unexpected time-tag: %s", predictions[0].TimeTag)
	}
	if predictions[0].PredictedSSN != 148.9 {
		t.Errorf("unexpected predicted_ssn: %f", predictions[0].PredictedSSN)
	}
}
