package fetcher

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"salland1-metadata-wordpress/internal/cache"
	"salland1-metadata-wordpress/internal/parser"
)

type Fetcher struct {
	sourceURL  string
	interval   time.Duration
	jitter     time.Duration
	cache      *cache.Cache
	httpClient *http.Client

	mu              sync.Mutex
	successfulFetch bool

	// Excerpt fetch state. Only touched from the fetch goroutine (Start), so it
	// needs no extra locking.
	lastRoute      string
	excerptFetched bool
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
			jitterDuration := time.Duration(rand.Int63n(int64(f.jitter.Milliseconds()*2))) - f.jitter
			slog.Debug("Adding jitter", "jitter_ms", jitterDuration.Milliseconds())
			time.Sleep(jitterDuration)
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
	f.mu.Lock()
	f.successfulFetch = true
	f.mu.Unlock()
	slog.Info("Successfully fetched and cached data", "url", f.sourceURL)

	// The programme excerpt lives on the current show's route endpoint, not in
	// the main response, so fetch it separately.
	f.fetchExcerpt(data)
}

// HasSuccessfulFetch returns true if at least one successful fetch has occurred
func (f *Fetcher) HasSuccessfulFetch() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.successfulFetch
}

// fetchExcerpt fetches the current show's route endpoint to obtain the
// programme excerpt (a short programme description), which is not present in
// the main metadata response. It only re-fetches when the route (i.e. the
// current show) changes, and degrades gracefully on failure by leaving the
// previously cached excerpt intact (or empty on the first fetch).
func (f *Fetcher) fetchExcerpt(sourceData interface{}) {
	route := parser.ParseRoute(sourceData)
	if route == "" {
		f.cache.SetExcerpt("")
		f.lastRoute = ""
		f.excerptFetched = false
		return
	}

	// Reuse the cached excerpt while the current show hasn't changed.
	if f.excerptFetched && route == f.lastRoute {
		return
	}

	slog.Debug("Fetching programme excerpt", "url", route)

	req, err := http.NewRequest("GET", route, nil)
	if err != nil {
		slog.Error("Failed to create excerpt request", "error", err, "url", route)
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed to fetch programme excerpt", "error", err, "url", route)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected status code fetching excerpt", "status", resp.StatusCode, "url", route)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read excerpt response body", "error", err, "url", route)
		return
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		slog.Error("Failed to parse excerpt JSON", "error", err, "url", route)
		return
	}

	excerpt := parser.ParseExcerpt(data)
	f.cache.SetExcerpt(excerpt)
	f.lastRoute = route
	f.excerptFetched = true
	slog.Debug("Cached programme excerpt", "url", route, "excerpt", excerpt)
}
