package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/iamvalson/blink/internal/config"
	applog "github.com/iamvalson/blink/internal/log"

	zlog "github.com/rs/zerolog/log"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging
	applog.Init(cfg.LogLevel)

	zlog.Info().Str("env", cfg.Env).Int("port", cfg.Port).Msg("Starting API server")

	// Router Setup
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	zlog.Info().Str("addr", addr).Msg("HTTP server listening")

	if err := http.ListenAndServe(addr, mux); err != nil {
		applog.Fatal(err, "Server crashed")
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Set Content-Type, Set data, Encode to JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		zlog.Error().Err(err).Msg("failed to write health response")
	}
	zlog.Debug().Msg("Health check passed")
}