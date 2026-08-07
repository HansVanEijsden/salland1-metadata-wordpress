package coverart

import (
	"os"
	"strconv"
	"time"
)

// Config holds cover-art resolver settings, all sourced from environment
// variables so each station can be configured without code changes.
type Config struct {
	Enabled         bool          // master switch; off by default
	HubURL          string        // base URL of the metadata hub, e.g. http://172.21.0.66:9000
	HubInput        string        // hub dynamic input that receives the resolved URL
	HubSecret       string        // secret of the hub dynamic input
	FallbackImage   string        // station placeholder when nothing resolves
	WPURL           string        // WordPress metadata API used for the show-avatar fallback
	ITunesCountry   string        // iTunes store country code
	ITunesLimit     int           // number of iTunes search results to score
	SpecialTrigger  string        // title/text prefix marking a "special song"; empty = disabled
	SpecialURL      string        // API returning the special song's artwork
	CacheTTL        time.Duration // how long a resolved track is remembered
	MinInterval     time.Duration // minimum spacing between iTunes API calls
	ErrorCooldown   time.Duration // skip iTunes after errors/rate-limits for this long
	MinMusicSeconds int           // tracks shorter than this are not treated as music
}

// LoadConfig reads cover-art settings from the environment. wpURL is the
// default for the WordPress metadata API (normally the service's SOURCE_URL).
func LoadConfig(wpURL string) *Config {
	return &Config{
		Enabled:         getEnvBool("COVERART_ENABLED", false),
		HubURL:          getEnv("COVERART_HUB_URL", ""),
		HubInput:        getEnv("COVERART_HUB_INPUT", "radio-cover-art"),
		HubSecret:       getEnv("COVERART_HUB_SECRET", ""),
		FallbackImage:   getEnv("COVERART_FALLBACK_IMAGE", ""),
		WPURL:           getEnv("COVERART_WP_URL", wpURL),
		ITunesCountry:   getEnv("COVERART_ITUNES_COUNTRY", "nl"),
		ITunesLimit:     getEnvInt("COVERART_ITUNES_LIMIT", 15),
		SpecialTrigger:  getEnv("COVERART_SPECIAL_TRIGGER", ""),
		SpecialURL:      getEnv("COVERART_SPECIAL_URL", ""),
		CacheTTL:        getEnvDuration("COVERART_CACHE_TTL", 6*time.Hour),
		MinInterval:     getEnvDuration("COVERART_MIN_INTERVAL", 3*time.Second),
		ErrorCooldown:   getEnvDuration("COVERART_ERROR_COOLDOWN", 5*time.Minute),
		MinMusicSeconds: getEnvInt("COVERART_MIN_MUSIC_SECONDS", 120),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
