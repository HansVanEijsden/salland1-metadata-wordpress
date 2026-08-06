package coverart

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"salland1-metadata-wordpress/internal/cache"
	"salland1-metadata-wordpress/internal/parser"
)

// Request is the payload the metadata hub sends on every track change (the
// payloadMapping of the hub's radio-cover-art-req output).
type Request struct {
	Station    string `json:"station"`
	NowPlaying struct {
		Title    string `json:"title"`
		Artist   string `json:"artist"`
		Text     string `json:"text"`
		Duration string `json:"duration"`
	} `json:"now_playing"`
	Metadata struct {
		HasArtist string `json:"has_artist"`
		Source    string `json:"source"`
	} `json:"metadata"`
}

// Resolver resolves album art for now-playing metadata and pushes the result
// back to the metadata hub. It is safe for concurrent use.
type Resolver struct {
	cfg    *Config
	client *http.Client
	cache  *cache.Cache

	// iTunes guard: serialises API calls, enforces a minimum interval and a
	// cooldown after rate-limit/errors.
	mu            sync.Mutex
	lastITunes    time.Time
	cooldownUntil time.Time

	tracks *trackCache

	currentMu sync.RWMutex
	current   string // last successfully pushed artwork URL (for GET /cover-art)
}

// New builds a Resolver. appCache is the service's shared WordPress metadata
// cache, used for the show-avatar fallback.
func New(cfg *Config, appCache *cache.Cache) *Resolver {
	return &Resolver{
		cfg: cfg,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		cache:  appCache,
		tracks: newTrackCache(cfg.CacheTTL),
	}
}

// Handle is the HTTP handler for POST /cover-art. It parses the hub's payload,
// resolves the artwork and pushes the result back to the hub. It always
// answers 200 once the payload is valid, even when resolution fails, so the
// hub does not retry/queue.
func (r *Resolver) Handle(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload Request
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.NowPlaying.Title) == "" && strings.TrimSpace(payload.NowPlaying.Artist) == "" {
		http.Error(w, "now_playing.title/artist required", http.StatusBadRequest)
		return
	}

	artworkURL := r.resolve(payload)

	if err := r.pushToHub(artworkURL); err != nil {
		slog.Error("Failed to push artwork to hub", "error", err)
	} else {
		slog.Info("Cover art pushed to hub", "artwork", artworkURL)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Current is the HTTP handler for GET /cover-art, returning the last artwork
// URL that was pushed to the hub (useful for debugging and as a polling input
// alternative). Returns 204 when nothing has been resolved yet.
func (r *Resolver) Current(w http.ResponseWriter, req *http.Request) {
	r.currentMu.RLock()
	current := r.current
	r.currentMu.RUnlock()
	if current == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(current))
}

// resolve runs the artwork resolution pipeline and returns the final URL. It
// never fails: worst case it returns the configured fallback image.
func (r *Resolver) resolve(p Request) string {
	// Step 1: special song (e.g. Sallandschijf) when configured.
	if r.cfg.SpecialTrigger != "" && r.cfg.SpecialURL != "" {
		if url := r.resolveSpecial(p); url != "" {
			slog.Info("Cover art resolved via special song", "artwork", url, "station", p.Station)
			r.setCurrent(url)
			return url
		}
	}

	// Step 2: music → iTunes album art.
	if isMusic(p, r.cfg.MinMusicSeconds) {
		if url := r.resolveITunes(p); url != "" {
			slog.Info("Cover art resolved via iTunes", "artwork", url, "artist", p.NowPlaying.Artist, "title", p.NowPlaying.Title)
			r.setCurrent(url)
			return url
		}
	}

	// Step 3: current show avatar from the (already cached) WordPress API.
	if url := r.resolveShowAvatar(); url != "" {
		slog.Info("Cover art resolved via show avatar", "artwork", url)
		r.setCurrent(url)
		return url
	}

	// Step 4: station fallback image.
	slog.Info("Cover art resolved via fallback image", "artwork", r.cfg.FallbackImage)
	r.setCurrent(r.cfg.FallbackImage)
	return r.cfg.FallbackImage
}

// isMusic reports whether the payload looks like a playable song: it has an
// artist and a duration of at least minSeconds.
func isMusic(p Request, minSeconds int) bool {
	if strings.TrimSpace(p.NowPlaying.Artist) == "" {
		return false
	}
	seconds, ok := parseDurationToSeconds(p.NowPlaying.Duration)
	if !ok {
		// No parseable duration: fall back to "is music" based on artist only,
		// matching the hub's own duration filter behaviour (pass on unparseable).
		return true
	}
	return seconds >= minSeconds
}

// parseDurationToSeconds parses the hub's supported duration formats:
// "272", "272.5", "272,5", "3:45", "03:45", "1:30:00".
func parseDurationToSeconds(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) > 3 {
			return 0, false
		}
		total := 0
		for _, part := range parts {
			sec, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || sec < 0 {
				return 0, false
			}
			total = total*60 + sec
		}
		return total, true
	}
	// Plain seconds, allowing decimal comma or period.
	s = strings.Replace(s, ",", ".", 1)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int(f), true
}

// resolveSpecial fetches the special-song artwork (e.g. Sallandschijf) when the
// title or formatted text starts with the configured trigger.
func (r *Resolver) resolveSpecial(p Request) string {
	title := strings.TrimSpace(p.NowPlaying.Title)
	text := strings.TrimSpace(p.NowPlaying.Text)
	if !strings.HasPrefix(title, r.cfg.SpecialTrigger) && !strings.HasPrefix(text, r.cfg.SpecialTrigger) {
		return ""
	}

	slog.Info("Special song trigger detected", "trigger", r.cfg.SpecialTrigger, "url", r.cfg.SpecialURL)

	req, err := http.NewRequest("GET", r.cfg.SpecialURL, nil)
	if err != nil {
		slog.Error("Special song request build failed", "error", err)
		return ""
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		slog.Error("Special song API request failed", "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Special song API unexpected status", "status", resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		slog.Error("Special song API read failed", "error", err)
		return ""
	}

	var data struct {
		ImageURL string `json:"image_url"`
		Active   *bool  `json:"active"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		slog.Error("Special song API parse failed", "error", err)
		return ""
	}

	if data.ImageURL == "" {
		slog.Info("Special song API returned no image_url")
		return ""
	}
	if data.Active != nil && !*data.Active {
		slog.Info("Special song is not active per API")
		return ""
	}

	return forceHTTPS(data.ImageURL)
}

// resolveITunes resolves album art via the iTunes Search API, guarded by a
// track cache, a minimum call interval and an error cooldown so we never
// hammer the API (the reason the legacy PHP was blacklisted).
func (r *Resolver) resolveITunes(p Request) string {
	key := trackCacheKey(p.NowPlaying.Artist, p.NowPlaying.Title)
	if cached, ok := r.tracks.get(key); ok {
		return cached
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Re-check after acquiring the lock (another request may have resolved it).
	if cached, ok := r.tracks.get(key); ok {
		return cached
	}

	now := time.Now()
	if now.Before(r.cooldownUntil) {
		slog.Info("iTunes in cooldown, skipping", "until", r.cooldownUntil.Format(time.RFC3339))
		return ""
	}
	if now.Sub(r.lastITunes) < r.cfg.MinInterval {
		slog.Info("iTunes call throttled by minimum interval")
		return ""
	}

	query := buildSearchQuery(p.NowPlaying.Artist, p.NowPlaying.Title, p.NowPlaying.Text)
	artworkURL, err := r.searchITunes(p.NowPlaying.Artist, p.NowPlaying.Title, query)
	r.lastITunes = time.Now()
	if err != nil {
		slog.Error("iTunes search failed", "error", err)
		if isRateLimitError(err) {
			r.cooldownUntil = now.Add(r.cfg.ErrorCooldown)
			slog.Warn("iTunes rate-limit detected, entering cooldown", "cooldown", r.cfg.ErrorCooldown)
		}
		return ""
	}
	if artworkURL != "" {
		r.tracks.set(key, artworkURL)
	}
	return artworkURL
}

// resolveShowAvatar returns the avatar of the currently playing show from the
// service's shared WordPress cache (already fetched every minute by the
// fetcher, so no extra HTTP call).
func (r *Resolver) resolveShowAvatar() string {
	data, ok := r.cache.Get()
	if !ok {
		return ""
	}
	avatarURL := parser.Parse(data).AvatarURL
	if avatarURL == "" {
		return ""
	}
	return forceHTTPS(avatarURL)
}

// pushToHub sends the resolved artwork URL to the hub's dynamic input, which
// makes it available via the radio-cover-website websocket (and any other
// outputs wired to the radio-cover-art input).
func (r *Resolver) pushToHub(artworkURL string) error {
	if r.cfg.HubURL == "" {
		slog.Warn("COVERART_HUB_URL not set, skipping hub push")
		return fmt.Errorf("hub URL not configured")
	}
	if artworkURL == "" {
		return fmt.Errorf("empty artwork URL, nothing to push")
	}

	endpoint := fmt.Sprintf("%s/input/dynamic?input=%s&title=%s&secret=%s",
		strings.TrimRight(r.cfg.HubURL, "/"),
		url.QueryEscape(r.cfg.HubInput),
		url.QueryEscape(artworkURL),
		url.QueryEscape(r.cfg.HubSecret),
	)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "salland1-metadata-wordpress/coverart")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub returned status %d", resp.StatusCode)
	}
	return nil
}

func (r *Resolver) setCurrent(artworkURL string) {
	r.currentMu.Lock()
	r.current = artworkURL
	r.currentMu.Unlock()
}

// trackCacheKey builds a stable cache key from artist and title.
func trackCacheKey(artist, title string) string {
	return normalize(artist) + "\x00" + normalize(title)
}

// isRateLimitError reports whether an iTunes error suggests rate limiting.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "403") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "timeout")
}
