package connectors

import (
	"context"
	"io"
)

type ReconciliationOutcome string

const (
	ReconciliationFound             ReconciliationOutcome = "FOUND"
	ReconciliationNotFoundConfirmed ReconciliationOutcome = "NOT_FOUND_CONFIRMED"
	ReconciliationUnknown           ReconciliationOutcome = "UNKNOWN"
)

type ReconciliationResult struct {
	Outcome        ReconciliationOutcome `json:"outcome"`
	PlatformPostID string                `json:"platform_post_id"`
	PublicURL      string                `json:"public_url"`
}

// PlatformConnector defines the interface for publishing to a platform.
// Each platform (e.g. X, YouTube) implements its own package-level connector
// that satisfies this interface. The publish worker dispatches through this
// interface, so it never needs to import a concrete platform package.
type PlatformConnector interface {
	// Authenticate exchanges an OAuth auth code for tokens,
	// which are encrypted and stored in the DB.
	Authenticate(ctx context.Context, params AuthParams) (result AuthResult, err error)

	// UploadMedia uploads media to the platform and returns a URL/ID.
	// For Twitter (X) it uploads via v2 API; for YouTube it queues a video upload.
	UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (mediaId string, err error)

	// Publish posts content to the platform.
	// Returns the public URL and platform-specific post ID.
	Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (publicURL string, platformPostID string, err error)

	// GetStatus polls the platform for post status.
	// Used for async publishing (e.g. YouTube video processing).
	GetStatus(ctx context.Context, platformPostID string) (status string, publicURL string, err error)

	// ReconcilePublish resolves an ambiguous publication without creating another post.
	ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (ReconciliationResult, error)
}

// OAuthConnector is implemented by any platform that supports the
// browser-redirect OAuth flow. The generic AuthHandler uses this interface
// so it never needs to import a concrete platform package.
type OAuthConnector interface {
	PlatformConnector

	// PlatformID returns the stable slug for this platform (e.g. "twitter").
	// It is used as the URL segment (/auth/<platform>) and as the storage key.
	PlatformID() string

	// AuthCodeURL returns the URL the browser should be redirected to in order
	// to begin the OAuth flow for this platform.
	AuthCodeURL(state, verifier string) string
}

var _ PlatformConnector = (*MockConnector)(nil)

// Platform Constants
const (
	PlatformTwitter = "twitter"
	PlatformYoutube = "youtube"
)
