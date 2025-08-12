package propagation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/la5nta/pat/internal/debug"
)

// predictionCache is a singleton for caching prediction results.
var predictionCache = newCache()

// CachingPredictor is a middleware that adds caching to a Predictor.
type CachingPredictor struct {
	predictor Predictor
	cache     *cache
}

// WithCaching wraps a Predictor with a caching layer.
func WithCaching(predictor Predictor) *CachingPredictor {
	return &CachingPredictor{
		predictor: predictor,
		cache:     predictionCache,
	}
}

// Predict implements the Predictor interface, adding a caching layer.
func (p *CachingPredictor) Predict(ctx context.Context, params PredictionParams) (*Prediction, error) {
	key := cacheKey(params)

	// Lock based on the cache key to prevent concurrent predictions for the same params.
	mu := p.cache.lock(key)
	mu.Lock()
	defer mu.Unlock()

	// Check cache
	if pred, err := p.cache.get(key); err == nil {
		return pred, nil
	}

	// Run prediction if not in cache
	pred, err := p.predictor.Predict(ctx, params)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := p.cache.set(key, pred); err != nil {
		debug.Printf("failed to write prediction cache: %v", err)
	}
	return pred, nil
}

// cache manages the file-based cache for predictions.
type cache struct {
	dir   string
	locks map[string]*sync.Mutex
	mu    sync.Mutex
}

// newCache creates a new cache instance.
func newCache() *cache {
	dir := filepath.Join(os.TempDir(), "pat-prediction-cache")
	if err := os.MkdirAll(dir, 0755); err != nil {
		debug.Printf("Failed to create prediction cache key: %s", err)
		return &cache{
			locks: make(map[string]*sync.Mutex),
		}
	}
	return &cache{
		dir:   dir,
		locks: make(map[string]*sync.Mutex),
	}
}

// cacheKey generates a cache key from the prediction parameters.
func cacheKey(params PredictionParams) string {
	keyString := fmt.Sprint(
		params.Time.Year(),
		int(params.Time.Month()),
		params.Time.Hour(),
		params.SSN,
		params.From,
		params.To,
		int(float64(params.Frequency)/1e6), // The integer part of the frequency in MHz is close enough for caching.
		params.TransmitPower,
	)
	hash := sha256.Sum256([]byte(keyString))
	return hex.EncodeToString(hash[:])
}

// get returns the cached prediction for the given key.
func (c *cache) get(key string) (*Prediction, error) {
	if c.dir == "" {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(c.dir, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pred Prediction
	if err := json.Unmarshal(data, &pred); err != nil {
		return nil, err
	}
	return &pred, nil
}

// set stores a prediction in the cache.
func (c *cache) set(key string, pred *Prediction) error {
	if c.dir == "" {
		return nil // Caching is disabled
	}
	data, err := json.Marshal(pred)
	if err != nil {
		return err
	}
	path := filepath.Join(c.dir, key)
	return os.WriteFile(path, data, 0644)
}

// lock returns a mutex for the given key.
func (c *cache) lock(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.locks[key]; !ok {
		c.locks[key] = &sync.Mutex{}
	}
	return c.locks[key]
}
