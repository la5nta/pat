package silso

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/internal/debug"
)

// Improved 12-month ahead predictions obtained by applying an adaptative
// Kalman filter to the primary Combined Method (CM) predictions from
// WDC-SILSO. This technique improves the approximation of the smoothed monthly
// sunspot number over the last 6 months, i.e. the interval between the last
// available smoothed value of the sunspot number and the current month of the
// prediction.
//
// https://link.springer.com/article/10.1007/s11207-011-9899-y
var SourceURL = "https://www.sidc.be/SILSO/FORECASTS/KFprediCM.txt"

//go:embed KFprediCM.txt
var embedded []byte

// Prediction represents a single SSN prediction entry.
type Prediction struct {
	TimeTag      string
	PredictedSSN float64
}

// GetPredictedSSN fetches the predicted solar cycle from the SIDC API.
func GetPredictedSSN(ctx context.Context) ([]Prediction, error) {
	return GetPredictedSSNCached(ctx, "")
}

// GetPredictedSSNCached fetches the predicted solar cycle data from the SIDC API, using a file cache to store the data.
//
// It uses the Last-Modified header to validate the cache. If the API is unreachable or otherwise errors, the stale cache is returned if available.
// If cachePath is empty, caching is disabled.
func GetPredictedSSNCached(ctx context.Context, cachePath string) ([]Prediction, error) {
	// CachedSSNData wraps the prediction data with an LastModified for caching.
	type CachedSSNData struct {
		LastModified time.Time    `json:"last_modified"`
		Predictions  []Prediction `json:"predictions"`
	}

	// Try to read from cache first. It's ok if it fails.
	var cachedData CachedSSNData
	if cachePath != "" {
		if cachedBytes, _ := os.ReadFile(cachePath); cachedBytes != nil {
			_ = json.Unmarshal(cachedBytes, &cachedData)
		}
		// Skip fetching if we have data from this month
		if now := time.Now(); cachedData.LastModified.Month() == now.Month() && cachedData.LastModified.Year() == now.Year() {
			debug.Printf("SILSO data up to date (%s)", cachedData.LastModified)
			return cachedData.Predictions, nil
		}
	}

	// Use embedded copy as fallback in case the cache is empty and the service is unavailable
	if len(cachedData.Predictions) == 0 {
		cachedData.Predictions, _ = parse(bytes.NewReader(embedded))
		debug.Printf("SILSO embedded data loaded")
	}

	// Prepare request
	req, err := http.NewRequest("GET", SourceURL, nil)
	if err != nil { // Should not happen
		return nil, err
	}
	req = req.WithContext(ctx)
	if !cachedData.LastModified.IsZero() {
		req.Header.Set("If-Modified-Since", cachedData.LastModified.Format(http.TimeFormat))
	}

	// Perform request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Network error, return stale cache if we have it.
		if len(cachedData.Predictions) > 0 {
			debug.Printf("SILSO endpoint unavailable: %s", err)
			return cachedData.Predictions, nil
		}
		return nil, fmt.Errorf("failed to get solar cycle prediction: %w", err)
	}
	defer resp.Body.Close()

	// Handle response
	if resp.StatusCode == http.StatusNotModified {
		debug.Printf("SILSO data not modified")
		return cachedData.Predictions, nil
	}

	if resp.StatusCode != http.StatusOK {
		// Other error, return stale cache if we have it.
		if len(cachedData.Predictions) > 0 {
			debug.Printf("SILSO endpoint unexpected status code: %d", resp.StatusCode)
			return cachedData.Predictions, nil
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Got new data (200 OK)
	predictions, err := parse(resp.Body)
	if err != nil {
		// Parsing error, return stale cache if we have it.
		if len(cachedData.Predictions) > 0 {
			debug.Printf("SILSO data parse error: %s", err)
			return cachedData.Predictions, nil
		}
		return nil, fmt.Errorf("failed to decode solar cycle prediction: %w", err)
	}

	if cachePath != "" {
		// New data is valid, save it to cache.
		t, err := http.ParseTime(resp.Header.Get("Last-Modified"))
		if err != nil {
			t = time.Now()
		}
		cachedData.LastModified = t
		cachedData.Predictions = predictions
		if newCachedBytes, err := json.Marshal(cachedData); err == nil {
			_ = os.WriteFile(cachePath, newCachedBytes, 0644)
		}
	}

	log.Println("SIDC solar cycle (sunspot predictions) updated")
	return predictions, nil
}

func parse(r io.Reader) ([]Prediction, error) {
	var predictions []Prediction
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		year, month := fields[0], fields[1]
		ssn, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			continue
		}
		predictions = append(predictions, Prediction{
			TimeTag:      year + "-" + month,
			PredictedSSN: ssn,
		})
	}
	return predictions, scanner.Err()
}
