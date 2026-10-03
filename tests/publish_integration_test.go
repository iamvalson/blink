package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
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

type retryFakePlatform struct {
	mu         sync.Mutex
	serverURL  string
	calls      int
	attemptIDs []string
}

func (f *retryFakePlatform) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls++
	f.attemptIDs = append(f.attemptIDs, r.Header.Get("X-Attempt-ID"))
	call := f.calls
	f.mu.Unlock()
	if call < 3 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"fake-post-3","url":"https://fake.platform/posts/fake-post-3"}`))
}

func (f *retryFakePlatform) Authenticate(context.Context, connectors.AuthParams) (connectors.AuthResult, error) {
	return connectors.AuthResult{PlatformUserID: "fake-user"}, nil
}

func (f *retryFakePlatform) UploadMedia(context.Context, string, string, io.Reader, string) (string, error) {
	return "", nil
}

func (f *retryFakePlatform) Publish(ctx context.Context, token, attemptID, caption string, mediaIDs ...string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.serverURL, strings.NewReader(caption))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("X-Attempt-ID", attemptID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", connectors.HTTPError(resp.StatusCode, fmt.Sprintf("fake platform returned %d", resp.StatusCode))
	}
	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	return result.URL, result.ID, nil
}

func (f *retryFakePlatform) GetStatus(context.Context, string) (string, string, error) {
	return "published", "https://fake.platform/posts/fake-post-3", nil
}

func (f *retryFakePlatform) ReconcilePublish(context.Context, string, string, string, string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}

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

func TestPublishRetryIntegrationWithPostgresRedis(t *testing.T) {
	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("DATABASE_URL and REDIS_URL are required for the retry integration test")
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
	ensureRetryMigration(t, ctx, db)

	redisAddr := strings.TrimPrefix(strings.TrimPrefix(redisURL, "redis://"), "rediss://")
	jobsClient, err := jobs.NewClient(redisAddr)
	if err != nil {
		t.Skipf("cannot connect to Redis: %v; skipping", err)
	}
	defer jobsClient.Close()
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
	defer inspector.Close()

	fakePlatform := &retryFakePlatform{}
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fakePlatform.handler(w, r)
	}))
	defer platformServer.Close()
	fakePlatform.serverURL = platformServer.URL

	encryptionKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testUserID := uuid.New()
	testPostID := uuid.New()
	testTargetID := uuid.New()
	testAttemptID := uuid.New()
	testEmail := "retry_integration_" + testUserID.String()[:8] + "@example.com"
	_, err = db.Exec(ctx, `INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, 'Retry Tester', 'dummy_hash')`, testUserID, testEmail)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer func() { _, _ = db.Exec(ctx, `DELETE FROM users WHERE id = $1`, testUserID) }()
	encryptedToken, err := auth.EncryptToken("retry-token", encryptionKey)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	accountID := uuid.New()
	_, err = db.Exec(ctx, `INSERT INTO social_accounts (id, user_id, platform, platform_user_id, access_token) VALUES ($1, $2, 'twitter', 'fake-user', $3)`, accountID, testUserID, encryptedToken)
	if err != nil {
		t.Fatalf("insert social account: %v", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO posts (id, user_id, caption, status) VALUES ($1, $2, 'retry integration', 'QUEUED')`, testPostID, testUserID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO post_targets (id, post_id, social_account_id, status) VALUES ($1, $2, $3, 'PENDING')`, testTargetID, testPostID, accountID)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO publication_attempts (id, post_target_id, status) VALUES ($1, $2, 'PENDING')`, testAttemptID, testTargetID)
	if err != nil {
		t.Fatalf("insert publication attempt: %v", err)
	}

	defer func() {
		if scheduled, listErr := inspector.ListScheduledTasks("default"); listErr == nil {
			for _, task := range scheduled {
				_ = inspector.DeleteTask("default", task.ID)
			}
		}
	}()

	processor := worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{
		connectors.PlatformTwitter: fakePlatform,
	}, encryptionKey, jobsClient)
	task, err := jobs.NewPublishTask(testPostID)
	if err != nil {
		t.Fatalf("create publish task: %v", err)
	}

	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatalf("first publish execution: %v", err)
	}
	assertRetryState(t, ctx, db, testAttemptID, "RETRYING", 1)
	firstScheduled := waitForScheduledTask(t, inspector, testPostID)
	assertDelay(t, firstScheduled.NextProcessAt, time.Minute)
	_ = inspector.DeleteTask("default", firstScheduled.ID)

	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatalf("second publish execution: %v", err)
	}
	assertRetryState(t, ctx, db, testAttemptID, "RETRYING", 2)
	secondScheduled := waitForScheduledTask(t, inspector, testPostID)
	assertDelay(t, secondScheduled.NextProcessAt, 5*time.Minute)
	_ = inspector.DeleteTask("default", secondScheduled.ID)

	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatalf("third publish execution: %v", err)
	}
	var status string
	var attemptCount int
	var platformPostID, platformURL string
	err = db.QueryRow(ctx, `SELECT status, attempt_count, platform_post_id, platform_url FROM publication_attempts WHERE id = $1`, testAttemptID).Scan(&status, &attemptCount, &platformPostID, &platformURL)
	if err != nil {
		t.Fatalf("query final attempt: %v", err)
	}
	if status != "SUCCEEDED" || attemptCount != 3 || platformPostID != "fake-post-3" || platformURL != "https://fake.platform/posts/fake-post-3" {
		t.Fatalf("unexpected final attempt: status=%s count=%d id=%s url=%s", status, attemptCount, platformPostID, platformURL)
	}
	fakePlatform.mu.Lock()
	defer fakePlatform.mu.Unlock()
	if fakePlatform.calls != 3 || len(fakePlatform.attemptIDs) != 3 {
		t.Fatalf("expected exactly 3 platform deliveries, calls=%d ids=%d", fakePlatform.calls, len(fakePlatform.attemptIDs))
	}
	for _, attemptID := range fakePlatform.attemptIDs {
		if attemptID != testAttemptID.String() {
			t.Fatalf("platform received attempt ID %q, want %q", attemptID, testAttemptID)
		}
	}
}

func ensureRetryMigration(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	var hasUsersTable bool
	if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')`).Scan(&hasUsersTable); err != nil {
		t.Fatalf("check base schema: %v", err)
	}
	if !hasUsersTable {
		data, err := os.ReadFile("../migrations/000001_init_schema.up.sql")
		if err != nil {
			t.Fatalf("read base migration: %v", err)
		}
		if _, err := db.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply base migration: %v", err)
		}
	}

	var hasRetryColumn bool
	if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'publication_attempts' AND column_name = 'next_retry_at')`).Scan(&hasRetryColumn); err != nil {
		t.Fatalf("check retry migration: %v", err)
	}
	if hasRetryColumn {
		return
	}
	for _, migration := range []string{"../migrations/000002_publication_reliability.up.sql", "../migrations/000003_publication_retry.up.sql"} {
		data, err := os.ReadFile(migration)
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if _, err := db.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}
}

func assertRetryState(t *testing.T, ctx context.Context, db *pgxpool.Pool, attemptID uuid.UUID, wantStatus string, wantCount int) {
	t.Helper()
	var status string
	var count int
	if err := db.QueryRow(ctx, `SELECT status, attempt_count FROM publication_attempts WHERE id = $1`, attemptID).Scan(&status, &count); err != nil {
		t.Fatalf("query retry state: %v", err)
	}
	if status != wantStatus || count != wantCount {
		t.Fatalf("retry state = %s/%d, want %s/%d", status, count, wantStatus, wantCount)
	}
}

func waitForScheduledTask(t *testing.T, inspector *asynq.Inspector, postID uuid.UUID) *asynq.TaskInfo {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		tasks, err := inspector.ListScheduledTasks("default")
		if err == nil {
			for _, task := range tasks {
				job, parseErr := jobs.ParsePublishJob(task.Payload)
				if parseErr == nil && job.PostID == postID {
					return task
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for scheduled task for post %s", postID)
	return nil
}

func assertDelay(t *testing.T, scheduledAt time.Time, expected time.Duration) {
	t.Helper()
	delta := time.Until(scheduledAt)
	if delta < expected-5*time.Second || delta > expected+5*time.Second {
		t.Fatalf("scheduled delay = %s, want approximately %s", delta, expected)
	}
}
