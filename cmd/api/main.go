package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/iamvalson/blink/internal/api"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/jobs"
	applog "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/outbox"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"

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
		ClientID:     os.Getenv("TWITTER_CLIENT_ID"),
		ClientSecret: os.Getenv("TWITTER_CLIENT_SECRET"),
		CallbackURL:  os.Getenv("TWITTER_CALLBACK_URL"),
	}
	twitterConnector := twitter.New(twitterCfg)

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		applog.Fatal(err, "Failed to open database")
	}
	defer db.Close()
	if err := db.Ping(context.Background()); err != nil {
    applog.Fatal(err, "Failed to connect to database")
}
	accounts := storage.NewSocialAccountRepository(db)
	users := storage.NewUserRepository(db)
	posts := storage.NewPostRepository(db)
	outboxRepo := storage.NewOutboxRepository(db)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		applog.Fatal(err, "Failed to generate JWT keys")
	}
	jwtService := auth.NewJWTService(privateKey, publicKey)
	signupService := service.NewSignupService(users, jwtService)
	loginService := service.NewLoginService(users, jwtService)

	// Initialize Redis for Asynq job queue
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	jobsClient, err := jobs.NewClient(redisURL)
	if err != nil {
		applog.Fatal(err, "Failed to create Asynq client")
	}
	defer jobsClient.Close()

	// Initialize outbox dispatcher
	dispatcher := outbox.NewDispatcher(outboxRepo, jobsClient)

	// Start outbox dispatcher in background (runs every 5 seconds)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := dispatcher.ProcessPendingEvents(ctx); err != nil {
				zlog.Error().Err(err).Msg("Failed to process pending events")
			}
			cancel()
		}
	}()

	zlog.Info().Msg("Outbox dispatcher started")

	// Router Setup
	router := api.NewRouter(twitterConnector, accounts, posts, cfg.EncryptionKey, signupService, loginService, jwtService)

	// HTTP Server
	addr := fmt.Sprintf(":%d", cfg.Port)

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	zlog.Info().Str("addr", addr).Msg("HTTP server listening")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		applog.Fatal(err, "Server crashed")
	}
}
