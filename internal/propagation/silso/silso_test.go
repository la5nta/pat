package silso

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbedded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failure", http.StatusBadGateway)
	}))
	defer server.Close()

	SourceURL = server.URL

	predictions, err := GetPredictedSSN(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(predictions) == 0 {
		t.Fatalf("expected predictions, got none")
	}
}

func TestGetPredictedSSN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/SILSO/FORECASTS/KFprediCM.txt" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join("testdata", "KFprediCM.txt"))
		if err != nil {
			t.Fatalf("failed to read testdata: %v", err)
		}
		w.Write(b)
	}))
	defer server.Close()

	SourceURL = server.URL + "/SILSO/FORECASTS/KFprediCM.txt"

	predictions, err := GetPredictedSSN(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(predictions) != 18 {
		t.Fatalf("expected 72 predictions, got %d", len(predictions))
	}
	if predictions[0].TimeTag != "2025-01" {
		t.Errorf("unexpected time-tag: %s", predictions[0].TimeTag)
	}
	if predictions[0].PredictedSSN != 153.1 {
		t.Errorf("unexpected predicted_ssn: %f", predictions[0].PredictedSSN)
	}
}
func TestParse(t *testing.T) {
	data := `
# comment
2025 01  2025.042 :  149.5     0
2025 02  2025.122 :  148.6     0
`
	r := strings.NewReader(data)
	predictions, err := parse(r)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(predictions) != 2 {
		t.Fatalf("got %d predictions, want 2", len(predictions))
	}
	expected := []Prediction{
		{TimeTag: "2025-01", PredictedSSN: 149.5},
		{TimeTag: "2025-02", PredictedSSN: 148.6},
	}
	for i, p := range predictions {
		if p.TimeTag != expected[i].TimeTag || p.PredictedSSN != expected[i].PredictedSSN {
			t.Errorf("prediction %d: got %+v, want %+v", i, p, expected[i])
		}
	}
}
