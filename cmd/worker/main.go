package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/jobs"
	applog "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load .env file in development (ignore if not present)
	_ = godotenv.Load()

	// Initialize logging
	applog.Init("debug")

	log.Info().Msg("Starting Blink worker")

	// Database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal().Msg("DATABASE_URL environment variable not set")
	}

	db, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open database")
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}

	log.Info().Msg("Connected to database")

	// Redis connection
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	redisURL = strings.TrimPrefix(redisURL, "redis://")
	redisURL = strings.TrimPrefix(redisURL, "rediss://")

	// Create Asynq client for testing connection
	testClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisURL})
	defer testClient.Close()

	if err := testClient.Ping(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}

	log.Info().Str("addr", redisURL).Msg("Connected to Redis")

	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		log.Fatal().Msg("ENCRYPTION_KEY environment variable not set")
	}

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

	platformConnectors := map[string]connectors.PlatformConnector{
		"twitter": twitterConnector,
	}

	// Create worker server
	srv, err := worker.NewServer(redisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create worker server")
	}

	log.Info().Msg("Worker server initialized")

	// Create and register handler
	mux := asynq.NewServeMux()
	processor := worker.NewPublishProcessor(postsRepo, publicationsRepo, platformConnectors, encryptionKey)

	mux.HandleFunc(jobs.TypePublishPost, processor.ProcessPublishJob)

	log.Info().Msg("Job handlers registered")

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)

	go func() {
		log.Info().Msg("Worker started")
		if err := srv.Start(mux); err != nil {
			errCh <- fmt.Errorf("worker error: %w", err)
		}
	}()

	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		srv.Shutdown()
		log.Info().Msg("Worker shut down gracefully")
	case err := <-errCh:
		log.Fatal().Err(err).Msg("Worker error")
	}
}
