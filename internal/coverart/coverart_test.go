package coverart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"salland1-metadata-wordpress/internal/cache"
)

// roundTripFunc lets tests stub out HTTP without hitting the network.
type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// newTestResolver returns a Resolver whose iTunes/special/hub traffic is
// handled by the given round-trip function, and whose WordPress cache already
// holds the supplied avatar URL.
func newTestResolver(rt roundTripFunc, avatarURL string) *Resolver {
	appCache := cache.New()
	if avatarURL != "" {
		appCache.Set(map[string]interface{}{
			"broadcast": map[string]interface{}{
				"current_show": map[string]interface{}{
					"show": map[string]interface{}{
						"name":       "Test Show",
						"avatar_url": avatarURL,
					},
				},
			},
		})
	}
	cfg := &Config{
		Enabled:         true,
		HubURL:          "http://hub.test:9000",
		HubInput:        "radio-cover-art",
		HubSecret:       "sekret",
		FallbackImage:   "https://station.example/fallback.jpg",
		ITunesCountry:   "nl",
		ITunesLimit:     5,
		SpecialTrigger:  "Sallandschijf - ",
		SpecialURL:      "http://special.test/api",
		CacheTTL:        time.Hour,
		MinInterval:     time.Millisecond,
		ErrorCooldown:   time.Minute,
		MinMusicSeconds: 120,
	}
	r := New(cfg, appCache)
	r.client = &http.Client{Transport: roundTripFunc(rt)}
	return r
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ABBA", "abba"},
		{"Dancing Queen", "dancing queen"},
		{"  Queen   -   Bohemian Rhapsody  ", "queen bohemian rhapsody"},
		{"AC/DC", "ac dc"},
		{"The Beatles [Remastered]", "the beatles"},
		{"Hello (Edit)", "hello"},
		{"Song - Live", "song"},
		{"Café de la Paix", "café de la paix"},
	}
	for _, c := range cases {
		if got := normalize(c.in); got != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	if s := similarity("ABBA", "abba"); s != 1.0 {
		t.Errorf("exact match similarity = %v, want 1.0", s)
	}
	if s := similarity("Dancing Queen", "Dancing Queen 2015"); s != 0.9 {
		t.Errorf("containment similarity = %v, want 0.9", s)
	}
	if s := similarity("Madonna", "Madonna"); s != 1.0 {
		t.Errorf("identical similarity = %v, want 1.0", s)
	}
	if s := similarity("ABBA", "The Rolling Stones"); s >= 0.6 {
		t.Errorf("dissimilar strings scored too high: %v", s)
	}
}

func TestParseDurationToSeconds(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"272", 272, true},
		{"272.5", 272, true},
		{"272,5", 272, true},
		{"3:45", 225, true},
		{"03:45", 225, true},
		{"1:30:00", 5400, true},
		{"0:30", 30, true},
		{"120", 120, true},
		{"119", 119, true},
		{"", 0, false},
		{"abc", 0, false},
		{"1:2:3:4", 0, false},
	}
	for _, c := range cases {
		got, ok := parseDurationToSeconds(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseDurationToSeconds(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestIsMusic(t *testing.T) {
	mk := func(artist, duration string) Request {
		var p Request
		p.NowPlaying.Artist = artist
		p.NowPlaying.Duration = duration
		return p
	}
	if !isMusic(mk("ABBA", "3:45"), 120) {
		t.Error("song >= 2min with artist should be music")
	}
	if isMusic(mk("ABBA", "1:30"), 120) {
		t.Error("song < 2min should not be music")
	}
	if isMusic(mk("", "3:45"), 120) {
		t.Error("no artist should not be music")
	}
	if !isMusic(mk("ABBA", "180"), 120) {
		t.Error("seconds duration should be music")
	}
	if !isMusic(mk("ABBA", ""), 120) {
		t.Error("unparseable duration with artist should pass (matching hub filter behaviour)")
	}
}

func TestBuildSearchQuery(t *testing.T) {
	if got := buildSearchQuery("Weeks & Company", "Rock Your World (Joho, Joho)", "Weeks & Company - Rock Your World (Joho, Joho)"); got != "Weeks & Company - Rock Your World" {
		t.Errorf("buildSearchQuery = %q", got)
	}
	if got := buildSearchQuery("Queen", "Bohemian Rhapsody [Remastered]", ""); got != "Queen Bohemian Rhapsody" {
		t.Errorf("buildSearchQuery = %q", got)
	}
}

func TestUpgradeArtworkURL(t *testing.T) {
	got := upgradeArtworkURL("http://is1.mzstatic.com/image/thumb/100x100bb.jpg")
	want := "https://is1.mzstatic.com/image/thumb/600x600bb.jpg"
	if got != want {
		t.Errorf("upgradeArtworkURL = %q, want %q", got, want)
	}
	// localhost is exempt from https forcing
	if got := forceHTTPS("http://localhost:8080/x.jpg"); got != "http://localhost:8080/x.jpg" {
		t.Errorf("forceHTTPS should leave localhost alone, got %q", got)
	}
}

func TestTrackCache(t *testing.T) {
	c := newTrackCache(50 * time.Millisecond)
	c.set("abba\u0000dancing queen", "https://example.com/art.jpg")
	if url, ok := c.get("abba\u0000dancing queen"); !ok || url != "https://example.com/art.jpg" {
		t.Fatalf("expected cached url, got %q ok=%v", url, ok)
	}
	time.Sleep(70 * time.Millisecond)
	if _, ok := c.get("abba\u0000dancing queen"); ok {
		t.Fatal("expected entry to expire")
	}
}

// iTunes JSON builder for a single result.
func itunesResultsJSON(artist, track, artwork string) string {
	results := []map[string]interface{}{
		{"artistName": artist, "trackName": track, "artworkUrl100": artwork},
	}
	payload, _ := json.Marshal(map[string]interface{}{"resultCount": len(results), "results": results})
	return string(payload)
}

func TestResolveViaITunes(t *testing.T) {
	var pushedURL string
	var pushErr error
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushedURL = r.URL.Query().Get("title")
		if r.URL.Query().Get("input") != "radio-cover-art" {
			pushErr = fmt.Errorf("unexpected input param %q", r.URL.Query().Get("input"))
		}
		if r.URL.Query().Get("secret") != "sekret" {
			pushErr = fmt.Errorf("unexpected secret")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer hub.Close()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "itunes.apple.com") {
			return jsonResponse(200, itunesResultsJSON("ABBA", "Dancing Queen", "http://is1.mzstatic.com/100x100bb.jpg")), nil
		}
		// Everything else (the real httptest hub) passes through to the
		// default transport so the push-back actually reaches the server.
		return http.DefaultTransport.RoundTrip(r)
	})

	r := newTestResolver(rt, "http://wp.example/avatar.jpg")
	r.cfg.HubURL = hub.URL

	payload, _ := json.Marshal(map[string]interface{}{
		"station": "Salland1",
		"now_playing": map[string]interface{}{
			"artist":   "ABBA",
			"title":    "Dancing Queen",
			"text":     "ABBA - Dancing Queen",
			"duration": "3:35",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/cover-art", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	r.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if pushErr != nil {
		t.Fatal(pushErr)
	}
	if !strings.Contains(pushedURL, "600x600bb") {
		t.Fatalf("expected upgraded iTunes artwork pushed to hub, got %q", pushedURL)
	}
	if !strings.HasPrefix(pushedURL, "https:") {
		t.Fatalf("expected https artwork pushed to hub, got %q", pushedURL)
	}
}

func TestResolveFallbackToShowAvatar(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// iTunes returns no acceptable match.
		return jsonResponse(200, `{"resultCount":1,"results":[{"artistName":"Wrong","trackName":"Thing","artworkUrl100":"http://x/100x100bb.jpg"}]}`), nil
	})
	r := newTestResolver(rt, "http://wp.example/avatar.jpg")

	var p Request
	p.NowPlaying.Artist = "Someone"
	p.NowPlaying.Title = "A Track"
	p.NowPlaying.Text = "Someone - A Track"
	p.NowPlaying.Duration = "3:00"

	url := r.resolve(p)
	if url != "https://wp.example/avatar.jpg" {
		t.Fatalf("expected show avatar fallback, got %q", url)
	}
}

func TestResolveFallbackImage(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"resultCount":0,"results":[]}`), nil
	})
	r := newTestResolver(rt, "") // no avatar cached

	var p Request
	p.NowPlaying.Artist = "Nobody"
	p.NowPlaying.Title = "Nothing"
	p.NowPlaying.Duration = "2:30"

	url := r.resolve(p)
	if url != "https://station.example/fallback.jpg" {
		t.Fatalf("expected fallback image, got %q", url)
	}
}

func TestResolveSpecialSong(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "special.test") {
			return jsonResponse(200, `{"image_url":"http://wp.example/special.jpg","active":true}`), nil
		}
		return jsonResponse(200, `{"resultCount":0,"results":[]}`), nil
	})
	r := newTestResolver(rt, "http://wp.example/avatar.jpg")

	var p Request
	p.NowPlaying.Title = "Sallandschijf - Some Special Song"
	p.NowPlaying.Artist = "Some Artist"
	p.NowPlaying.Duration = "3:00"

	url := r.resolve(p)
	if url != "https://wp.example/special.jpg" {
		t.Fatalf("expected special song artwork, got %q", url)
	}
}

func TestResolveSpecialSongInactive(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "special.test") {
			return jsonResponse(200, `{"image_url":"http://wp.example/special.jpg","active":false}`), nil
		}
		return jsonResponse(200, `{"resultCount":0,"results":[]}`), nil
	})
	r := newTestResolver(rt, "")

	var p Request
	p.NowPlaying.Title = "Sallandschijf - Some Special Song"
	p.NowPlaying.Artist = "Some Artist"
	p.NowPlaying.Duration = "3:00"

	if url := r.resolve(p); url != "https://station.example/fallback.jpg" {
		t.Fatalf("inactive special song should fall through to fallback, got %q", url)
	}
}

func TestHandleValidatesInput(t *testing.T) {
	r := newTestResolver(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"resultCount":0,"results":[]}`), nil
	}, "")

	// Invalid JSON -> 400
	req := httptest.NewRequest(http.MethodPost, "/cover-art", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	r.Handle(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON should be 400, got %d", rr.Code)
	}

	// Missing now_playing -> 400
	req = httptest.NewRequest(http.MethodPost, "/cover-art", bytes.NewBufferString(`{"station":"Salland1"}`))
	rr = httptest.NewRecorder()
	r.Handle(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing now_playing should be 400, got %d", rr.Code)
	}

	// GET -> 405
	req = httptest.NewRequest(http.MethodGet, "/cover-art", nil)
	rr = httptest.NewRecorder()
	r.Handle(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET should be 405, got %d", rr.Code)
	}
}

func TestITunesCooldownOnRateLimit(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("Forbidden"))}, nil
	})
	r := newTestResolver(rt, "")
	r.cfg.MinInterval = 0 // allow immediate retries so cooldown is the only guard

	var p Request
	p.NowPlaying.Artist = "ABBA"
	p.NowPlaying.Title = "Dancing Queen"
	p.NowPlaying.Duration = "3:35"

	r.resolve(p) // first call -> 403 -> cooldown
	r.resolve(p) // second call should be blocked by cooldown

	if calls != 1 {
		t.Fatalf("expected iTunes to be called once during cooldown, got %d calls", calls)
	}
}

func TestNormalizeArtist(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Dua Lipa feat. DaBaby", "dua lipa"},
		{"Dua Lipa ft. DaBaby", "dua lipa"},
		{"Ed Sheeran featuring Justin Bieber", "ed sheeran"},
		{"The Police", "police"},
		{"The Beatles [Remastered]", "beatles"},
		{"AC/DC", "ac dc"},
		{"Simon & Garfunkel", "simon and garfunkel"},
		{"Café de la Paix", "café de la paix"},
	}
	for _, c := range cases {
		if got := normalizeArtist(c.in); got != c.want {
			t.Errorf("normalizeArtist(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestArtistSimilarity(t *testing.T) {
	if s := artistSimilarity("Dua Lipa", "Dua Lipa feat. DaBaby"); s != 1.0 {
		t.Errorf("feat. clause should not lower artist similarity, got %v", s)
	}
	if s := artistSimilarity("The Police", "Police"); s != 1.0 {
		t.Errorf("leading 'The' should be ignored, got %v", s)
	}
	if s := artistSimilarity("Simon & Garfunkel", "Simon And Garfunkel"); s < 0.95 {
		t.Errorf("& and 'and' should match closely, got %v", s)
	}
	if s := artistSimilarity("ABBA", "The Rolling Stones"); s >= artistMinMatch {
		t.Errorf("unrelated artists scored above the gate: %v", s)
	}
}

func TestCompilationFactor(t *testing.T) {
	check := func(name string, f, want float64) {
		t.Helper()
		if math.Abs(f-want) > 1e-9 {
			t.Errorf("%s factor = %v, want %v", name, f, want)
		}
	}
	single := iTunesResult{ArtistName: "ABBA", CollectionArtistName: "ABBA", CollectionName: "Arrival", TrackCount: 10}
	check("single-artist album", compilationFactor(single), 1.0)
	comp := iTunesResult{ArtistName: "ABBA", CollectionArtistName: "Various Artists"}
	check("various-artists", compilationFactor(comp), 0.6)
	dutch := iTunesResult{ArtistName: "ABBA", CollectionArtistName: "Verschillende artiesten"}
	check("dutch various-artists", compilationFactor(dutch), 0.6)
	multi := iTunesResult{ArtistName: "ABBA", CollectionArtistName: "Benny Andersson & Meryl Streep"}
	check("multi-artist album", compilationFactor(multi), 0.8)
	greatestHits := iTunesResult{ArtistName: "ABBA", CollectionName: "ABBA Gold: Greatest Hits (40th Anniversary Edition)", TrackCount: 19}
	check("greatest-hits name + track count", compilationFactor(greatestHits), 0.72)
	boxSet := iTunesResult{ArtistName: "ABBA", CollectionName: "Thank You For The Music", TrackCount: 163}
	check("box set", compilationFactor(boxSet), 0.7)
	unknown := iTunesResult{ArtistName: "ABBA"}
	check("unknown collection", compilationFactor(unknown), 1.0)
}

func TestVersionFactor(t *testing.T) {
	if f := versionFactor("Blinding Lights (Remix)", "Blinding Lights"); f != 0.65 {
		t.Errorf("remix should be penalised, got %v", f)
	}
	if f := versionFactor("Blinding Lights", "Blinding Lights"); f != 1.0 {
		t.Errorf("original should not be penalised, got %v", f)
	}
	if f := versionFactor("Blinding Lights (Remix)", "Blinding Lights (Remix)"); f != 1.0 {
		t.Errorf("query already a remix should not be penalised, got %v", f)
	}
	if f := versionFactor("Live Is Life", "Live Is Life"); f != 1.0 {
		t.Errorf("'Live Is Life' should not be treated as a live version, got %v", f)
	}
	if f := versionFactor("Song - Live", "Song"); f != 0.65 {
		t.Errorf("' - live' suffix should be penalised, got %v", f)
	}
}

func TestScoreCandidateGates(t *testing.T) {
	wrongArtist := iTunesResult{ArtistName: "The Rolling Stones", TrackName: "Dancing Queen", CollectionArtistName: "The Rolling Stones", ArtworkURL100: "http://x/wrong.jpg"}
	if _, ok := scoreCandidate(wrongArtist, "ABBA", "Dancing Queen"); ok {
		t.Error("wrong artist must be rejected even with a perfect title match")
	}
	wrongSong := iTunesResult{ArtistName: "ABBA", TrackName: "Mamma Mia", CollectionArtistName: "ABBA", ArtworkURL100: "http://x/wrong.jpg"}
	if _, ok := scoreCandidate(wrongSong, "ABBA", "Dancing Queen"); ok {
		t.Error("wrong song by the right artist must be rejected")
	}
	right := iTunesResult{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionArtistName: "ABBA", ArtworkURL100: "http://x/right.jpg"}
	if _, ok := scoreCandidate(right, "ABBA", "Dancing Queen"); !ok {
		t.Error("correct artist+song should be accepted")
	}
}

func TestBestCandidatePrefersAlbumOverCompilation(t *testing.T) {
	results := []iTunesResult{
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionArtistName: "Various Artists", ArtworkURL100: "http://x/comp.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionArtistName: "ABBA", ArtworkURL100: "http://x/album.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen (Remix)", CollectionArtistName: "ABBA", ArtworkURL100: "http://x/remix.jpg"},
	}
	best, _, ok := bestCandidate(results, "ABBA", "Dancing Queen")
	if !ok {
		t.Fatal("expected a candidate")
	}
	if !strings.Contains(best.ArtworkURL100, "album.jpg") {
		t.Fatalf("expected the single-artist album cover to win, got %q", best.ArtworkURL100)
	}
}

func TestBestCandidateRealDancingQueenPool(t *testing.T) {
	// Real iTunes (nl) result pool for "ABBA - Dancing Queen" captured
	// 2026-08-08. The original album "Arrival" must beat the greatest-hits,
	// live, box-set and soundtrack versions that iTunes ranks above it.
	pool := []iTunesResult{
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "ABBA Gold: Greatest Hits (40th Anniversary Edition)", TrackCount: 19, ArtworkURL100: "http://x/gold40.jpg"},
		{ArtistName: "Meryl Streep, Julie Walters & Christine Baranski", TrackName: "Dancing Queen", CollectionName: "Mamma Mia! (The Movie Soundtrack)", TrackCount: 22, ArtworkURL100: "http://x/mamma.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen (Live)", CollectionName: "Live at Wembley Arena", TrackCount: 20, ArtworkURL100: "http://x/live.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "Thank You For The Music", TrackCount: 163, ArtworkURL100: "http://x/box.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "The Singles (The First Fifty Years)", TrackCount: 56, ArtworkURL100: "http://x/singles.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "70s Mixtape", CollectionArtistName: "Verschillende artiesten", TrackCount: 40, ArtworkURL100: "http://x/mixtape.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "ABBA Gold: Greatest Hits", TrackCount: 19, ArtworkURL100: "http://x/gold.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "The Essential Collection", TrackCount: 30, ArtworkURL100: "http://x/essential.jpg"},
		{ArtistName: "ABBA", TrackName: "Dancing Queen", CollectionName: "Arrival (Bonus Track Version)", TrackCount: 10, ArtworkURL100: "http://x/arrival.jpg"},
	}
	best, _, ok := bestCandidate(pool, "ABBA", "Dancing Queen")
	if !ok {
		t.Fatal("expected a candidate from the real pool")
	}
	if !strings.Contains(best.ArtworkURL100, "arrival.jpg") {
		t.Fatalf("expected the original Arrival album cover to win, got %q (%s)", best.ArtworkURL100, best.CollectionName)
	}
}

func TestSearchITunesTitleRescue(t *testing.T) {
	var calls []string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		term := r.URL.Query().Get("term")
		calls = append(calls, term)
		// Primary ("artist - title") finds nothing; the title-only rescue does.
		if strings.Contains(term, "-") {
			return jsonResponse(200, `{"resultCount":0,"results":[]}`), nil
		}
		return jsonResponse(200, itunesResultsJSON("Dua Lipa", "Dance The Night", "http://is1.mzstatic.com/100x100bb.jpg")), nil
	})
	r := newTestResolver(rt, "")
	r.cfg.ITunesLimit = 10

	url, err := r.searchITunes("Dua Lipa", "Dance The Night", "Dua Lipa - Dance The Night")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "600x600bb") {
		t.Fatalf("expected artwork via title rescue, got %q", url)
	}
	if len(calls) != 2 {
		t.Fatalf("expected primary + rescue pass, got %d calls (%v)", len(calls), calls)
	}
}

func TestSearchITunesRescueBlocksDifferentArtist(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		term := r.URL.Query().Get("term")
		if strings.Contains(term, "-") {
			return jsonResponse(200, `{"resultCount":0,"results":[]}`), nil
		}
		// A karaoke/cover artist with an exact title must NOT be rescued.
		return jsonResponse(200, itunesResultsJSON("Karaoke All-Stars", "Dancing Queen", "http://is1.mzstatic.com/100x100bb.jpg")), nil
	})
	r := newTestResolver(rt, "")
	r.cfg.ITunesLimit = 10

	url, err := r.searchITunes("ABBA", "Dancing Queen", "ABBA - Dancing Queen")
	if err != nil {
		t.Fatal(err)
	}
	if url != "" {
		t.Fatalf("different-artist cover must not be rescued, got %q", url)
	}
}

func TestSearchITunesStopsAfterStrongPrimary(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse(200, itunesResultsJSON("ABBA", "Dancing Queen", "http://is1.mzstatic.com/100x100bb.jpg")), nil
	})
	r := newTestResolver(rt, "")
	r.cfg.ITunesLimit = 10

	url, err := r.searchITunes("ABBA", "Dancing Queen", "ABBA - Dancing Queen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "600x600bb") {
		t.Fatalf("expected artwork from strong primary match, got %q", url)
	}
	if calls != 1 {
		t.Fatalf("expected a single API call for a strong match, got %d", calls)
	}
}
