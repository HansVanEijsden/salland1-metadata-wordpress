package fetcher

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"salland1-metadata-wordpress/internal/cache"
)

type Fetcher struct {
	sourceURL       string
	interval        time.Duration
	jitter          time.Duration
	cache           *cache.Cache
	httpClient      *http.Client
	successfulFetch bool
}

func New(sourceURL string, interval, jitter time.Duration, cache *cache.Cache) *Fetcher {
	return &Fetcher{
		sourceURL: sourceURL,
		interval:  interval,
		jitter:    jitter,
		cache:     cache,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		successfulFetch: false,
	}
}

func (f *Fetcher) Start(ctx context.Context) {
	// Perform initial fetch immediately
	f.fetch()

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Fetcher stopped")
			return
		case <-ticker.C:
			// Add jitter (random between -jitter and +jitter)
			jitterMs := time.Duration(rand.Int63n(int64(f.jitter.Milliseconds()*2))) - f.jitter
			slog.Debug("Adding jitter", "jitter_ms", jitterMs.Milliseconds())
			time.Sleep(jitterMs)
			f.fetch()
		}
	}
}

func (f *Fetcher) fetch() {
	slog.Debug("Fetching data from WordPress API", "url", f.sourceURL)

	req, err := http.NewRequest("GET", f.sourceURL, nil)
	if err != nil {
		slog.Error("Failed to create request", "error", err)
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed to fetch data", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected status code", "status", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read response body", "error", err)
		return
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		slog.Error("Failed to parse JSON", "error", err)
		return
	}

	// Cache successful response
	f.cache.Set(data)
	f.successfulFetch = true
	slog.Info("Successfully fetched and cached data", "url", f.sourceURL)
}

// HasSuccessfulFetch returns true if at least one successful fetch has occurred
func (f *Fetcher) HasSuccessfulFetch() bool {
	return f.successfulFetch
}
