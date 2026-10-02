package connectors

import (
	"context"
	"io"
)

// MockConnector is a test double for PlatformConnector
type MockConnector struct {
	AuthenticateFn func(ctx context.Context, params AuthParams) (AuthResult, error)
	UploadMediaFn  func(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error)
	PublishFn      func(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error)
	GetStatusFn    func(ctx context.Context, platformPostID string) (string, string, error)
	ReconcileFn    func(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (bool, string, string, error)
}

func (m *MockConnector) Authenticate(ctx context.Context, params AuthParams) (AuthResult, error) {
	return m.AuthenticateFn(ctx, params)
}

func (m *MockConnector) UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error) {
	return m.UploadMediaFn(ctx, token, attemptID, media, mediaType)
}

func (m *MockConnector) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error) {
	return m.PublishFn(ctx, token, attemptID, caption, mediaIDs...)
}

func (m *MockConnector) GetStatus(ctx context.Context, platformPostID string) (string, string, error) {
	return m.GetStatusFn(ctx, platformPostID)
}

func (m *MockConnector) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (ReconciliationResult, error) {
	found, publicURL, platformPostID, err := m.ReconcileFn(ctx, token, platformUserID, attemptID, caption)
	if err != nil {
		return ReconciliationResult{}, err
	}
	if found {
		return ReconciliationResult{Outcome: ReconciliationFound, PublicURL: publicURL, PlatformPostID: platformPostID}, nil
	}
	return ReconciliationResult{Outcome: ReconciliationNotFoundConfirmed}, nil
}

// NewMockConnector creates a mock with default no-op implementations.
func NewMockConnector() *MockConnector {
	return &MockConnector{
		AuthenticateFn: func(ctx context.Context, params AuthParams) (AuthResult, error) {
			return AuthResult{PlatformUserID: "mock_user_123"}, nil
		},
		UploadMediaFn: func(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error) {
			return "mock_media_123", nil
		},
		PublishFn: func(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error) {
			return "https://example.com/post/123", "mock_post_123", nil
		},
		GetStatusFn: func(ctx context.Context, platformPostID string) (string, string, error) {
			return "published", "https://example.com/post/123", nil
		},
		ReconcileFn: func(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (bool, string, string, error) {
			return false, "", "", nil
		},
	}
}
