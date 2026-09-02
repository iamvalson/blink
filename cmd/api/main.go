package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/iamvalson/blink/internal/api"
	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/connectors/twitter"
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


	// Initialize Twitter connector
	twitterCfg := twitter.TwitterConfig{
		ClientID:			os.Getenv("TWITTER_CLIENT_ID"),
		ClientSecret:		os.Getenv("TWITTER_CLIENT_SECRET"),
		CallbackURL:		os.Getenv("TWITTER_CALLBACK_URL"),
	}
	twitterConnector := twitter.New(twitterCfg)

	// Router Setup
	router := api.NewRouter(twitterConnector)
	
	// HTTP Server
	addr := fmt.Sprintf(":%d", cfg.Port)

	server := &http.Server{
		Addr: addr,
		Handler: router,
	}

	zlog.Info().Str("addr", addr).Msg("HTTP server listening")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		applog.Fatal(err, "Server crashed")
	}
}