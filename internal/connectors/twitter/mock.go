package twitter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/connectors"
)

// MockConnector simulates the Twitter/X platform connector for testing and local development
// without making any outbound network requests or consuming API credits.
type MockConnector struct{}

var _ connectors.PlatformConnector = (*MockConnector)(nil)

// NewMock creates a new mock Twitter/X platform connector.
func NewMock() *MockConnector {
	return &MockConnector{}
}

// Authenticate simulates an OAuth exchange and returns fake user credentials.
func (m *MockConnector) Authenticate(ctx context.Context, params connectors.AuthParams) (connectors.AuthResult, error) {
	fakeID := fmt.Sprintf("mock_user_%s", uuid.New().String()[:8])
	return connectors.AuthResult{
		PlatformUserID: fakeID,
		AccessToken:    fmt.Sprintf("mock_token_%s", uuid.New().String()),
		RefreshToken:   fmt.Sprintf("mock_refresh_%s", uuid.New().String()),
		Expiry:         time.Now().Add(24 * time.Hour),
	}, nil
}

// UploadMedia simulates media upload to Twitter/X and returns a fake media ID.
func (m *MockConnector) UploadMedia(ctx context.Context, media io.Reader, mediaType string) (string, error) {
	return fmt.Sprintf("mock_media_%s", uuid.New().String()[:8]), nil
}

// Publish simulates posting a tweet on X without making network requests.
// It returns a mock platform post ID and URL.
func (m *MockConnector) Publish(ctx context.Context, token string, caption string, mediaIDs ...string) (publicURL string, platformPostID string, err error) {
	if token == "" {
		return "", "", fmt.Errorf("no access token set")
	}

	platformPostID = fmt.Sprintf("mock_x_%s", uuid.New().String()[:12])
	publicURL = fmt.Sprintf("http://mock.x.local/status/%s", platformPostID)

	return publicURL, platformPostID, nil
}

// GetStatus returns the mock status and URL for a simulated post.
func (m *MockConnector) GetStatus(ctx context.Context, platformPostID string) (status string, publicURL string, err error) {
	publicURL = fmt.Sprintf("http://mock.x.local/status/%s", platformPostID)
	return "published", publicURL, nil
}

