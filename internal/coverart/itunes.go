package coverart

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// iTunes candidates are scored with a weighted combination of artist and title
// similarity, then gated and penalised so wrong-artist, compilation and remix
// covers cannot win. The hard gates are the key anti-mismatch guards: a strong
// title no longer masks a wrong artist, and a strong artist no longer masks a
// wrong song.
const (
	artistWeight   = 0.6
	titleWeight    = 0.4
	minMatch       = 0.6 // minimum final score to accept a candidate
	artistMinMatch = 0.7 // hard gate: minimum artist similarity (primary pass)
	titleMinMatch  = 0.5 // hard gate: minimum title similarity (primary pass)
	strongMatch    = 0.85
	// The rescue pass relaxes the gates but keeps them safe: a clearly
	// different artist (e.g. karaoke covers) must never win.
	rescueArtistMin = 0.45
	rescueTitleMin  = 0.85
)

var (
	// reBracket strips [edit], [remaster] style annotations.
	reBracket = regexp.MustCompile(`\s*\[[^\]]*\]\s*`)
	// reParen strips (edit), (single version) style annotations.
	reParen = regexp.MustCompile(`\s*\([^)]*\)\s*`)
	// reSuffix strips " - remastered", "- live" style suffixes.
	reSuffix = regexp.MustCompile(`\s*-\s*(remastered|remaster|edit|single|version|mix|live|acoustic|original)\s*`)
	// reNonWord keeps letters, digits and whitespace (Unicode aware).
	reNonWord = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	// reSpace collapses runs of whitespace.
	reSpace = regexp.MustCompile(`\s+`)
	// reFeat drops "feat. X", "ft. X" and "featuring X" collaboration clauses
	// from artist names so "Dua Lipa" matches "Dua Lipa feat. DaBaby".
	reFeat = regexp.MustCompile(`(?i)\s*(?:feat\.?|ft\.?|featuring)\b.*$`)
	// reVersionTag flags titles that mark a non-original version. "live" is
	// deliberately excluded and handled separately (see reLiveSuffix) so songs
	// like "Live Is Life" are not penalised.
	reVersionTag = regexp.MustCompile(`(?i)\b(?:remix|extended|instrumental|karaoke|acoustic|remaster(?:ed)?|reprise|rework|bootleg|re-?record(?:ed|ing)?|radio edit|edit|club mix|club edit|dub mix|cover version)\b`)
	// reLiveSuffix flags " - live" / "(live)" title suffixes only.
	reLiveSuffix = regexp.MustCompile(`(?i)(?:\s*-\s*live|\s*\(\s*live\s*\))\s*$`)
	// reVarious matches collection artist names that mean "compilation"
	// (English and the Dutch names iTunes returns for the nl store).
	reVarious = regexp.MustCompile(`(?i)^(?:various(?:\s+artists)?|va|verschillende artiesten|diverse artiesten)$`)
	// reCompilationName matches collection names that indicate a greatest-hits /
	// best-of compilation or box set (e.g. "ABBA Gold: Greatest Hits"), which
	// iTunes ranks above the original album more often than not.
	reCompilationName = regexp.MustCompile(`(?i)\b(greatest hits|best of|the essential|essential|number ones|the singles|the very best|anthology|platinum|gold|collection|the ultimate|ultimate|the hits|hits|now that'?s what i call|100 hits|the definitive|icon|legend|mixtape)\b`)
)

// iTunesResult is the subset of the iTunes Search API response we consume.
type iTunesResult struct {
	ArtistName           string `json:"artistName"`
	TrackName            string `json:"trackName"`
	CollectionName       string `json:"collectionName"`
	CollectionArtistName string `json:"collectionArtistName"`
	TrackCount           int    `json:"trackCount"`
	ArtworkURL100        string `json:"artworkUrl100"`
}

type iTunesResponse struct {
	ResultCount int            `json:"resultCount"`
	Results     []iTunesResult `json:"results"`
}

// normalize mirrors the PHP normaliseer_string(): lowercases, removes common
// annotations, punctuation and collapses whitespace.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reBracket.ReplaceAllString(s, " ")
	s = reParen.ReplaceAllString(s, " ")
	s = reSuffix.ReplaceAllString(s, " ")
	s = reNonWord.ReplaceAllString(s, " ")
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// normalizeArtist normalizes an artist name for comparison: lowercases, drops
// feat. collaboration clauses and leading "the", turns "&" into "and", strips
// annotations and punctuation, and collapses whitespace. It makes "Dua Lipa"
// equal to "Dua Lipa feat. DaBaby" and "The Police" equal to "Police".
func normalizeArtist(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reFeat.ReplaceAllString(s, " ")
	s = reBracket.ReplaceAllString(s, " ")
	s = reParen.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&", " and ")
	s = reNonWord.ReplaceAllString(s, " ")
	s = reSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return strings.TrimPrefix(s, "the ")
}

// artistSimilarity is stricter than the general similarity(): it is semantic
// about collaboration markers, leading articles and ampersands, so genuine
// matches score high while unrelated artists stay far below the gate.
func artistSimilarity(a, b string) float64 {
	na, nb := normalizeArtist(a), normalizeArtist(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1.0
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return 0.95
	}
	maxLen := len(na)
	if len(nb) > maxLen {
		maxLen = len(nb)
	}
	if maxLen == 0 {
		return 0
	}
	return 1.0 - float64(levenshtein(na, nb))/float64(maxLen)
}

// levenshtein returns the edit distance between a and b (bytes).
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// similarity returns a value in [0,1] mirroring the PHP bereken_match_score():
// exact match after normalisation is 1.0, containment is 0.9, otherwise a
// Levenshtein-based similarity.
func similarity(a, b string) float64 {
	na, nb := normalize(a), normalize(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1.0
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return 0.9
	}
	maxLen := len(na)
	if len(nb) > maxLen {
		maxLen = len(nb)
	}
	if maxLen == 0 {
		return 0
	}
	return 1.0 - float64(levenshtein(na, nb))/float64(maxLen)
}

// compilationFactor scores how album-accurate a result's artwork is. iTunes
// frequently returns tracks from compilations ("Now That's What I Call Music",
// "ABBA Gold: Greatest Hits", box sets) whose cover is the compilation, not
// the single/album the artist actually released. Multi-artist "Various
// Artists" collections (English and Dutch), compilation-style collection names
// and very large collections are all penalised — so the artist's real album
// cover wins while a compilation stays usable as a last resort.
func compilationFactor(r iTunesResult) float64 {
	factor := 1.0
	ca := strings.TrimSpace(r.CollectionArtistName)
	switch {
	case reVarious.MatchString(ca):
		factor *= 0.6
	case ca != "" && normalizeArtist(ca) != normalizeArtist(r.ArtistName):
		factor *= 0.8
	}
	if reCompilationName.MatchString(strings.ToLower(r.CollectionName)) {
		factor *= 0.8
	}
	if r.TrackCount > 25 {
		factor *= 0.7 // box sets / mega compilations
	} else if r.TrackCount > 16 {
		factor *= 0.9 // large compilations
	}
	return factor
}

// versionFactor penalises results whose title marks a non-original version
// (remix, edit, live, instrumental, ...) when the playout title is the
// original. If the playout title itself contains the marker (the DJ is playing
// the remix), no penalty applies.
func versionFactor(resultTitle, queryTitle string) float64 {
	if !containsVersionTag(resultTitle) {
		return 1.0
	}
	if containsVersionTag(queryTitle) {
		return 1.0
	}
	return 0.65
}

// containsVersionTag reports whether a raw title marks a non-original version.
func containsVersionTag(s string) bool {
	if reVersionTag.MatchString(s) {
		return true
	}
	// "live" is only a version marker as a suffix, so "Live Is Life" passes.
	return reLiveSuffix.MatchString(s)
}

// scoreCandidate computes the match score for a single iTunes result, applying
// the artist/title gates and the compilation/version penalties. It reports
// false when the candidate must be rejected outright.
func scoreCandidate(r iTunesResult, artist, title string) (float64, bool) {
	artistScore := artistSimilarity(r.ArtistName, artist)
	if artistScore < artistMinMatch {
		return 0, false
	}
	titleScore := similarity(r.TrackName, title)
	if titleScore < titleMinMatch {
		return 0, false
	}
	base := artistScore*artistWeight + titleScore*titleWeight
	total := base * compilationFactor(r) * versionFactor(r.TrackName, title)
	return total, total >= minMatch
}

// scoreRescueCandidate scores a candidate for the title-only rescue pass. The
// gates are relaxed (a near-exact title can compensate for an imperfect artist
// string from playout), but a clearly different artist is still rejected so a
// cover by someone else (e.g. karaoke versions) never wins.
func scoreRescueCandidate(r iTunesResult, artist, title string) (float64, bool) {
	artistScore := artistSimilarity(r.ArtistName, artist)
	titleScore := similarity(r.TrackName, title)
	if artistScore < rescueArtistMin || titleScore < rescueTitleMin {
		return 0, false
	}
	base := artistScore*artistWeight + titleScore*titleWeight
	total := base * compilationFactor(r) * versionFactor(r.TrackName, title)
	return total, total >= minMatch
}

// bestCandidate returns the highest-scoring acceptable iTunes result for the
// given playout artist/title under the strict primary scoring. Ties are
// resolved by the finer-grained score the penalties produce, so the proper
// album cover wins over compilation/remix candidates of similar similarity.
func bestCandidate(results []iTunesResult, artist, title string) (iTunesResult, float64, bool) {
	var best iTunesResult
	bestScore := 0.0
	found := false
	for _, r := range results {
		score, ok := scoreCandidate(r, artist, title)
		slog.Debug("iTunes candidate",
			"artist", r.ArtistName, "track", r.TrackName,
			"album", r.CollectionName,
			"score", round3(score), "accepted", ok)
		if r.ArtworkURL100 == "" || !ok || score <= bestScore+1e-9 {
			continue
		}
		best, bestScore, found = r, score, true
	}
	return best, bestScore, found
}

// bestRescueCandidate is bestCandidate but for the relaxed title-only rescue
// scoring.
func bestRescueCandidate(results []iTunesResult, artist, title string) (iTunesResult, float64, bool) {
	var best iTunesResult
	bestScore := 0.0
	found := false
	for _, r := range results {
		score, ok := scoreRescueCandidate(r, artist, title)
		slog.Debug("iTunes rescue candidate",
			"artist", r.ArtistName, "track", r.TrackName,
			"album", r.CollectionName,
			"score", round3(score), "accepted", ok)
		if r.ArtworkURL100 == "" || !ok || score <= bestScore+1e-9 {
			continue
		}
		best, bestScore, found = r, score, true
	}
	return best, bestScore, found
}

// buildTitleQuery strips annotations from the title for the title-only rescue
// search (used when the playout artist string is mangled).
func buildTitleQuery(title string) string {
	q := reBracket.ReplaceAllString(title, " ")
	q = reParen.ReplaceAllString(q, " ")
	q = reSpace.ReplaceAllString(q, " ")
	return strings.TrimSpace(q)
}

// buildSearchQuery builds the iTunes search term from the formatted text,
// stripping parenthetical/bracket annotations (e.g. "(Remastered)", "[Edit]")
// that only degrade match quality.
func buildSearchQuery(artist, title, text string) string {
	q := strings.TrimSpace(text)
	if q == "" {
		q = strings.TrimSpace(strings.Join([]string{artist, title}, " "))
	}
	q = reBracket.ReplaceAllString(q, " ")
	q = reParen.ReplaceAllString(q, " ")
	q = reSpace.ReplaceAllString(q, " ")
	return strings.TrimSpace(q)
}

// searchITunes queries the iTunes Search API and returns the best matching
// artwork URL (upgraded to 600x600, forced https) or "".
//
// It runs at most two passes and stops early once a strong match is found, so
// the common case is still a single API call per track (respecting Apple's
// ~20 calls/minute limit together with COVERART_MIN_INTERVAL):
//  1. "artist - title" (entity=song) — the primary search with strict gates.
//  2. "<title>" only — a rescue for when the playout artist string is mangled
//     (typos, missing "feat." collaborators) but the title is intact.
func (r *Resolver) searchITunes(artist, title, query string) (string, error) {
	primary := strings.TrimSpace(query)
	if primary == "" {
		primary = strings.TrimSpace(strings.Join([]string{artist, title}, " "))
	}
	titleOnly := buildTitleQuery(title)

	bestURL := ""
	bestScore := 0.0

	// Pass 1: primary "artist - title" search with strict artist/title gates.
	results, err := r.iTunesSearch(primary)
	if err != nil {
		return "", err
	}
	if cand, score, ok := bestCandidate(results, artist, title); ok {
		bestURL, bestScore = cand.ArtworkURL100, score
	}
	if bestScore >= strongMatch {
		return upgradeArtworkURL(bestURL), nil
	}

	// Pass 2: title-only rescue scored with relaxed-but-safe gates.
	if titleOnly != "" && titleOnly != primary {
		results, err = r.iTunesSearch(titleOnly)
		if err != nil {
			return "", err
		}
		if cand, score, ok := bestRescueCandidate(results, artist, title); ok && score > bestScore+1e-9 {
			bestURL, bestScore = cand.ArtworkURL100, score
		}
	}

	if bestURL == "" {
		slog.Debug("No iTunes candidate above match threshold", "query", query, "best_score", round3(bestScore))
		return "", nil
	}
	return upgradeArtworkURL(bestURL), nil
}

// iTunesSearch performs one iTunes Search API call for term and returns the
// raw song results.
func (r *Resolver) iTunesSearch(term string) ([]iTunesResult, error) {
	searchURL := fmt.Sprintf(
		"https://itunes.apple.com/search?term=%s&country=%s&media=music&entity=song&limit=%d",
		url.QueryEscape(term), url.QueryEscape(r.cfg.ITunesCountry), r.cfg.ITunesLimit,
	)
	slog.Debug("iTunes API request", "url", searchURL)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", itunesUserAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iTunes returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("iTunes response read: %w", err)
	}

	var data iTunesResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("iTunes response parse: %w", err)
	}
	return data.Results, nil
}

// upgradeArtworkURL forces https and bumps artwork resolution to 600x600.
func upgradeArtworkURL(artworkURL string) string {
	artworkURL = strings.Replace(artworkURL, "100x100bb", "600x600bb", 1)
	artworkURL = forceHTTPS(artworkURL)
	return artworkURL
}

// forceHTTPS rewrites an http:// URL to https:// (except localhost).
func forceHTTPS(s string) string {
	if strings.HasPrefix(s, "http:") && !strings.HasPrefix(s, "http://localhost") {
		return "https:" + strings.TrimPrefix(s, "http:")
	}
	return s
}

func round3(f float64) float64 {
	return float64(int(f*1000)) / 1000
}

const itunesUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.5.2 Safari/605.1.15"
