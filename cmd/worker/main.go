package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/connectors/youtube"
	"github.com/iamvalson/blink/internal/jobs"
	applog "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Error().Err(err).Msg("Worker stopped with error")
	}
}

func run(ctx context.Context) error {
	// Load .env file in development (ignore if not present)
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Initialize logging
	applog.Init("debug")

	log.Info().Msg("Starting Blink worker")

	// Database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}

	db, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		log.Info().Str("event", "database_shutdown_started").Msg("Closing PostgreSQL")
		db.Close()
		log.Info().Str("event", "database_shutdown_completed").Msg("PostgreSQL closed")
	}()

	if err := db.Ping(context.Background()); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	log.Info().Msg("Connected to database")

	// Redis connection
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisURL = strings.TrimPrefix(redisURL, "redis://")
	redisURL = strings.TrimPrefix(redisURL, "rediss://")

	// The worker uses the same client for explicit, durable business retries.
	retryClient, err := jobs.NewClient(redisURL)
	if err != nil {
		return fmt.Errorf("connect to Redis: %w", err)
	}
	defer func() {
		log.Info().Str("event", "redis_shutdown_started").Msg("Closing Redis")
		if err := retryClient.Close(); err != nil {
			log.Error().Err(err).Str("component", "redis").Msg("Redis shutdown failed")
		}
		log.Info().Str("event", "redis_shutdown_completed").Msg("Redis closed")
	}()

	log.Info().Str("addr", redisURL).Msg("Connected to Redis")

	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return fmt.Errorf("ENCRYPTION_KEY environment variable not set")
	}
	shutdownTimeout := cfg.ShutdownTimeout

	// Create repositories
	postsRepo := storage.NewPostRepository(db)
	publicationsRepo := storage.NewPublicationRepository(db)

	// Create platform connectors
	var twitterConnector connectors.PlatformConnector
	platformMode := strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_MODE")))

	if platformMode == "mock" {
		log.Warn().Msg("PLATFORM_MODE is set to 'mock': using mock Twitter/X connector")
		twitterConnector = twitter.NewMock()
	} else {
		log.Info().Msg("PLATFORM_MODE is set to 'real' (or default): using real Twitter/X connector")
		twitterCfg := twitter.TwitterConfig{
			ClientID:     os.Getenv("TWITTER_CLIENT_ID"),
			ClientSecret: os.Getenv("TWITTER_CLIENT_SECRET"),
			CallbackURL:  os.Getenv("TWITTER_CALLBACK_URL"),
		}
		twitterConnector = twitter.New(twitterCfg)
	}

	youtubeCfg := youtube.YouTubeConfig{
		ClientID:     os.Getenv("YOUTUBE_CLIENT_ID"),
		ClientSecret: os.Getenv("YOUTUBE_CLIENT_SECRET"),
		CallbackURL:  os.Getenv("YOUTUBE_CALLBACK_URL"),
	}
	youtubeConnector := youtube.New(youtubeCfg)

	// To add a new publish platform, add it to this map.
	// No other file outside internal/connectors/<platform>/ needs to change.
	platformConnectors := map[string]connectors.PlatformConnector{
		connectors.PlatformTwitter: twitterConnector,
		connectors.PlatformYoutube: youtubeConnector,
	}

	// Create worker server
	srv := worker.NewServerWithShutdownTimeout(redisURL, shutdownTimeout)

	log.Info().Msg("Worker server initialized")

	// Initialize media storage & repository
	localMediaStore, err := mediastore.NewLocalStore(cfg.MediaStoragePath)
	if err != nil {
		return fmt.Errorf("initialize media store: %w", err)
	}
	mediaRepo := storage.NewMediaRepository(db)

	// Create and register handler
	mux := asynq.NewServeMux()
	processor := worker.NewPublishProcessor(postsRepo, publicationsRepo, platformConnectors, encryptionKey, retryClient).
		WithMediaStore(mediaRepo, localMediaStore)

	mux.HandleFunc(jobs.TypePublishPost, processor.ProcessPublishJob)

	log.Info().Msg("Job handlers registered")

	errCh := make(chan error, 1)

	go func() {
		log.Info().Msg("Worker started")
		if err := srv.Start(mux); err != nil {
			errCh <- fmt.Errorf("worker error: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info().Str("event", "shutdown_started").Str("signal", "received").Msg("Starting graceful worker shutdown")
		shutdownStarted := time.Now()
		srv.Shutdown()
		if time.Since(shutdownStarted) >= shutdownTimeout {
			log.Warn().Str("event", "shutdown_timeout").Str("component", "worker").Dur("duration", time.Since(shutdownStarted)).Msg("Worker shutdown reached its deadline")
		}
		if err := <-errCh; err != nil {
			return err
		}
		log.Info().Str("event", "worker_shutdown_completed").Msg("Worker shut down gracefully")
		log.Info().Str("event", "shutdown_completed").Msg("Worker shutdown complete")
	case err := <-errCh:
		return err
	}
	return nil
}
