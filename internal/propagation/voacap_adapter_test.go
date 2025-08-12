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
	testCases := []struct {
		fileName string
		expected *Prediction
	}{
		{
			// Local connection with excellent quality (99%)
			fileName: "LA5NTA_to_LA3F_60m.out",
			expected: &Prediction{
				LinkQuality:     94,
				SNR:             69,
				PathReliability: 1.00,
				SignalPower:     -81,
			},
		},
		{
			// Local station on 80m band with good quality (65%)
			fileName: "LA5NTA_to_LA3F_80m.out",
			expected: &Prediction{
				LinkQuality:     59,
				SNR:             53,
				PathReliability: 0.99,
				SignalPower:     -94,
			},
		},
		{
			// Arctic station (Svalbard) with medium quality (50%)
			fileName: "LA5NTA_to_JW5E_20m.out",
			expected: &Prediction{
				LinkQuality:     41,
				SNR:             51,
				PathReliability: 0.93,
				SignalPower:     -112,
			},
		},
		{
			// Same Arctic station on 80m band with no connectivity (0%)
			fileName: "LA5NTA_to_JW5E_80m.out",
			expected: &Prediction{
				LinkQuality:     0,
				SNR:             -53,
				PathReliability: 0.00,
				SignalPower:     -200,
			},
		},
		{
			// Medium distance with good quality (73%)
			fileName: "LA5NTA_to_HB9AK_30m.out",
			expected: &Prediction{
				LinkQuality:     61,
				SNR:             50,
				PathReliability: 0.98,
				SignalPower:     -109,
			},
		},
		{
			// Same station on a different band with no connectivity (0%)
			fileName: "LA5NTA_to_HB9AK_80m.out",
			expected: &Prediction{
				LinkQuality:     0,
				SNR:             -25,
				PathReliability: 0.00,
				SignalPower:     -172,
			},
		},
		{
			// Very long distance to Hawaii with poor quality (3%)
			fileName: "LA5NTA_to_AH7L_20m.out",
			expected: &Prediction{
				LinkQuality:     2,
				SNR:             29,
				PathReliability: 0.66,
				SignalPower:     -134,
			},
		},
		{
			// Medium distance to Germany with good quality (78%)
			fileName: "LA5NTA_to_DM6TS_30m.out",
			expected: &Prediction{
				LinkQuality:     66,
				SNR:             53,
				PathReliability: 0.99,
				SignalPower:     -106,
			},
		},
	}

	predictor := &voacapPredictor{}

	for _, tc := range testCases {
		// Use filename without extension as test name
		testName := strings.TrimSuffix(tc.fileName, filepath.Ext(tc.fileName))
		t.Run(testName, func(t *testing.T) {
			outputData, err := os.ReadFile(filepath.Join("voacap", "testdata", tc.fileName))
			if err != nil {
				t.Fatalf("failed to read voacap/testdata/%s: %v", tc.fileName, err)
			}

			prediction, err := predictor.parseOutput(bytes.NewReader(outputData))
			if err != nil {
				t.Fatalf("ParseOutput() error = %v", err)
			}
			prediction.OutputRaw, prediction.OutputValues = "", nil

			if *prediction != *tc.expected {
				t.Errorf("ParseOutput() mismatch (-want +got):\n%s", getDiff(fmt.Sprintf("%+v", tc.expected), fmt.Sprintf("%+v", prediction)))
			}
		})
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

func TestToSMeter(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		// Using IARU Region 1 Technical Recommendation R.1
		// S9 = -73 dBm = -103 dBW (dBW = dBm - 30)
		// Each S-unit is 6 dB

		// Testing S9+40dB according to IARU standard
		{"S9+40dB", -63.0, "S9+40dB"},

		// Test S9+n cases
		{"S9+20dB", -83.0, "S9+20dB"},
		{"S9+10dB", -93.0, "S9+10dB"},
		{"S9", -103.0, "S9"},

		// Test S1-S8 cases
		{"S8", -109.0, "S8"},
		{"S7", -115.0, "S7"},
		{"S6", -121.0, "S6"},
		{"S5", -127.0, "S5"},
		{"S4", -133.0, "S4"},
		{"S3", -139.0, "S3"},
		{"S2", -145.0, "S2"},
		{"S1", -150.0, "S1"},

		// Test below S1
		{"Below S1", -152.0, "<S1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toSMeter(tt.input); got != tt.want {
				t.Errorf("toSMeter(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
