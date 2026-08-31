package connectors

import (
	"context"
	"io"
)

// PlatformConnector defines the interface for publishing to a platform
// Each platform (eg. X, Instagram, etc) implements its own

type PlatformConnector interface {
	// Authenticate exchanges an OAuth auth code for tokens
	// Which are encryted and stored in db
	Authenticate(ctx context.Context, code string) (platformUserID string, err error)


	// UploadMedia uploads media to the platform and return a URL/ID
	// For Twitter (X), it uploads via v2 API
	// For Youtube, this queues video upload
	UploadMedia(ctx context.Context, media io.Reader, mediaType string) (mediaId string, err error)


	// Publish post content to the platform
	// Returns the public URL and platform-specific post ID
	Publish(ctx context.Context, caption string, mediaIDs ...string) (publicURL string, platformPostID string, err error)


	// GetStatus polls the platform for post status
	// Used for async publishing (eg. Youtube video processing)
	GetStatus(ctx context.Context, platformPostID string) (status string, publicURL string, err error)
}


// Platform Constants
const (
	PlatformTwitter = "twitter"
	PlatformYoutube = "youtube"
)