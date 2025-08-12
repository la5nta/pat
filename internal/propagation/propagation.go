package propagation

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
)

// ErrFrequencyOutOfBounds is returned if the frequency is out of bounds for the prediction model.
var ErrFrequencyOutOfBounds = errors.New("frequency out of bounds")

// Maidenhead represents a Maidenhead locator string.
type Maidenhead string

// PredictionParams contains all the necessary inputs for a prediction.
type PredictionParams struct {
	Time          time.Time  // Time period to predict
	SSN           int        // Smoothed Sunspot Number for the specified time
	From          Maidenhead // Location of the local station
	To            Maidenhead // Location of the remote station
	Frequency     int        // Frequency in Hz
	TransmitPower int        // Transmit power in watt
}

// Prediction holds the output from a prediction model.
type Prediction struct {
	LinkQuality int // Estimated link quality (0-100)

	PathReliability float64 // Path reliability
	SNR             float64 // Signal-to-Noise Ratio
	SignalPower     float64 // Signal Power in dBW

	OutputRaw    string // Raw output from the prediction engine
	OutputValues any    // Parsed output from the prediction engine
}

// Predictor defines the interface for a propagation prediction engine.
type Predictor interface {
	Predict(ctx context.Context, params PredictionParams) (*Prediction, error)
}

// PredictParallel runs predictions in parallel and calls the provided callback for each result.
func PredictParallel(ctx context.Context, p Predictor, params []PredictionParams, callback func(int, *Prediction, error)) {
	type job struct {
		idx    int
		params PredictionParams
	}

	jobs := make(chan job)
	go func() {
		defer close(jobs)
		for i, p := range params {
			jobs <- job{idx: i, params: p}
		}
	}()

	numWorkers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				prediction, err := p.Predict(ctx, j.params)
				callback(j.idx, prediction, err)
			}
		}()
	}
	wg.Wait()
}
