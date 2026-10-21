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
	"github.com/iamvalson/blink/internal/outbox"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
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
	ctx := context.Background()
	db := testDatabase(t)
	encryptionKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var err error

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
	task, err := jobs.NewPublishTask(testPostID, 1)
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
	ctx := context.Background()
	db := testDatabase(t)
	redisURL := os.Getenv("BLINK_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set BLINK_TEST_REDIS_URL to run Redis delivery integration tests")
	}

	redisAddr := strings.TrimPrefix(strings.TrimPrefix(redisURL, "redis://"), "rediss://")
	jobsClient, err := jobs.NewClient(redisAddr)
	if err != nil {
		t.Fatalf("cannot connect to test Redis: %v", err)
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

	var taskIDs []string
	defer func() {
		for _, id := range taskIDs {
			_ = inspector.DeleteTask("default", id)
		}
	}()

	processor := worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{
		connectors.PlatformTwitter: fakePlatform,
	}, encryptionKey, jobsClient)
	task, err := jobs.NewPublishTask(testPostID, 1)
	if err != nil {
		t.Fatalf("create publish task: %v", err)
	}

	dispatcher := outbox.NewDispatcher(storage.NewOutboxRepository(db), jobsClient)
	for attempt := 1; attempt <= 3; attempt++ {
		if err := processor.ProcessPublishJob(ctx, task); err != nil {
			t.Fatal(err)
		}
		if attempt == 3 {
			break
		}
		assertRetryState(t, ctx, db, testAttemptID, "RETRYING", attempt)
		var eventID uuid.UUID
		var availableAt time.Time
		if err := db.QueryRow(ctx, `SELECT id, available_at FROM outbox_events
			WHERE aggregate_id = $1 AND status = 'PENDING'`, testPostID).Scan(&eventID, &availableAt); err != nil {
			t.Fatal(err)
		}
		delay := time.Minute
		if attempt == 2 {
			delay = 5 * time.Minute
		}
		assertDelay(t, availableAt, delay)
		// Early duplicate deliveries must not bypass the database retry time.
		if err := processor.ProcessPublishJob(ctx, task); err != nil {
			t.Fatal(err)
		}
		assertRetryState(t, ctx, db, testAttemptID, "RETRYING", attempt)
		if err := dispatcher.ProcessPendingEvents(ctx); err != nil {
			t.Fatal(err)
		}
		var pending string
		if err := db.QueryRow(ctx, `SELECT status FROM outbox_events WHERE id=$1`, eventID).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if pending != "PENDING" {
			t.Fatalf("future event dispatched: %s", pending)
		}
		// Advance only this test's retry, rather than sleeping for minutes.
		if _, err := db.Exec(ctx, `UPDATE publication_attempts SET next_retry_at=NOW()-INTERVAL '1 second' WHERE id=$1`, testAttemptID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `UPDATE outbox_events SET available_at=NOW()-INTERVAL '1 second' WHERE id=$1`, eventID); err != nil {
			t.Fatal(err)
		}
		if err := dispatcher.ProcessPendingEvents(ctx); err != nil {
			t.Fatal(err)
		}
		id := "outbox:" + eventID.String()
		taskIDs = append(taskIDs, id)
		queued, err := inspector.GetTaskInfo("default", id)
		if err != nil {
			t.Fatal(err)
		}
		job, err := jobs.ParsePublishJob(queued.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if job.TargetID == nil || *job.TargetID != testTargetID || job.SchedulingGeneration != 1 {
			t.Fatalf("retry must address only its target and generation: %+v", job)
		}
		task = asynq.NewTask(queued.Type, queued.Payload)
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

func assertDelay(t *testing.T, scheduledAt time.Time, expected time.Duration) {
	t.Helper()
	delta := time.Until(scheduledAt)
	if delta < expected-5*time.Second || delta > expected+5*time.Second {
		t.Fatalf("scheduled delay = %s, want approximately %s", delta, expected)
	}
}
