package patapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const ApiBaseURL = "https://api.getpat.io/v1"

type LatestRelease struct {
	Version    string `json:"version"`
	ReleaseURL string `json:"release_url"`
}

// GetLatestVersion retrieves the latest release info from the pat API
func GetLatestVersion(ctx context.Context) (*LatestRelease, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", ApiBaseURL+"/releases/latest", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting latest version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var release *LatestRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return release, nil
}
