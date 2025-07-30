package propagation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/propagation/voacap"
)

// voacapPredictor implements the Predictor interface for the VOACAP.
type voacapPredictor struct {
	executable string
	dataDir    string

	mu sync.Mutex // Guard against concurrent voacapw.exe instances
}

// NewVOACAPPredictor creates a new VOACAP predictor
func NewVOACAPPredictor(executable, dataDir string) (Predictor, error) {
	// Find CLI executable
	if executable == "" {
		switch runtime.GOOS {
		case "windows":
			executable = `c:\itshfbc\bin_win\voacapw.exe`
		default:
			executable = `voacapl`
		}
	}
	executable, err := findExecutable(executable)
	if err != nil {
		return nil, fmt.Errorf("failed to find executable: %w", err)
	}

	// Find model data directory
	if dataDir == "" {
		switch runtime.GOOS {
		case "windows":
			// Usually c:\itshfbc (c:\itshfbc\bin_win\..)
			dataDir = filepath.Join(filepath.Dir(executable), "..")
		default:
			dataDir = os.ExpandEnv("$HOME/itshfbc")
		}
	}
	if _, err := os.Stat(dataDir); err != nil {
		return nil, fmt.Errorf("failed to find datadir: %w", err)
	}

	return &voacapPredictor{executable: executable, dataDir: dataDir}, nil
}

// Predict implements the Predictor interface.
func (p *voacapPredictor) Predict(ctx context.Context, params PredictionParams) (*Prediction, error) {
	const (
		inputName  = "input.dat"
		outputName = "output.out"
	)

	runDir := filepath.Join(p.dataDir, "run")
	args := []string{p.dataDir, inputName, outputName}
	if filepath.Base(p.executable) == "voacapl" {
		// voacapl supports the --run-dir option that allows for concurrent execution.
		dir, err := os.MkdirTemp("", "pat-voacap-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(dir)
		runDir = dir
		args = append([]string{"--run-dir=" + runDir}, args...)
	} else {
		// This is probably the original voacapw, which does not support the --run-dir option.
		// Grab the lock to prevent concurrent execution.
		p.mu.Lock()
		defer p.mu.Unlock()
	}

	// Generate input file and write it to the run dir.
	inputFile, err := p.generateInputFile(params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate input file: %w", err)
	}
	inputPath := filepath.Join(runDir, inputName)
	if err := os.WriteFile(inputPath, inputFile, 0644); err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}

	// Construct and execute the command.
	cmd := exec.CommandContext(ctx, p.executable, args...)
	cmd.Dir = runDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to execute prediction command: %w\nOutput:\n%s", err, string(output))
	}

	// Open and parse the output file.
	outputPath := filepath.Join(runDir, outputName)
	f, err := os.Open(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %w", err)
	}
	defer f.Close()

	prediction, err := p.parseOutput(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse output: %w", err)
	}
	return prediction, nil
}

func (*voacapPredictor) generateInputFile(params PredictionParams) ([]byte, error) {
	if params.Frequency >= 31e6 {
		return nil, ErrFrequencyOutOfBounds
	}
	var buf bytes.Buffer
	err := voacap.Encode(&buf, voacap.EncodeParams{
		DateTime:      params.Time,
		From:          string(params.From),
		To:            string(params.To),
		SSN:           params.SSN,
		TransmitPower: params.TransmitPower,
		MinSNR:        params.MinSNR,
		Frequency:     float64(params.Frequency) / 1e6,
	})
	if err != nil {
		return nil, err
	}
	if debug.Enabled() {
		fmt.Fprintf(os.Stderr, "--- voacap input ---\n%s\n", buf.Bytes())
	}
	return buf.Bytes(), nil
}

func (*voacapPredictor) parseOutput(output io.Reader) (*Prediction, error) {
	b, err := io.ReadAll(output)
	if err != nil {
		return nil, fmt.Errorf("failed to read voacap output: %w", err)
	}
	if debug.Enabled() {
		fmt.Fprintf(os.Stderr, "--- voacap output ---\n%s\n", b)
	}
	voacapOutput, err := voacap.Parse(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return toPrediction(voacapOutput)
}

func toPrediction(output *voacap.VoacapOutput) (*Prediction, error) {
	var prediction *voacap.Prediction
	for _, p := range output.Predictions {
		if p.Hour == float64(output.Request.Hour) {
			prediction = &p
			break
		}
	}
	if prediction == nil {
		return nil, fmt.Errorf("no prediction found for hour %d", output.Request.Hour)
	}

	var bestBand *voacap.BandPrediction
	var minFreqDiff float64
	for i, band := range prediction.BandPredictions {
		if math.IsNaN(band.SNR) {
			continue
		}
		freqDiff := math.Abs(band.Freq - output.Request.Frequency)
		if bestBand == nil || freqDiff < minFreqDiff {
			bestBand = &prediction.BandPredictions[i]
			minFreqDiff = freqDiff
		}
	}

	if bestBand == nil {
		return nil, fmt.Errorf("no usable prediction found in VOACAP output")
	}

	// ESQ (Expected Signal Quality) combines the probability of a link being
	// established (Circuit Reliability) with its quality (Signal-to-Noise
	// Ratio). The result is a single value indicating the overall expected
	// quality of the link. It addresses the shortcoming of using `REL` alone,
	// which does not account for how noisy a link might be.
	const (
		snrMin = -15.0
		snrMax = 70.0
	)
	snrFactor := (bestBand.SNR - snrMin) / (snrMax - snrMin)
	snrFactor = math.Max(0, math.Min(1, snrFactor))
	// Apply REL threshold and 10th power penalty for low reliability
	rel := bestBand.Rel
	if rel < 0.6 {
		rel = 0
	}
	rel = math.Pow(rel, 5)
	esq := rel * snrFactor

	return &Prediction{
		LinkQuality: int(math.Round(esq * 100)),
		REL:         bestBand.Rel,
		SNR:         bestBand.SNR,
		S_DBW:       bestBand.SDBW,
	}, nil
}
