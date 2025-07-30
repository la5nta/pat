package noaa

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

var SolarCyclePredictionURL = "https://services.swpc.noaa.gov/json/solar-cycle/predicted-solar-cycle.json"

// PredictedSSN represents a single solar cycle prediction entry.
type PredictedSSN struct {
	// TimeTag is the date of the prediction. Format: "YYYY-MM".
	TimeTag string `json:"time-tag"`
	// PredictedSSN is the predicted sunspot number.
	PredictedSSN float64 `json:"predicted_ssn"`
}

// GetPredictedSSN fetches the predicted solar cycle data from the NOAA API.
func GetPredictedSSN(ctx context.Context) ([]PredictedSSN, error) {
	return GetPredictedSSNCached(ctx, "")
}

// GetPredictedSSNCached fetches the predicted solar cycle data from the NOAA API, using a file cache to store the data.
//
// It uses Etag to validate the cache. If the API is unreachable or otherwise errors, the stale cache is returned if available.
// If cachePath is empty, caching is disabled.
func GetPredictedSSNCached(ctx context.Context, cachePath string) ([]PredictedSSN, error) {
	// CachedSSNData wraps the prediction data with an Etag for caching.
	type CachedSSNData struct {
		Etag        string         `json:"etag"`
		Predictions []PredictedSSN `json:"predictions"`
	}

	// Try to read from cache first. It's ok if it fails.
	var cachedData CachedSSNData
	if cachePath != "" {
		if cachedBytes, _ := os.ReadFile(cachePath); cachedBytes != nil {
			_ = json.Unmarshal(cachedBytes, &cachedData)
		}
	}

	// Prepare request
	req, err := http.NewRequest("GET", SolarCyclePredictionURL, nil)
	if err != nil { // Should not happen
		return nil, err
	}
	req = req.WithContext(ctx)
	if cachedData.Etag != "" {
		req.Header.Set("If-None-Match", cachedData.Etag)
	}

	// Perform request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Network error, return stale cache if we have it.
		if len(cachedData.Predictions) > 0 {
			return cachedData.Predictions, nil
		}
		return nil, fmt.Errorf("failed to get solar cycle prediction: %w", err)
	}
	defer resp.Body.Close()

	// Handle response
	if resp.StatusCode == http.StatusNotModified {
		return cachedData.Predictions, nil
	}
	log.Println("Old ETag:", cachedData.Etag)
	log.Println("New ETag:", resp.Header.Get("Etag"))

	if resp.StatusCode != http.StatusOK {
		// Other error, return stale cache if we have it.
		if len(cachedData.Predictions) > 0 {
			return cachedData.Predictions, nil
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Got new data (200 OK)
	var predictions []PredictedSSN
	if err := json.NewDecoder(resp.Body).Decode(&predictions); err != nil {
		// JSON decoding error, return stale cache if we have it.
		if len(cachedData.Predictions) > 0 {
			return cachedData.Predictions, nil
		}
		return nil, fmt.Errorf("failed to decode solar cycle prediction: %w", err)
	}

	if cachePath != "" {
		// New data is valid, save it to cache.
		cachedData.Etag = resp.Header.Get("Etag")
		cachedData.Predictions = predictions
		if newCachedBytes, err := json.Marshal(cachedData); err == nil {
			_ = os.WriteFile(cachePath, newCachedBytes, 0644)
		}
	}

	log.Println("NOAA solar cycle (sunspot predictions) updated")
	return predictions, nil
}
