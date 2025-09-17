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

	"github.com/la5nta/pat/internal/propagation/voacap"
)

// Static VOACAP input parameters
const (
	MinTakeoffAngle    = 3.0
	MultipathDelay     = 0.1
	MultipathTolerance = 0.5
	LocalNoise         = 145.0
)

// Weights for LQI calculation components
// These weights determine the relative importance of each factor in the LQI calculation.
//
// Tuning guide:
// - Increase WeightReliability to prioritize connections with high reliability
// - Increase WeightSNR to favor connections with strong signal-to-noise ratio
// - Increase WeightTakeoffAngle to prefer optimal takeoff angles (prioritize for NVIS)
// - Increase WeightMUFday to prefer connections that are below MUF most of the month
// - Increase WeightMode to prioritize single-hop F2 layer propagation paths
// - Increase WeightSDBW to favor connections with stronger signal levels
//
// The weights can be any positive number (0.1, 1.0, 2.0, etc.)
// Higher values give that factor more influence on the final score.
// The final LQI value will still be normalized to a 0-100 scale.
const (
	WeightReliability  = 2.0 // Weight for reliability factor
	WeightSNR          = 1.0 // Weight for signal-to-noise ratio factor
	WeightTakeoffAngle = 0.3 // Weight for takeoff angle factor
	WeightMUFday       = 0.2 // Weight for MUF day percentage factor
	WeightMode         = 1.0 // Weight for propagation mode factor
	WeightSDBW         = 0.6 // Weight for signal strength factor
)

// voacapPredictor implements the Predictor interface for the VOACAP.
type voacapPredictor struct {
	executable string
	dataDir    string

	mu sync.Mutex // Guard against concurrent voacapw.exe instances
}

// NewVOACAPPredictor creates a new VOACAP predictor
func NewVOACAPPredictor(executable, dataDir string) (*voacapPredictor, error) {
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
			// Search default paths (prefer homedir)
			dataDir = os.ExpandEnv("$HOME/itshfbc")
			for _, d := range []string{
				dataDir,
				"/usr/share/voacapl/itshfbc",
				"/usr/local/share/voacapl/itshfbc",
				filepath.Join(brewPrefix(), "/share/voacapl/itshfbc"),
			} {
				if _, err := os.Stat(d); err == nil {
					dataDir = d
					break
				}
			}
		}
	}
	if _, err := os.Stat(dataDir); err != nil {
		return nil, fmt.Errorf("failed to find datadir: %w", err)
	}

	return &voacapPredictor{executable: executable, dataDir: dataDir}, nil
}

func (p *voacapPredictor) Version() string {
	// Only VOACAP for Linux has this feature
	if !isVoacapl(p.executable) {
		return filepath.Base(p.executable) + " (unknown version)"
	}
	cmd := exec.Command(p.executable, "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(bytes.TrimSpace(output))
}

// Predict implements the Predictor interface.
func (p *voacapPredictor) Predict(ctx context.Context, params PredictionParams) (*Prediction, error) {
	const (
		inputName  = "input.dat"
		outputName = "output.out"
	)

	runDir := filepath.Join(p.dataDir, "run")
	args := []string{p.dataDir, inputName, outputName}
	if isVoacapl(p.executable) {
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
		// voacapw also needs the SILENT flag to prevent it from opening a window.
		args = append([]string{"SILENT"}, args...)
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
		DateTime:           params.Time,
		From:               string(params.From),
		To:                 string(params.To),
		SSN:                params.SSN,
		TransmitPower:      params.TransmitPower,
		MinSNR:             20, // Affects the REL output
		Frequency:          float64(params.Frequency) / 1e6,
		MinTakeoffAngle:    MinTakeoffAngle,
		MultipathDelay:     MultipathDelay,
		MultipathTolerance: MultipathTolerance,
		LocalNoise:         LocalNoise,
	})
	if err != nil {
		return nil, err
	}
	debugf("--- voacap input ---\n%s", buf.Bytes())
	return buf.Bytes(), nil
}

func (*voacapPredictor) parseOutput(output io.Reader) (*Prediction, error) {
	b, err := io.ReadAll(output)
	if err != nil {
		return nil, fmt.Errorf("failed to read voacap output: %w", err)
	}
	debugf("--- voacap output ---\n%s", b)
	voacapOutput, err := voacap.Parse(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return toPrediction(voacapOutput, string(b))
}

func toPrediction(output *voacap.VoacapOutput, outputRaw string) (*Prediction, error) {
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

	// Identify the relevant band prediction
	var band *voacap.BandPrediction
	var minFreqDiff float64
	for i, b := range prediction.BandPredictions {
		if math.IsNaN(b.SNR) {
			continue
		}
		freqDiff := math.Abs(b.Freq - output.Request.Frequency)
		if band == nil || freqDiff < minFreqDiff {
			band = &prediction.BandPredictions[i]
			minFreqDiff = freqDiff
		}
	}
	if band == nil {
		return nil, fmt.Errorf("no usable prediction found in VOACAP output")
	}

	// MODE factor is now implemented, evaluating propagation paths based on number of hops and ionospheric layers

	// LQI (Link Quality Index) is a score from 0 to 100 that assesses the expected quality
	// of a point-to-point HF circuit. It combines factors for reliability, signal-to-noise ratio,
	// and takeoff angle.

	// Reliability Factor (F_REL)
	// This factor uses a sigmoid-like function to create a smooth transition around 90%,
	// while still heavily penalizing values below that threshold.
	// The formula creates a gentle slope rather than a hard cutoff.
	//
	// Example outputs (with updated formula):
	// REL = 1.00 → F_REL = 1.00    (perfect reliability gets perfect factor)
	// REL = 0.99 → F_REL ≈ 0.97    (very high reliability, minimal penalty)
	// REL = 0.97 → F_REL ≈ 0.96    (high reliability, very small penalty)
	// REL = 0.95 → F_REL = 0.95    (high reliability baseline)
	// REL = 0.90 → F_REL ≈ 0.87    (good reliability)
	// REL = 0.85 → F_REL ≈ 0.73    (threshold value)
	// REL = 0.84 → F_REL ≈ 0.49    (below threshold, significant penalty)
	// REL = 0.80 → F_REL ≈ 0.46    (below threshold, major penalty)
	// REL = 0.70 → F_REL ≈ 0.39    (poor reliability)
	const (
		RelSigmoidSlope       = 3.0  // Controls steepness of sigmoid transition (lower = gentler)
		RelThreshold          = 0.85 // Critical reliability threshold (85%)
		RelHighFactorMinValue = 0.73 // Minimum factor for REL ≥ 80%
		RelLowScaleFactor     = 1.0  // Multiplier for REL < 85% (higher = less penalty)
	)
	relFactor := 0.0
	if band.Rel > 0.0 {
		// Special handling for very high reliability values
		if band.Rel >= 0.999 {
			// Perfect reliability (1.0) gets perfect score
			relFactor = 1.0
		} else if band.Rel >= 0.95 {
			// For very high reliability (≥ 95%), use a linear scale from 0.95 to 0.999
			// that maps to factor values from 0.95 to 0.98
			// This creates a more gradual transition for high reliability values
			normalizedPos := (band.Rel - 0.95) / (0.999 - 0.95)
			relFactor = 0.95 + (0.98-0.95)*normalizedPos
		} else if band.Rel >= RelThreshold {
			// For values between threshold and high reliability, use standard scaling
			// Use sigmoid-like function centered around RelThreshold (85%)
			sigmoidValue := 1.0 / (1.0 + math.Exp(-RelSigmoidSlope*(band.Rel-RelThreshold)))
			// Scale to range from RelHighFactorMinValue to 0.95
			relFactor = RelHighFactorMinValue + (0.95-RelHighFactorMinValue)*sigmoidValue
		} else {
			// For values below threshold, use sigmoid with steep drop-off
			sigmoidValue := 1.0 / (1.0 + math.Exp(-RelSigmoidSlope*(band.Rel-RelThreshold)))
			relFactor = sigmoidValue * RelLowScaleFactor
		}
	}

	// SNR Factor (F_SNR)
	// This factor evaluates the Signal-to-Noise Ratio (SNR) based on practical thresholds for digital modes on HF.
	// For HF digital modes:
	// - -18 dB: Theoretical minimum for PACTOR-2
	// - 3-10 dB: Minimum for basic operation
	// - 10-20 dB: Good operation for robust modes
	// - 20-30 dB: Very good for most digital modes
	// - 30-40 dB: Excellent for high-speed data modes
	// - >40 dB: Exceptional (rarely achieved in real HF conditions)
	//
	// NOTE: VOACAP SNR values are theoretical and often optimistic compared to real-world conditions.
	// Users typically experience 10-20 dB worse SNR due to local noise sources not accounted for
	// in the model (urban noise, interference, seasonal factors, etc.). Using a higher maximum
	// threshold helps account for this reality and better differentiates between frequencies that
	// might perform differently under real noise conditions.
	const (
		SNRMinimumThreshold = 3.0  // Assumed minimum SNR (dB) for a sustainable link, accounting for real-world noise
		SNRMaximumThreshold = 60.0 // SNR (dB) above which the signal is considered perfect, accounting for real-world noise
	)
	snrFactor := 0.0
	if band.SNR >= SNRMinimumThreshold {
		if band.SNR >= SNRMaximumThreshold {
			snrFactor = 1.0
		} else {
			// Calculate SNR factor as a proportion of the range
			snrFactor = (band.SNR - SNRMinimumThreshold) / (SNRMaximumThreshold - SNRMinimumThreshold)
		}
	}

	// Simplified Angle Factor (F_ANGLE)
	// This factor uses a simpler approach that primarily penalizes very low angles
	// which typically perform poorly due to local terrain and buildings.
	// Since the MODE factor now accounts for optimal propagation paths,
	// this factor focuses only on practical takeoff angle limitations.
	const (
		LowAngleThreshold = 30.0            // Threshold below which we begin applying penalties
		MinTakeoffAngle   = MinTakeoffAngle // Same value as given to VOACAP
		MaxAnglePenalty   = 0.5             // Maximum penalty (50%) for very low takeoff angles
	)
	angleFactor := 0.0
	if band.Tangle >= MinTakeoffAngle && band.Tangle <= 90.0 {
		if band.Tangle >= LowAngleThreshold {
			// No penalty for angles above the threshold
			angleFactor = 1.0
		} else {
			// Linear penalty for low angles
			normalizedPos := (band.Tangle - MinTakeoffAngle) / (LowAngleThreshold - MinTakeoffAngle)
			angleFactor = MaxAnglePenalty + normalizedPos*(1.0-MaxAnglePenalty)
		}
	}

	// MUFday Factor (F_MUF)
	// This factor considers how close we are to the Maximum Usable Frequency
	// Higher MUFday means better chance of maintaining the circuit
	// VOACAP provides MUFday values as decimals (0.0-1.0, not percentages)
	const (
		MUFdayFloor = 0.3 // Minimum floor for MUF day percentage factor
	)
	mufFactor := 0.0
	if band.MUFday >= 0.0 && band.MUFday <= 1.0 {
		// Use the value directly, it's already in 0-1 range
		mufFactor = band.MUFday

		// Apply floor to ensure bands with good current conditions
		// but poor MUFday aren't completely penalized
		if mufFactor < MUFdayFloor {
			mufFactor = MUFdayFloor
		}
	}

	// MODE Factor (F_MODE)
	// This factor evaluates the propagation path based on number of hops and layer type
	// Ranking from best to worst:
	// 1. "1 F2" (Single-hop F2) - optimal for digital modes
	// 2. "1 F1" (Single-hop F1) - good but less stable than F2
	// 3. "1 E" (Single-hop E) - good for shorter distances but less stable
	// 4. "2 F2" (Two-hop F2) - acceptable but with increased multipath
	// 5. "2 F1", "2 E" (Two-hop F1/E) - more challenging
	// 6. "3+ hops" (Three or more hops) - significant multipath issues
	// 7. Long path modes - evaluated based on layers involved
	modeFactor := 0.0
	if band.Mode != "" {
		debugf("Parsing MODE: %q", band.Mode)

		// Parse the mode string - handle both formats from MODE.md
		if len(band.Mode) >= 2 && band.Mode[0] >= '0' && band.Mode[0] <= '9' {
			// Short path format: "NL" (e.g., "1F2", "2F2", "3E", etc.)
			// This handles both space and no-space formats ("1 F2" and "1F2")
			hops := int(band.Mode[0] - '0')

			// Extract layer part, handling both formats
			var layer string
			if len(band.Mode) >= 3 && band.Mode[1] == ' ' {
				// Format with space: "1 F2"
				layer = band.Mode[2:]
			} else {
				// Format without space: "1F2"
				layer = band.Mode[1:]
			}

			debugf("Parsed short path mode: %d hops via %s layer", hops, layer)

			if hops == 1 {
				switch layer {
				case "F2":
					modeFactor = 1.0 // Perfect - single hop F2
				case "F1":
					modeFactor = 0.9 // Very good - single hop F1
				case "E":
					modeFactor = 0.8 // Good - single hop E
				default:
					modeFactor = 0.7 // Unknown layer
				}
			} else if hops == 2 {
				switch layer {
				case "F2":
					modeFactor = 0.7 // Acceptable - two hop F2
				case "F1":
					modeFactor = 0.6 // Marginal - two hop F1
				case "E":
					modeFactor = 0.5 // Poor - two hop E
				default:
					modeFactor = 0.4 // Unknown layer
				}
			} else {
				modeFactor = 0.3 // 3+ hops - challenging
			}
		} else {
			// Long path format: "L1L2" (e.g., "F2F2", "EF1", etc.)
			debugf("Parsed long path mode: %s", band.Mode)

			if len(band.Mode) >= 4 {
				if band.Mode[:2] == "F2" && band.Mode[2:4] == "F2" {
					modeFactor = 0.5 // F2F2 is best long path option
				} else if band.Mode[:2] == "F2" || band.Mode[2:4] == "F2" {
					modeFactor = 0.4 // At least one F2 layer
				} else {
					modeFactor = 0.3 // No F2 layers
				}
			} else {
				modeFactor = 0.3 // Unrecognized format
			}
		}
	} else {
		modeFactor = 0.5 // Default if MODE is empty
	}

	// S_DBW Factor (F_SDBW)
	// This factor evaluates the signal strength based on S-meter equivalents according to
	// IARU Region 1 Technical Recommendation R.1:
	// S9 = -73 dBm = -103 dBW
	// Each S-unit is exactly 6 dB
	// S1 = -121 dBm = -151 dBW
	// S9+40dB = -33 dBm = -63 dBW (maximum value)
	// Below S1 signals extend to -163 dBW for weak digital modes
	const (
		SDBWMinThreshold = -163.0 // Extended below S0 for digital modes (S0-6dB in IARU standard)
		SDBWMaxThreshold = -63.0  // Equivalent to S9+40dB per IARU standard (-103dBW + 40dB)
		SDBWRange        = SDBWMaxThreshold - SDBWMinThreshold
		SDBWFloor        = 0.05 // Minimum non-zero factor for very weak but potentially usable signals
	)
	sdbwFactor := 0.0

	// If S_DBW is absurdly low (below extended range), keep it at 0
	if band.SDBW > -200.0 && band.SDBW <= SDBWMinThreshold {
		// For extremely weak signals that could still be useful for some digital modes,
		// assign a small but non-zero factor
		sdbwFactor = SDBWFloor
	} else if band.SDBW > SDBWMinThreshold {
		if band.SDBW >= SDBWMaxThreshold {
			sdbwFactor = 1.0
		} else {
			// Calculate S_DBW factor using a logarithmic scale that better matches
			// the logarithmic nature of dB measurements
			//
			// First, normalize the S_DBW value to [0,1] range
			normValue := (band.SDBW - SDBWMinThreshold) / SDBWRange
			// Then apply a sqrt function to create a more favorable curve
			// This gives higher weights to mid-range S-meter values
			// Ensure a minimum floor value for very weak but potentially usable signals
			sdbwFactor = math.Max(math.Sqrt(normValue), SDBWFloor)
		}
	}

	// Apply weights to each factor using exponentiation
	// This allows us to adjust the relative importance of each factor
	// while maintaining the multiplicative nature of the formula
	weightedRelFactor := math.Pow(relFactor, WeightReliability)
	weightedSnrFactor := math.Pow(snrFactor, WeightSNR)
	weightedAngleFactor := math.Pow(angleFactor, WeightTakeoffAngle)
	weightedMufFactor := math.Pow(mufFactor, WeightMUFday)
	weightedModeFactor := math.Pow(modeFactor, WeightMode)
	weightedSdbwFactor := math.Pow(sdbwFactor, WeightSDBW)

	// Calculate final LQI using multiplicative formula including all factors
	lqi := weightedRelFactor * weightedSnrFactor * weightedAngleFactor * weightedMufFactor * weightedModeFactor * weightedSdbwFactor * 100

	// Debug log the LQI calculation steps
	debugf("--- LQI calculation ---")
	debugf("%-6s  %-15s  %-6s  %-6s  %-6s", "PARAM", "VALUE", "RAW", "WEIGHT", "WEIGHTED")
	debugf("%-6s  %-15.2f  %-6s  %-6.1f  %.4f", "REL", band.Rel, fmt.Sprintf("%.4f", relFactor), WeightReliability, weightedRelFactor)
	debugf("%-6s  %-15s  %-6s  %-6.1f  %.4f", "SNR", fmt.Sprintf("%.1f dB", band.SNR), fmt.Sprintf("%.4f", snrFactor), WeightSNR, weightedSnrFactor)
	debugf("%-6s  %-15s  %-6s  %-6.1f  %.4f", "TANGLE", fmt.Sprintf("%.1f°", band.Tangle), fmt.Sprintf("%.4f", angleFactor), WeightTakeoffAngle, weightedAngleFactor)
	debugf("%-6s  %-15.2f  %-6s  %-6.1f  %.4f", "MUFday", band.MUFday, fmt.Sprintf("%.4f", mufFactor), WeightMUFday, weightedMufFactor)
	debugf("%-6s  %-15s  %-6s  %-6.1f  %.4f", "MODE", band.Mode, fmt.Sprintf("%.4f", modeFactor), WeightMode, weightedModeFactor)
	debugf("%-6s  %-15s  %-6s  %-6.1f  %.4f", "S_DBW", fmt.Sprintf("%.1f (%s)", band.SDBW, toSMeter(band.SDBW)), fmt.Sprintf("%.4f", sdbwFactor), WeightSDBW, weightedSdbwFactor)
	debugf("-------------------------------------------------------------")
	debugf("                               REL      SNR      TANGLE   MUFday   MODE     S_DBW")
	debugf("LQI = Weighted factors × 100 = %.4f × %.4f × %.4f × %.4f × %.4f × %.4f × 100 = %.1f",
		weightedRelFactor, weightedSnrFactor, weightedAngleFactor, weightedMufFactor, weightedModeFactor, weightedSdbwFactor, lqi)

	return &Prediction{
		LinkQuality:     int(math.Round(lqi)),
		PathReliability: band.Rel,
		SNR:             band.SNR,
		SignalPower:     band.SDBW,
		OutputRaw:       outputRaw,
		OutputValues:    band,
	}, nil
}

func toSMeter(SDBW float64) string {
	// Using IARU Region 1 Technical Recommendation R.1
	// S9 = -73 dBm = -103 dBW (dBW = dBm - 30)
	// Each S-unit is 6 dB
	// S9+40dB = -103 dBW + 40 dB = -63 dBW
	switch {
	case SDBW >= -103: // S9 or above
		// Calculate dB over S9
		overS9 := int(SDBW - (-103))
		if overS9 > 0 {
			return fmt.Sprintf("S9+%ddB", overS9)
		}
		return "S9"
	case SDBW >= -109: // S8 = S9 - 6dB
		return "S8"
	case SDBW >= -115: // S7 = S9 - 12dB
		return "S7"
	case SDBW >= -121: // S6 = S9 - 18dB
		return "S6"
	case SDBW >= -127: // S5 = S9 - 24dB
		return "S5"
	case SDBW >= -133: // S4 = S9 - 30dB
		return "S4"
	case SDBW >= -139: // S3 = S9 - 36dB
		return "S3"
	case SDBW >= -145: // S2 = S9 - 42dB
		return "S2"
	case SDBW >= -151: // S1 = S9 - 48dB
		return "S1"
	default:
		return "<S1"
	}
}

func isVoacapl(executable string) bool {
	return filepath.Base(executable) == "voacapl"
}

// brewPrefix returns the brew prefix path or empty string on error
func brewPrefix() string {
	cmd := exec.Command("brew", "--prefix")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(output))
}
