package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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
	r := chi.NewRouter()

	r.Use(middleware.RequestID,
    middleware.Logger,
    middleware.Recoverer,)

	r.Get("/health", healthHandler)
	
	// HTTP Server
	addr := fmt.Sprintf(":%d", cfg.Port)

	server := &http.Server{
		Addr: addr,
		Handler: r,
	}

	zlog.Info().Str("addr", addr).Msg("HTTP server listening")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		applog.Fatal(err, "Server crashed")
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Set Content-Type, Set data, Encode to JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	respone := map[string]string {
		"status":"ok",
	}

	if err := json.NewEncoder(w).Encode(respone); err != nil {
		zlog.Error().Err(err).Msg("failed to encode health response")
	}
	zlog.Debug().Msg("Health check passed")
}