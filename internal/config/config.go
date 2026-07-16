package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          int
	SourceURL     string
	FetchInterval time.Duration
	Jitter        time.Duration
	LogLevel      string
}

func Load() *Config {
	port := getEnvInt("PORT", 8080)
	sourceURL := getEnv("SOURCE_URL", "https://www.salland1.nl/wp-json/metadata/v1/current")
	fetchInterval := getEnvDuration("FETCH_INTERVAL", 60*time.Second)
	jitter := getEnvDuration("JITTER", 10*time.Second)
	logLevel := getEnv("LOG_LEVEL", "info")

	return &Config{
		Port:          port,
		SourceURL:     sourceURL,
		FetchInterval: fetchInterval,
		Jitter:        jitter,
		LogLevel:      logLevel,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
