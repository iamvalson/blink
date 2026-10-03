package worker

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
)

type refreshTestConnector struct {
	refreshCalls int
	publishToken string
}

func (c *refreshTestConnector) Authenticate(context.Context, connectors.AuthParams) (connectors.AuthResult, error) {
	return connectors.AuthResult{}, nil
}
func (c *refreshTestConnector) UploadMedia(context.Context, string, string, io.Reader, string) (string, error) {
	return "media", nil
}
func (c *refreshTestConnector) Publish(_ context.Context, token, _ string, _ string, _ ...string) (string, string, error) {
	c.publishToken = token
	return "https://example.test/post", "post-1", nil
}
func (c *refreshTestConnector) GetStatus(context.Context, string) (string, string, error) {
	return "published", "https://example.test/post", nil
}
func (c *refreshTestConnector) ReconcilePublish(context.Context, string, string, string, string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}
func (c *refreshTestConnector) RefreshToken(context.Context, string) (connectors.TokenResult, error) {
	c.refreshCalls++
	expiresAt := time.Now().Add(time.Hour)
	return connectors.TokenResult{AccessToken: "new-access", ExpiresAt: &expiresAt}, nil
}

func TestPublishToTargetRefreshesAndPersistsExpiredToken(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	accessToken, err := auth.EncryptToken("old-access", key)
	if err != nil {
		t.Fatal(err)
	}
	refreshToken, err := auth.EncryptToken("refresh", key)
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Hour)
	postID, targetID, attemptID, accountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := &recoveryStore{
		post:    &model.Post{ID: postID, Caption: stringPtr("hello"), Status: "QUEUED"},
		target:  model.PostTarget{ID: targetID, PostID: postID, Status: "PENDING"},
		account: model.SocialAccount{ID: accountID, Platform: connectors.PlatformTwitter, AccessToken: accessToken, RefreshToken: stringPtr(refreshToken), ExpiresAt: &expired},
		attempt: model.PublicationAttempt{ID: attemptID, PostTargetID: targetID, Status: "PENDING"},
	}
	connector := &refreshTestConnector{}
	processor := NewPublishProcessor(nil, store, map[string]connectors.PlatformConnector{connectors.PlatformTwitter: connector}, key)

	if err := processor.publishToTarget(context.Background(), store.post, &store.target); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if connector.refreshCalls != 1 || connector.publishToken != "new-access" {
		t.Fatalf("expected one refresh and refreshed publish token, got refreshes=%d token=%q", connector.refreshCalls, connector.publishToken)
	}
	decrypted, err := auth.DecryptToken(store.account.AccessToken, key)
	if err != nil || decrypted != "new-access" {
		t.Fatalf("expected persisted encrypted access token, got %q, %v", decrypted, err)
	}
}
