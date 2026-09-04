package connectors

import (
	"context"
	"io"
)

// MockConnector is a test double for PlatformConnector
type MockConnector struct {
	AuthenticateFn func(ctx context.Context, params AuthParams) (string, error)
	UploadMediaFn  func(ctx context.Context, media io.Reader, mediaType string) (string, error)
	PublishFn      func(ctx context.Context, caption string, mediaIDs ...string) (string, string, error)
	GetStatusFn    func(ctx context.Context, platformPostID string) (string, string, error)
}


func (m *MockConnector) Authenticate(ctx context.Context, params AuthParams) (string, error) {
	return m.AuthenticateFn(ctx, params)
}

func (m *MockConnector) UploadMedia(ctx context.Context, media io.Reader, mediaType string) (string, error) {
	return m.UploadMediaFn(ctx, media, mediaType)
}

func (m *MockConnector) Publish(ctx context.Context, caption string, mediaIDs ...string) (string, string, error) {
	return m.PublishFn(ctx, caption, mediaIDs...)
}

func (m *MockConnector) GetStatus(ctx context.Context, platformPostID string) (string, string, error) {
	return m.GetStatusFn(ctx, platformPostID)
}

// NewMockConnector creates a mock with default no-op implementations.
func NewMockConnector() *MockConnector {
	return &MockConnector{
		AuthenticateFn: func(ctx context.Context, params AuthParams) (string, error) {
			return "mock_user_123", nil
		},
		UploadMediaFn: func(ctx context.Context, media io.Reader, mediaType string) (string, error) {
			return "mock_media_123", nil
		},
		PublishFn: func(ctx context.Context, caption string, mediaIDs ...string) (string, string, error) {
			return "https://example.com/post/123", "mock_post_123", nil
		},
		GetStatusFn: func(ctx context.Context, platformPostID string) (string, string, error) {
			return "published", "https://example.com/post/123", nil
		},
	}
}