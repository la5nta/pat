package propagation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// voacapAPIPredictor implements the Predictor interface using a remote VOACAP API.
type voacapAPIPredictor struct {
	voacapPredictor
	apiURL string
}

// NewVOACAPAPIPredictor creates a new VOACAP API predictor that calls an external API.
func NewVOACAPAPIPredictor(apiURL string) *voacapAPIPredictor {
	return &voacapAPIPredictor{apiURL: apiURL}
}

// Version returns the version information from the remote VOACAP API.
func (p *voacapAPIPredictor) Version() string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url, err := url.JoinPath(p.apiURL, "/version")
	if err != nil {
		return fmt.Sprintf("invalid API url: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Sprintf("failed to create version request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("failed to get version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return resp.Status
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("failed to read version response: %v", err)
	}
	return string(bytes.TrimSpace(body))
}

// Predict implements the Predictor interface by calling the external VOACAP API.
func (p *voacapAPIPredictor) Predict(ctx context.Context, params PredictionParams) (*Prediction, error) {
	inputFile, err := p.generateInputFile(params)
	if err != nil {
		return nil, fmt.Errorf("failed to generate input file: %w", err)
	}

	url, err := url.JoinPath(p.apiURL, "/predict")
	if err != nil {
		return nil, fmt.Errorf("invalid API url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(inputFile))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	return p.parseOutput(resp.Body)
}
