package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/iamvalson/blink/internal/api"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/connectors/youtube"
	"github.com/iamvalson/blink/internal/jobs"
	applog "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/outbox"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"

	zlog "github.com/rs/zerolog/log"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		zlog.Error().Err(err).Msg("API server stopped with error")
	}
}

func run(ctx context.Context) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
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

	// Initialize YouTube connector
	youtubeCfg := youtube.YouTubeConfig{
		ClientID:     os.Getenv("YOUTUBE_CLIENT_ID"),
		ClientSecret: os.Getenv("YOUTUBE_CLIENT_SECRET"),
		CallbackURL:  os.Getenv("YOUTUBE_CALLBACK_URL"),
	}
	youtubeConnector := youtube.New(youtubeCfg)

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		zlog.Info().Str("event", "database_shutdown_started").Msg("Closing PostgreSQL")
		db.Close()
		zlog.Info().Str("event", "database_shutdown_completed").Msg("PostgreSQL closed")
	}()
	if err := db.Ping(context.Background()); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	accounts := storage.NewSocialAccountRepository(db)
	users := storage.NewUserRepository(db)
	posts := storage.NewPostRepository(db)
	outboxRepo := storage.NewOutboxRepository(db)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate JWT keys: %w", err)
	}
	jwtService := auth.NewJWTService(privateKey, publicKey)
	signupService := service.NewSignupService(users, jwtService)
	loginService := service.NewLoginService(users, jwtService)
	meService := service.NewMeService(users)

	// Initialize Redis for Asynq job queue
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	jobsClient, err := jobs.NewClient(redisURL)
	if err != nil {
		return fmt.Errorf("create Asynq client: %w", err)
	}
	defer func() {
		zlog.Info().Str("event", "redis_shutdown_started").Msg("Closing Redis")
		if err := jobsClient.Close(); err != nil {
			zlog.Error().Err(err).Str("component", "redis").Msg("Redis shutdown failed")
		}
		zlog.Info().Str("event", "redis_shutdown_completed").Msg("Redis closed")
	}()

	// Initialize outbox dispatcher
	dispatcher := outbox.NewDispatcher(outboxRepo, jobsClient)

	// Start outbox dispatcher in background (runs every 5 seconds)
	var dispatcherWG sync.WaitGroup
	dispatcherCtx, stopDispatcher := context.WithCancel(ctx)
	defer func() {
		stopDispatcher()
		dispatcherWG.Wait()
	}()
	dispatcherWG.Add(1)
	go func() {
		defer dispatcherWG.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-dispatcherCtx.Done():
				return
			case <-ticker.C:
				operationCtx, cancel := context.WithTimeout(dispatcherCtx, 10*time.Second)
				if err := dispatcher.ProcessPendingEvents(operationCtx); err != nil {
					zlog.Error().Err(err).Msg("Failed to process pending events")
				}
				cancel()
			}
		}
	}()

	zlog.Info().Msg("Outbox dispatcher started")

	// Media Storage & Service
	localMediaStore, err := mediastore.NewLocalStore(cfg.MediaStoragePath)
	if err != nil {
		return fmt.Errorf("initialize media store: %w", err)
	}
	mediaRepo := storage.NewMediaRepository(db)
	mediaService := service.NewMediaService(mediaRepo, localMediaStore)
	mediaHandler := handler.NewMediaHandler(mediaService)

	// Router Setup
	//
	// To add a new OAuth platform, append its connector to this slice.
	// No other file outside internal/connectors/<platform>/ needs to change.
	oauthConnectors := []connectors.OAuthConnector{twitterConnector, youtubeConnector}
	router := api.NewRouter(oauthConnectors, accounts, posts, cfg.EncryptionKey, cfg.FrontendURL, signupService, loginService, meService, jwtService, cfg.CORSOrigins, cfg.SecureCookie, cfg.CookieSecure, cfg.CookieSameSite, mediaHandler)

	// HTTP Server
	addr := fmt.Sprintf(":%d", cfg.Port)

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		zlog.Info().Str("addr", addr).Msg("HTTP server listening")
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server crashed: %w", err)
		}
	case <-ctx.Done():
		zlog.Info().Str("event", "shutdown_started").Str("signal", "received").Msg("Starting graceful shutdown")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		zlog.Info().Str("event", "http_shutdown_started").Msg("Stopping HTTP server")
		if err := server.Shutdown(shutdownCtx); err != nil {
			zlog.Error().Err(err).Str("component", "http").Str("event", "shutdown_timeout").Msg("HTTP shutdown did not complete")
		} else {
			zlog.Info().Str("event", "http_shutdown_completed").Msg("HTTP server stopped")
		}
		stopDispatcher()
		zlog.Info().Str("event", "shutdown_completed").Msg("API shutdown complete")
	}
	return nil
}
