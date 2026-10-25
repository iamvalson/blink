package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/model"
)

type recoveryStore struct {
	post    *model.Post
	target  model.PostTarget
	account model.SocialAccount
	attempt model.PublicationAttempt
}

func (s *recoveryStore) GetPostForPublishing(context.Context, uuid.UUID) (*model.Post, error) {
	return s.post, nil
}

func (s *recoveryStore) GetPendingPostTargets(context.Context, uuid.UUID) ([]model.PostTarget, error) {
	if s.target.Status == "PUBLISHED" {
		return nil, nil
	}
	return []model.PostTarget{s.target}, nil
}

func (s *recoveryStore) GetPublicationAttempt(context.Context, uuid.UUID) (*model.PublicationAttempt, error) {
	return &s.attempt, nil
}

func (s *recoveryStore) GetPostTargetWithSocialAccount(context.Context, uuid.UUID) (*model.PostTarget, *model.SocialAccount, error) {
	return &s.target, &s.account, nil
}

func (s *recoveryStore) MarkAttemptProcessing(context.Context, uuid.UUID) error {
	if s.attempt.Status != "PENDING" && s.attempt.Status != "UNKNOWN" && s.attempt.Status != "RETRYING" {
		return errors.New("attempt is not claimable")
	}
	s.attempt.Status = "PROCESSING"
	s.attempt.AttemptCount++
	return nil
}

func (s *recoveryStore) MarkAttemptUnknown(_ context.Context, _ uuid.UUID, code string, message string) error {
	s.attempt.Status = "UNKNOWN"
	s.attempt.ErrorCode = &code
	s.attempt.ErrorMessage = &message
	return nil
}

func (s *recoveryStore) ResetAttemptForRetry(context.Context, uuid.UUID) error {
	s.attempt.Status = "PENDING"
	return nil
}

func (s *recoveryStore) MarkAttemptSucceeded(_ context.Context, _ uuid.UUID, platformPostID string, platformURL string) error {
	s.attempt.Status = "SUCCEEDED"
	s.attempt.PlatformPostID = &platformPostID
	s.attempt.PlatformURL = &platformURL
	return nil
}

func (s *recoveryStore) MarkAttemptFailed(context.Context, uuid.UUID, string, string) error {
	s.attempt.Status = "FAILED"
	return nil
}

func (s *recoveryStore) MarkAttemptRetrying(_ context.Context, _ uuid.UUID, code, class, message string, nextRetryAt time.Time) error {
	s.attempt.Status = "RETRYING"
	s.attempt.ErrorCode = &code
	s.attempt.ErrorClass = &class
	s.attempt.ErrorMessage = &message
	s.attempt.NextRetryAt = &nextRetryAt
	return nil
}

func (s *recoveryStore) MarkPostTargetPublished(context.Context, uuid.UUID) error {
	s.target.Status = "PUBLISHED"
	return nil
}

func (s *recoveryStore) UpdatePostStatus(_ context.Context, _ uuid.UUID, status string) error {
	s.post.Status = status
	return nil
}

func (s *recoveryStore) RecordPermanentFailure(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, _ *string, _ string, _ string, _ int, _ string, _ string, _ string, _ string, _ []byte) error {
	s.attempt.Status = "FAILED"
	s.target.Status = "FAILED"
	return nil
}

func (s *recoveryStore) UpdateOAuthTokens(_ context.Context, _ uuid.UUID, accessToken string, expiresAt *time.Time, refreshToken *string) error {
	s.account.AccessToken = accessToken
	s.account.ExpiresAt = expiresAt
	if refreshToken != nil {
		s.account.RefreshToken = refreshToken
	}
	return nil
}

type crashRecoveryConnector struct {
	publishCalls   int
	reconcileCalls int
}

func (c *crashRecoveryConnector) Authenticate(context.Context, connectors.AuthParams) (connectors.AuthResult, error) {
	return connectors.AuthResult{}, nil
}

func (c *crashRecoveryConnector) UploadMedia(context.Context, string, string, io.Reader, string) (string, error) {
	return "", nil
}

func (c *crashRecoveryConnector) Publish(context.Context, string, string, string, ...string) (string, string, error) {
	c.publishCalls++
	return "", "", errors.New("connection lost after platform acceptance")
}

func (c *crashRecoveryConnector) GetStatus(context.Context, string) (string, string, error) {
	return "published", "https://example.test/posts/123", nil
}

func (c *crashRecoveryConnector) ReconcilePublish(context.Context, string, string, string, string) (connectors.ReconciliationResult, error) {
	c.reconcileCalls++
	return connectors.ReconciliationResult{
		Outcome:        connectors.ReconciliationFound,
		PlatformPostID: "platform-123",
		PublicURL:      "https://example.test/posts/123",
	}, nil
}

func TestPublishToTargetReconcilesAmbiguousResultWithoutRepublishing(t *testing.T) {
	postID := uuid.New()
	targetID := uuid.New()
	attemptID := uuid.New()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptedToken, err := auth.EncryptToken("access-token", key)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	store := &recoveryStore{
		post:    &model.Post{ID: postID, Caption: stringPtr("hello"), Status: "QUEUED", SchedulingGeneration: 1},
		target:  model.PostTarget{ID: targetID, PostID: postID, Status: "PENDING"},
		account: model.SocialAccount{Platform: connectors.PlatformTwitter, PlatformUserID: "user-123", AccessToken: encryptedToken},
		attempt: model.PublicationAttempt{ID: attemptID, PostTargetID: targetID, Status: "PENDING", CreatedAt: time.Now()},
	}
	connector := &crashRecoveryConnector{}
	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{
		connectors.PlatformTwitter: connector,
	}, key)

	if err := processor.publishToTarget(context.Background(), store.post, &store.target); err == nil {
		t.Fatal("expected ambiguous publish error")
	}
	if store.attempt.Status != "UNKNOWN" {
		t.Fatalf("expected UNKNOWN attempt after ambiguous error, got %s", store.attempt.Status)
	}

	if err := processor.publishToTarget(context.Background(), store.post, &store.target); err != nil {
		t.Fatalf("recovery publish failed: %v", err)
	}
	if connector.publishCalls != 1 {
		t.Fatalf("expected exactly one publish call, got %d", connector.publishCalls)
	}
	if connector.reconcileCalls != 1 {
		t.Fatalf("expected one reconciliation call, got %d", connector.reconcileCalls)
	}
	if store.attempt.Status != "SUCCEEDED" {
		t.Fatalf("expected SUCCEEDED attempt, got %s", store.attempt.Status)
	}
	if store.target.Status != "PUBLISHED" {
		t.Fatalf("expected PUBLISHED target, got %s", store.target.Status)
	}
}

type crashFileState struct {
	Status         string `json:"status"`
	AttemptCount   int    `json:"attempt_count"`
	PlatformPostID string `json:"platform_post_id"`
	PlatformURL    string `json:"platform_url"`
	TargetStatus   string `json:"target_status"`
}

type crashFileStore struct {
	path    string
	post    *model.Post
	target  model.PostTarget
	account model.SocialAccount
}

func (s *crashFileStore) read() (crashFileState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return crashFileState{}, err
	}
	var state crashFileState
	if err := json.Unmarshal(data, &state); err != nil {
		return crashFileState{}, err
	}
	return state, nil
}

func (s *crashFileStore) write(state crashFileState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *crashFileStore) GetPostForPublishing(context.Context, uuid.UUID) (*model.Post, error) {
	return s.post, nil
}

func (s *crashFileStore) GetPendingPostTargets(context.Context, uuid.UUID) ([]model.PostTarget, error) {
	state, err := s.read()
	if err != nil {
		return nil, err
	}
	if state.TargetStatus == "PUBLISHED" {
		return nil, nil
	}
	return []model.PostTarget{s.target}, nil
}

func (s *crashFileStore) GetPublicationAttempt(context.Context, uuid.UUID) (*model.PublicationAttempt, error) {
	state, err := s.read()
	if err != nil {
		return nil, err
	}
	return &model.PublicationAttempt{
		ID:             uuid.MustParse(os.Getenv("BLINK_TEST_ATTEMPT_ID")),
		PostTargetID:   s.target.ID,
		Status:         state.Status,
		AttemptCount:   state.AttemptCount,
		PlatformPostID: stringPtrIfSet(state.PlatformPostID),
		PlatformURL:    stringPtrIfSet(state.PlatformURL),
	}, nil
}

func (s *crashFileStore) GetPostTargetWithSocialAccount(context.Context, uuid.UUID) (*model.PostTarget, *model.SocialAccount, error) {
	return &s.target, &s.account, nil
}

func (s *crashFileStore) MarkAttemptProcessing(_ context.Context, _ uuid.UUID) error {
	state, err := s.read()
	if err != nil {
		return err
	}
	if state.Status != "PENDING" && state.Status != "UNKNOWN" && state.Status != "RETRYING" && state.Status != "FAILED" {
		return errors.New("attempt is not claimable")
	}
	state.Status = "PROCESSING"
	state.AttemptCount++
	return s.write(state)
}

func (s *crashFileStore) MarkAttemptUnknown(_ context.Context, _ uuid.UUID, _, _ string) error {
	state, err := s.read()
	if err != nil {
		return err
	}
	state.Status = "UNKNOWN"
	return s.write(state)
}

func (s *crashFileStore) ResetAttemptForRetry(context.Context, uuid.UUID) error {
	return nil
}

func (s *crashFileStore) MarkAttemptSucceeded(_ context.Context, _ uuid.UUID, postID, publicURL string) error {
	state, err := s.read()
	if err != nil {
		return err
	}
	state.Status = "SUCCEEDED"
	state.PlatformPostID = postID
	state.PlatformURL = publicURL
	return s.write(state)
}

func (s *crashFileStore) MarkAttemptFailed(context.Context, uuid.UUID, string, string) error {
	return nil
}

func (s *crashFileStore) MarkAttemptRetrying(context.Context, uuid.UUID, string, string, string, time.Time) error {
	return nil
}

func (s *crashFileStore) MarkPostTargetPublished(_ context.Context, _ uuid.UUID) error {
	state, err := s.read()
	if err != nil {
		return err
	}
	state.TargetStatus = "PUBLISHED"
	return s.write(state)
}

func (s *crashFileStore) UpdatePostStatus(context.Context, uuid.UUID, string) error {
	return nil
}

func (s *crashFileStore) RecordPermanentFailure(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *string, string, string, int, string, string, string, string, []byte) error {
	return nil
}

type crashHTTPConnector struct {
	baseURL string
}

func (c *crashHTTPConnector) Authenticate(context.Context, connectors.AuthParams) (connectors.AuthResult, error) {
	return connectors.AuthResult{}, nil
}

func (c *crashHTTPConnector) UploadMedia(context.Context, string, string, io.Reader, string) (string, error) {
	return "", nil
}

func (c *crashHTTPConnector) Publish(ctx context.Context, token, attemptID, caption string, mediaIDs ...string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/publish", strings.NewReader(caption))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("X-Attempt-ID", attemptID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	return "", "", errors.New("publish response was not persisted")
}

func (c *crashHTTPConnector) GetStatus(context.Context, string) (string, string, error) {
	return "published", "https://platform.test/posts/123", nil
}

func (c *crashHTTPConnector) ReconcilePublish(ctx context.Context, token, platformUserID, attemptID, caption string) (connectors.ReconciliationResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/reconcile?attempt="+attemptID, nil)
	if err != nil {
		return connectors.ReconciliationResult{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return connectors.ReconciliationResult{}, err
	}
	defer resp.Body.Close()
	var result connectors.ReconciliationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return connectors.ReconciliationResult{}, err
	}
	return result, nil
}

func TestSIGKILLRecoveryDoesNotRepublish(t *testing.T) {
	if os.Getenv("BLINK_CRASH_CHILD") == "1" {
		runCrashWorkerChild(t)
		return
	}

	statePath := filepath.Join(t.TempDir(), "attempt.json")
	postID := uuid.New()
	targetID := uuid.New()
	attemptID := uuid.New()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptedToken, err := auth.EncryptToken("access-token", key)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{"status":"PENDING","target_status":"PENDING"}`), 0600); err != nil {
		t.Fatalf("write initial state: %v", err)
	}

	var publishCalls atomic.Int32
	var reconcileCalls atomic.Int32
	publishStarted := make(chan struct{}, 1)
	allowPublish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/publish":
			publishCalls.Add(1)
			publishStarted <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-allowPublish:
			}
		case "/reconcile":
			reconcileCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"outcome":"FOUND","platform_post_id":"platform-123","public_url":"https://platform.test/posts/123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	childEnv := []string{
		"BLINK_CRASH_CHILD=1",
		"BLINK_CRASH_SERVER=" + server.URL,
		"BLINK_CRASH_STATE=" + statePath,
		"BLINK_TEST_ATTEMPT_ID=" + attemptID.String(),
		"BLINK_TEST_POST_ID=" + postID.String(),
		"BLINK_TEST_TARGET_ID=" + targetID.String(),
		"BLINK_TEST_TOKEN=" + encryptedToken,
		"BLINK_TEST_KEY=" + key,
		"BLINK_TEST_RECOVERY=0",
	}
	child := exec.Command(os.Args[0], "-test.run=TestSIGKILLRecoveryDoesNotRepublish", "-test.count=1")
	child.Env = append(os.Environ(), childEnv...)
	if err := child.Start(); err != nil {
		t.Fatalf("start crash child: %v", err)
	}
	select {
	case <-publishStarted:
	case <-time.After(5 * time.Second):
		_ = child.Process.Kill()
		t.Fatal("timed out waiting for fake platform acceptance")
	}
	if err := child.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("kill crash child: %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("expected crash child to be killed")
	}
	close(allowPublish)

	recoveryChild := exec.Command(os.Args[0], "-test.run=TestSIGKILLRecoveryDoesNotRepublish", "-test.count=1")
	recoveryEnv := append([]string{}, childEnv[:len(childEnv)-1]...)
	recoveryEnv = append(recoveryEnv, "BLINK_TEST_RECOVERY=1")
	recoveryChild.Env = append(os.Environ(), recoveryEnv...)
	output, err := recoveryChild.CombinedOutput()
	if err != nil {
		t.Fatalf("recovery child failed: %v\n%s", err, output)
	}

	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read final state: %v", err)
	}
	var finalState crashFileState
	if err := json.Unmarshal(stateData, &finalState); err != nil {
		t.Fatalf("decode final state: %v", err)
	}
	if finalState.Status != "SUCCEEDED" || finalState.PlatformPostID != "platform-123" {
		t.Fatalf("unexpected final attempt state: %+v", finalState)
	}
	if publishCalls.Load() != 1 {
		t.Fatalf("expected exactly one platform publish, got %d", publishCalls.Load())
	}
	if reconcileCalls.Load() != 1 {
		t.Fatalf("expected exactly one reconciliation, got %d", reconcileCalls.Load())
	}
}

func runCrashWorkerChild(t *testing.T) {
	postID := uuid.MustParse(os.Getenv("BLINK_TEST_POST_ID"))
	targetID := uuid.MustParse(os.Getenv("BLINK_TEST_TARGET_ID"))
	store := &crashFileStore{
		path:    os.Getenv("BLINK_CRASH_STATE"),
		post:    &model.Post{ID: postID, Caption: stringPtr("hello"), Status: "QUEUED", SchedulingGeneration: 1},
		target:  model.PostTarget{ID: targetID, PostID: postID, Status: "PENDING"},
		account: model.SocialAccount{Platform: connectors.PlatformTwitter, PlatformUserID: "user-123", AccessToken: os.Getenv("BLINK_TEST_TOKEN")},
	}
	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{
		connectors.PlatformTwitter: &crashHTTPConnector{baseURL: os.Getenv("BLINK_CRASH_SERVER")},
	}, os.Getenv("BLINK_TEST_KEY"))
	err := processor.publishToTarget(context.Background(), store.post, &store.target)
	if os.Getenv("BLINK_TEST_RECOVERY") == "1" {
		if err != nil {
			t.Fatalf("recovery attempt failed: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected crash child to remain in external call")
	}
}

func stringPtrIfSet(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringPtr(value string) *string {
	return &value
}

type testScheduler struct {
	enqueuedTasks []*asynq.Task
}

func (s *testScheduler) Enqueue(task *asynq.Task, opts ...asynq.Option) (string, error) {
	s.enqueuedTasks = append(s.enqueuedTasks, task)
	return "test-task-id", nil
}

type testMediaStore struct {
	content []byte
}

func (s *testMediaStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

type testMediaRepo struct {
	media *model.Media
}

func (r *testMediaRepo) GetMediaByID(ctx context.Context, id uuid.UUID) (*model.Media, error) {
	if r.media != nil && r.media.ID == id {
		return r.media, nil
	}
	return nil, errors.New("media not found")
}

type youtubeMockConnector struct {
	uploadedMediaID string
	publishedPostID string
	uploadCalled    bool
	publishCalled   bool
	reconcileCalled bool
}

func (y *youtubeMockConnector) Authenticate(ctx context.Context, params connectors.AuthParams) (connectors.AuthResult, error) {
	return connectors.AuthResult{}, nil
}

func (y *youtubeMockConnector) UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error) {
	y.uploadCalled = true
	y.uploadedMediaID = "yt-vid-999"
	return y.uploadedMediaID, nil
}

func (y *youtubeMockConnector) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error) {
	y.publishCalled = true
	if len(mediaIDs) == 0 {
		return "", "", connectors.NewClassifiedError(connectors.ErrorPermanent, "MEDIA_REQUIRED", errors.New("YouTube publishing requires a video"))
	}
	y.publishedPostID = mediaIDs[0]
	return "https://youtube.com/watch?v=" + y.publishedPostID, y.publishedPostID, nil
}

func (y *youtubeMockConnector) GetStatus(ctx context.Context, platformPostID string) (string, string, error) {
	return "published", "https://youtube.com/watch?v=" + platformPostID, nil
}

func (y *youtubeMockConnector) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (connectors.ReconciliationResult, error) {
	y.reconcileCalled = true
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}

func TestPublishToTarget_YouTubeMissingVideo_PermanentFailureNoReconciliation(t *testing.T) {
	postID := uuid.New()
	targetID := uuid.New()
	attemptID := uuid.New()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptedToken, err := auth.EncryptToken("access-token", key)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	store := &recoveryStore{
		post:    &model.Post{ID: postID, Caption: stringPtr("hello"), Status: "QUEUED", SchedulingGeneration: 1}, // no MediaURL and no MediaID
		target:  model.PostTarget{ID: targetID, PostID: postID, Status: "PENDING"},
		account: model.SocialAccount{Platform: connectors.PlatformYoutube, PlatformUserID: "channel-123", AccessToken: encryptedToken},
		attempt: model.PublicationAttempt{ID: attemptID, PostTargetID: targetID, Status: "PENDING", CreatedAt: time.Now()},
	}

	ytConnector := &youtubeMockConnector{}
	scheduler := &testScheduler{}

	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{
		connectors.PlatformYoutube: ytConnector,
	}, key, scheduler)

	processor.acquire = fakeAcquire(store)
	task, _ := jobs.NewPublishTask(postID, 1)
	err = processor.ProcessPublishJob(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessPublishJob returned error: %v", err)
	}

	// Verify attempt failed permanently
	if store.attempt.Status != "FAILED" {
		t.Fatalf("expected attempt status FAILED, got %s", store.attempt.Status)
	}

	// Verify reconciliation was NEVER called
	if ytConnector.reconcileCalled {
		t.Fatal("reconciliation should NEVER be called for missing video pre-publish error")
	}

	// Verify no reconciliation task was enqueued
	if len(scheduler.enqueuedTasks) > 0 {
		t.Fatalf("expected 0 retry/reconciliation tasks scheduled, got %d", len(scheduler.enqueuedTasks))
	}
}

func TestPublishToTarget_WithPersistentMedia_Success(t *testing.T) {
	postID := uuid.New()
	targetID := uuid.New()
	attemptID := uuid.New()
	mediaID := uuid.New()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptedToken, err := auth.EncryptToken("access-token", key)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	store := &recoveryStore{
		post:    &model.Post{ID: postID, Caption: stringPtr("video post"), MediaID: &mediaID, Status: "QUEUED", SchedulingGeneration: 1},
		target:  model.PostTarget{ID: targetID, PostID: postID, Status: "PENDING"},
		account: model.SocialAccount{Platform: connectors.PlatformYoutube, PlatformUserID: "channel-123", AccessToken: encryptedToken},
		attempt: model.PublicationAttempt{ID: attemptID, PostTargetID: targetID, Status: "PENDING", CreatedAt: time.Now()},
	}

	mediaRepo := &testMediaRepo{
		media: &model.Media{
			ID:           mediaID,
			StorageKey:   "users/123/video.mp4",
			ContentType:  "video/mp4",
			UploadStatus: "complete",
		},
	}
	mediaStore := &testMediaStore{content: []byte("video data")}

	ytConnector := &youtubeMockConnector{}
	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{
		connectors.PlatformYoutube: ytConnector,
	}, key).WithMediaStore(mediaRepo, mediaStore)

	processor.acquire = fakeAcquire(store)
	task, _ := jobs.NewPublishTask(postID, 1)
	err = processor.ProcessPublishJob(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessPublishJob returned error: %v", err)
	}

	if !ytConnector.uploadCalled {
		t.Fatal("expected UploadMedia to be called with persistent media")
	}
	if !ytConnector.publishCalled {
		t.Fatal("expected Publish to be called after media upload")
	}
	if store.attempt.Status != "SUCCEEDED" {
		t.Fatalf("expected attempt status SUCCEEDED, got %s", store.attempt.Status)
	}
	if store.target.Status != "PUBLISHED" {
		t.Fatalf("expected target status PUBLISHED, got %s", store.target.Status)
	}
}

func fakeAcquire(store *recoveryStore) acquirePublishLock {
	return func(context.Context, uuid.UUID) (publicationExecutionStore, func(), bool, error) {
		return store, func() {}, true, nil
	}
}

func (s *recoveryStore) ClaimPostPublishing(_ context.Context, id uuid.UUID, generation int) (bool, error) {
	if s.post.ID != id || s.post.SchedulingGeneration != generation ||
		(s.post.Status != "QUEUED" && s.post.Status != "PUBLISHING" && s.post.Status != "SCHEDULED") ||
		(s.post.ScheduledAt != nil && s.post.ScheduledAt.After(time.Now())) {
		return false, nil
	}
	s.post.Status = "PUBLISHING"
	return true, nil
}

func (s *recoveryStore) RecomputePostStatus(context.Context, uuid.UUID) error {
	switch s.target.Status {
	case "PUBLISHED", "FAILED":
		s.post.Status = s.target.Status
	}
	return nil
}

type failingExecutionStore struct {
	*recoveryStore
	persistErr error
}

func (s *failingExecutionStore) MarkAttemptRetrying(context.Context, uuid.UUID, string, string, string, time.Time) error {
	return s.persistErr
}
func (s *failingExecutionStore) RecordPermanentFailure(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *string, string, string, int, string, string, string, string, []byte) error {
	return s.persistErr
}
func (s *failingExecutionStore) QueueReconciliation(context.Context, uuid.UUID, uuid.UUID, int) error {
	return s.persistErr
}

type failureConnector struct {
	crashRecoveryConnector
	cause error
}

func (c *failureConnector) Publish(context.Context, string, string, string, ...string) (string, string, error) {
	c.publishCalls++
	return "", "", c.cause
}

func TestPublicationPersistenceFailureIsNotAcknowledged(t *testing.T) {
	for _, class := range []connectors.ErrorClass{connectors.ErrorRetryable, connectors.ErrorPermanent, connectors.ErrorAmbiguous} {
		t.Run(string(class), func(t *testing.T) {
			key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
			token, err := auth.EncryptToken("token", key)
			if err != nil {
				t.Fatal(err)
			}
			postID, targetID := uuid.New(), uuid.New()
			dbErr := errors.New("database unavailable")
			base := &recoveryStore{post: &model.Post{ID: postID, Status: "QUEUED", SchedulingGeneration: 1}, target: model.PostTarget{ID: targetID, PostID: postID, Status: "PENDING"}, account: model.SocialAccount{Platform: "twitter", AccessToken: token}, attempt: model.PublicationAttempt{ID: uuid.New(), PostTargetID: targetID, Status: "PENDING"}}
			store := &failingExecutionStore{recoveryStore: base, persistErr: dbErr}
			connector := &failureConnector{cause: connectors.NewClassifiedError(class, "TEST", errors.New("platform failure"))}
			processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{"twitter": connector}, key)
			processor.acquire = func(context.Context, uuid.UUID) (publicationExecutionStore, func(), bool, error) {
				return store, func() {}, true, nil
			}
			task, err := jobs.NewPublishTask(postID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := processor.ProcessPublishJob(context.Background(), task); !errors.Is(err, dbErr) {
				t.Fatalf("persistence failure acknowledged: %v", err)
			}
		})
	}
}

func TestTargetRetryDoesNotPublishAnotherTarget(t *testing.T) {
	id, target, other := uuid.New(), uuid.New(), uuid.New()
	store := &recoveryStore{post: &model.Post{ID: id, Status: "QUEUED", SchedulingGeneration: 1}, target: model.PostTarget{ID: target, PostID: id, Status: "PENDING"}, attempt: model.PublicationAttempt{Status: "PENDING"}}
	connector := &failureConnector{}
	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{"twitter": connector}, "")
	processor.acquire = fakeAcquire(store)
	task, err := jobs.NewPublishTaskForJob(jobs.PublishJob{PostID: id, TargetID: &other, SchedulingGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := processor.ProcessPublishJob(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if store.attempt.Status != "PENDING" || connector.publishCalls != 0 {
		t.Fatal("targeted retry processed another target")
	}
}

func TestRecoveryCredentialFailurePreservesUnknownOutcome(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encrypted, err := auth.EncryptToken("token", key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, platform string
		expired               bool
	}{
		{"expired without refresh token", encrypted, "twitter", true},
		{"corrupt encrypted token", "invalid", "twitter", false},
		{"missing connector", encrypted, "missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, target := uuid.New(), uuid.New()
			store := &recoveryStore{post: &model.Post{ID: id, Status: "PUBLISHING", SchedulingGeneration: 1}, target: model.PostTarget{ID: target, PostID: id, Status: "PENDING"}, account: model.SocialAccount{Platform: tc.platform, AccessToken: tc.token}, attempt: model.PublicationAttempt{ID: uuid.New(), PostTargetID: target, Status: "PROCESSING", AttemptCount: 1}}
			if tc.expired {
				past := time.Now().Add(-time.Hour)
				store.account.ExpiresAt = &past
			}
			conn := &crashRecoveryConnector{}
			processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{"twitter": conn}, key)
			processor.acquire = fakeAcquire(store)
			task, err := jobs.NewPublishTask(id, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := processor.ProcessPublishJob(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if store.attempt.Status != "UNKNOWN" || store.attempt.ErrorCode == nil || *store.attempt.ErrorCode != "MANUAL_REVIEW_REQUIRED" || store.target.Status != "PENDING" || conn.publishCalls != 0 {
				t.Fatalf("uncertain publication became retryable: %+v", store.attempt)
			}
		})
	}
}

type mockXConnectorWithUploadFailure struct {
	uploadCalled  bool
	publishCalled bool
}

func (m *mockXConnectorWithUploadFailure) Authenticate(ctx context.Context, params connectors.AuthParams) (connectors.AuthResult, error) {
	return connectors.AuthResult{}, nil
}

func (m *mockXConnectorWithUploadFailure) UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error) {
	m.uploadCalled = true
	return "", connectors.NewClassifiedError(connectors.ErrorPermanent, "UPLOAD_FAILED", errors.New("simulated upload failure"))
}

func (m *mockXConnectorWithUploadFailure) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error) {
	m.publishCalled = true
	return "", "", nil
}

func (m *mockXConnectorWithUploadFailure) GetStatus(ctx context.Context, platformPostID string) (string, string, error) {
	return "published", "", nil
}

func (m *mockXConnectorWithUploadFailure) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}

func TestTwitterMediaUploadFailurePreventsPublication(t *testing.T) {
	postID := uuid.New()
	targetID := uuid.New()
	attemptID := uuid.New()
	mediaID := uuid.New()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptedToken, err := auth.EncryptToken("access-token", key)
	if err != nil {
		t.Fatal(err)
	}

	caption := "Test X post with media"
	store := &recoveryStore{
		post: &model.Post{
			ID:                   postID,
			Caption:              &caption,
			MediaID:              &mediaID,
			Status:               "PUBLISHING",
			SchedulingGeneration: 1,
		},
		target: model.PostTarget{
			ID:     targetID,
			PostID: postID,
			Status: "PENDING",
		},
		account: model.SocialAccount{
			Platform:    "twitter",
			AccessToken: encryptedToken,
		},
		attempt: model.PublicationAttempt{
			ID:           attemptID,
			PostTargetID: targetID,
			Status:       "PENDING",
			AttemptCount: 1,
			CreatedAt:    time.Now(),
		},
	}

	mediaRepo := &testMediaRepo{
		media: &model.Media{
			ID:           mediaID,
			StorageKey:   "users/123/image.png",
			ContentType:  "image/png",
			UploadStatus: "complete",
		},
	}
	mediaStore := &testMediaStore{content: []byte("image data")}

	xConnector := &mockXConnectorWithUploadFailure{}
	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{
		connectors.PlatformTwitter: xConnector,
	}, key).WithMediaStore(mediaRepo, mediaStore)

	processor.acquire = fakeAcquire(store)
	task, _ := jobs.NewPublishTask(postID, 1)
	err = processor.ProcessPublishJob(context.Background(), task)
	if err != nil {
		t.Fatalf("ProcessPublishJob returned unexpected error: %v", err)
	}

	if !xConnector.uploadCalled {
		t.Fatal("expected UploadMedia to be called")
	}
	if xConnector.publishCalled {
		t.Fatal("expected Publish NOT to be called when UploadMedia fails")
	}
	if store.attempt.Status != "FAILED" {
		t.Fatalf("expected attempt status FAILED, got %s", store.attempt.Status)
	}
}
