package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func TestPublishPipelineIntegrationWithMockConnector(t *testing.T) {
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping database integration test")
	}

	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		encryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("cannot connect to DB: %v; skipping", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		t.Skipf("database ping failed: %v; skipping", err)
	}

	// Ensure tables exist in fresh database (e.g. CI environments)
	var usersTableExists bool
	err = db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_name = 'users'
		)
	`).Scan(&usersTableExists)
	if err != nil {
		t.Skipf("cannot query database schema: %v; skipping", err)
	}

	if !usersTableExists {
		migrationSQL, readErr := os.ReadFile("../migrations/000001_init_schema.up.sql")
		if readErr != nil {
			migrationSQL, readErr = os.ReadFile("migrations/000001_init_schema.up.sql")
		}
		if readErr != nil {
			t.Fatalf("failed to read migrations file: %v", readErr)
		}

		if _, execErr := db.Exec(ctx, string(migrationSQL)); execErr != nil {
			t.Fatalf("failed to apply migration schema: %v", execErr)
		}
	}

	// 1. Create a test user
	testUserID := uuid.New()
	testEmail := "test_integration_" + testUserID.String()[:8] + "@example.com"
	_, err = db.Exec(ctx, `
		INSERT INTO users (id, email, display_name, password_hash)
		VALUES ($1, $2, 'Integration Tester', 'dummy_hash')
	`, testUserID, testEmail)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_, _ = db.Exec(ctx, `DELETE FROM users WHERE id = $1`, testUserID)
	}()

	// 2. Create a test social account for Twitter with encrypted access token
	encryptedToken, err := auth.EncryptToken("integration_oauth_token", encryptionKey)
	if err != nil {
		t.Fatalf("failed to encrypt test token: %v", err)
	}

	testSocialAccountID := uuid.New()
	_, err = db.Exec(ctx, `
		INSERT INTO social_accounts (id, user_id, platform, platform_user_id, access_token)
		VALUES ($1, $2, 'twitter', 'test_platform_user_123', $3)
	`, testSocialAccountID, testUserID, encryptedToken)
	if err != nil {
		t.Fatalf("failed to insert test social account: %v", err)
	}

	// 3. Create a test post
	testPostID := uuid.New()
	caption := "Integration Test Tweet"
	_, err = db.Exec(ctx, `
		INSERT INTO posts (id, user_id, caption, status)
		VALUES ($1, $2, $3, 'QUEUED')
	`, testPostID, testUserID, caption)
	if err != nil {
		t.Fatalf("failed to insert test post: %v", err)
	}

	// 4. Create post target
	testTargetID := uuid.New()
	_, err = db.Exec(ctx, `
		INSERT INTO post_targets (id, post_id, social_account_id, status)
		VALUES ($1, $2, $3, 'PENDING')
	`, testTargetID, testPostID, testSocialAccountID)
	if err != nil {
		t.Fatalf("failed to insert test post target: %v", err)
	}

	// 5. Create publication attempt
	testAttemptID := uuid.New()
	_, err = db.Exec(ctx, `
		INSERT INTO publication_attempts (id, post_target_id, status)
		VALUES ($1, $2, 'PENDING')
	`, testAttemptID, testTargetID)
	if err != nil {
		t.Fatalf("failed to insert test publication attempt: %v", err)
	}

	// 6. Instantiate repositories and worker processor with Mock Twitter connector
	postsRepo := storage.NewPostRepository(db)
	publicationsRepo := storage.NewPublicationRepository(db)
	mockTwitterConnector := twitter.NewMock()

	platformConnectors := map[string]connectors.PlatformConnector{
		"twitter": mockTwitterConnector,
	}

	processor := worker.NewPublishProcessor(postsRepo, publicationsRepo, platformConnectors, encryptionKey)

	// 7. Create Asynq PublishJob task and process it
	task, err := jobs.NewPublishTask(testPostID)
	if err != nil {
		t.Fatalf("failed to serialize publish job: %v", err)
	}

	err = processor.ProcessPublishJob(ctx, task)
	if err != nil {
		t.Fatalf("processor.ProcessPublishJob failed: %v", err)
	}

	// 8. Verify database results
	// Check post status
	var postStatus string
	err = db.QueryRow(ctx, `SELECT status FROM posts WHERE id = $1`, testPostID).Scan(&postStatus)
	if err != nil {
		t.Fatalf("failed to query post status: %v", err)
	}
	if postStatus != "PUBLISHED" {
		t.Errorf("expected post status 'PUBLISHED', got %q", postStatus)
	}

	// Check post target status
	var targetStatus string
	err = db.QueryRow(ctx, `SELECT status FROM post_targets WHERE id = $1`, testTargetID).Scan(&targetStatus)
	if err != nil {
		t.Fatalf("failed to query target status: %v", err)
	}
	if targetStatus != "PUBLISHED" {
		t.Errorf("expected target status 'PUBLISHED', got %q", targetStatus)
	}

	// Check publication attempt status, platform_post_id, and platform_url
	var attemptStatus, platformPostID, platformURL string
	err = db.QueryRow(ctx, `
		SELECT status, platform_post_id, platform_url
		FROM publication_attempts
		WHERE id = $1
	`, testAttemptID).Scan(&attemptStatus, &platformPostID, &platformURL)
	if err != nil {
		t.Fatalf("failed to query publication attempt: %v", err)
	}

	if attemptStatus != "SUCCEEDED" {
		t.Errorf("expected attempt status 'SUCCEEDED', got %q", attemptStatus)
	}

	if !strings.HasPrefix(platformPostID, "mock_x_") {
		t.Errorf("expected platformPostID to start with 'mock_x_', got %q", platformPostID)
	}

	expectedPrefix := "http://mock.x.local/status/mock_x_"
	if !strings.HasPrefix(platformURL, expectedPrefix) {
		t.Errorf("expected platformURL to start with %q, got %q", expectedPrefix, platformURL)
	}

	// 9. Verify GetPost returns published URLs for targets
	postService := service.NewPostService(postsRepo)
	postDetails, err := postService.GetPost(ctx, testUserID, testPostID)
	if err != nil {
		t.Fatalf("failed to get post details: %v", err)
	}

	if postDetails.Status != "PUBLISHED" {
		t.Errorf("expected postDetails.Status to be 'PUBLISHED', got %q", postDetails.Status)
	}

	if len(postDetails.Targets) != 1 {
		t.Fatalf("expected 1 target in postDetails, got %d", len(postDetails.Targets))
	}

	targetDetail := postDetails.Targets[0]
	if targetDetail.Platform != "twitter" {
		t.Errorf("expected platform 'twitter', got %q", targetDetail.Platform)
	}
	if targetDetail.PlatformURL == nil || !strings.HasPrefix(*targetDetail.PlatformURL, expectedPrefix) {
		t.Errorf("expected target platform URL with prefix %q, got %v", expectedPrefix, targetDetail.PlatformURL)
	}
}
