package propagation

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVOACAPLAdapter_ParseOutput(t *testing.T) {
	predictor := &voacaplPredictor{}
	outputData, err := os.ReadFile(filepath.Join("voacap", "testdata", "output.out"))
	if err != nil {
		t.Fatalf("failed to read testdata/output.out: %v", err)
	}
	prediction, err := predictor.parseOutput(bytes.NewReader(outputData))
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	expected := &Prediction{
		LinkQuality: 99,
		SNR:         69,
		REL:         1.00,
		S_DBW:       -81,
	}
	if *prediction != *expected {
		t.Errorf("ParseOutput() mismatch (-want +got):\n%s", getDiff(fmt.Sprintf("%+v", expected), fmt.Sprintf("%+v", prediction)))
	}
}

func getDiff(want, got string) string {
	var diffs []string
	wantLines := strings.Split(strings.TrimSpace(want), "\n")
	gotLines := strings.Split(strings.TrimSpace(got), "\n")
	maxLines := len(wantLines)
	if len(gotLines) > maxLines {
		maxLines = len(gotLines)
	}

	for i := 0; i < maxLines; i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			diffs = append(diffs, fmt.Sprintf("line %d:\n- want: %q\n- got:  %q", i+1, w, g))
		}
	}
	return strings.Join(diffs, "\n")
}
