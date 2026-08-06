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

// iTunes results are scored with a weighted combination of artist and title
// similarity. These constants mirror the behaviour of the legacy PHP resolver.
const (
	artistWeight = 0.6
	titleWeight  = 0.4
	minMatch     = 0.6
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
)

// iTunesResult is the subset of the iTunes Search API response we consume.
type iTunesResult struct {
	ArtistName    string `json:"artistName"`
	TrackName     string `json:"trackName"`
	ArtworkURL100 string `json:"artworkUrl100"`
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

// searchITunes queries the iTunes Search API for the track and returns the best
// matching artwork URL (upgraded to 600x600, forced https) or "".
func (r *Resolver) searchITunes(artist, title, query string) (string, error) {
	searchURL := fmt.Sprintf(
		"https://itunes.apple.com/search?term=%s&country=%s&media=music&entity=song&limit=%d",
		url.QueryEscape(query), url.QueryEscape(r.cfg.ITunesCountry), r.cfg.ITunesLimit,
	)

	slog.Debug("iTunes API request", "url", searchURL, "artist", artist, "title", title)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", itunesUserAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iTunes returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var data iTunesResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("iTunes response parse: %w", err)
	}
	if data.ResultCount == 0 || len(data.Results) == 0 {
		slog.Debug("iTunes returned no results", "query", query)
		return "", nil
	}

	bestURL := ""
	bestScore := 0.0
	for _, result := range data.Results {
		artistScore := similarity(result.ArtistName, artist)
		titleScore := similarity(result.TrackName, title)
		total := artistScore*artistWeight + titleScore*titleWeight
		slog.Debug("iTunes candidate",
			"artist", result.ArtistName, "track", result.TrackName,
			"artist_score", round3(artistScore), "title_score", round3(titleScore), "total", round3(total))
		if total > bestScore && total >= minMatch {
			bestScore = total
			bestURL = result.ArtworkURL100
		}
	}

	if bestURL == "" {
		slog.Debug("No iTunes candidate above match threshold", "query", query, "best_score", round3(bestScore))
		return "", nil
	}

	return upgradeArtworkURL(bestURL), nil
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

// isUnicodeLetterOrDigit reports whether r is a letter or digit (mirrors the
// \p{L}\p{N} character class used in normalize).
const itunesUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.5.2 Safari/605.1.15"
