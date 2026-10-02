package worker

import (
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
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
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
	if s.attempt.Status != "PENDING" && s.attempt.Status != "UNKNOWN" {
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

func (s *recoveryStore) MarkPostTargetPublished(context.Context, uuid.UUID) error {
	s.target.Status = "PUBLISHED"
	return nil
}

func (s *recoveryStore) UpdatePostStatus(_ context.Context, _ uuid.UUID, status string) error {
	s.post.Status = status
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
		post:    &model.Post{ID: postID, Caption: stringPtr("hello"), Status: "QUEUED"},
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
	if state.Status != "PENDING" && state.Status != "UNKNOWN" && state.Status != "FAILED" {
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
		post:    &model.Post{ID: postID, Caption: stringPtr("hello"), Status: "QUEUED"},
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
