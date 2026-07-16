package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"salland1-metadata-wordpress/internal/cache"
	"salland1-metadata-wordpress/internal/config"
	"salland1-metadata-wordpress/internal/fetcher"
	"salland1-metadata-wordpress/internal/handlers"
	"salland1-metadata-wordpress/internal/logger"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	logger.Init(cfg.LogLevel)
	slog.Info("Starting Salland1 Metadata WordPress service",
		"version", "1.0.0",
		"source_url", cfg.SourceURL,
		"fetch_interval", cfg.FetchInterval,
		"jitter", cfg.Jitter)

	// Initialize cache
	cache := cache.New()

	// Initialize fetcher
	fetcher := fetcher.New(cfg.SourceURL, cfg.FetchInterval, cfg.Jitter, cache)

	// Start fetcher in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go fetcher.Start(ctx)

	// Initialize HTTP handlers
	handlers := handlers.New(cache, fetcher)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/radio-fm-pty", handlers.RadioFmPty)
	mux.HandleFunc("/radio-fm-ptyn", handlers.RadioFmPtyn)
	mux.HandleFunc("/radio-fm-programme", handlers.RadioFmProgramme)
	mux.HandleFunc("/radio-stream-programme", handlers.RadioStreamProgramme)
	mux.HandleFunc("/radio-dab-programme", handlers.RadioDabProgramme)
	mux.HandleFunc("/radio-tv-programme", handlers.RadioTvProgramme)
	mux.HandleFunc("/radio-tv-host", handlers.RadioTvHost)
	mux.HandleFunc("/health", handlers.Health)

	// Add logging middleware
	loggedMux := logger.HTTPLogger(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      loggedMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server
	go func() {
		slog.Info("HTTP server started", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
}
