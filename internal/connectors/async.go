package connectors

import (
	"context"

	"github.com/iamvalson/blink/internal/model"
)

type VideoPublishInput struct {
	Caption  string
	MediaURL string
	Options  model.TikTokOptions
}
type VideoPublishStatus struct {
	Status        string
	FailureReason string
	PublicPostIDs []string
}

// AsyncVideoPublisher initializes an operation without claiming publication.
// Callers must durably save its ID before any follow-up request.
type AsyncVideoPublisher interface {
	ValidateVideo(context.Context, string, VideoPublishInput) error
	InitializeVideo(context.Context, string, VideoPublishInput) (string, error)
	FetchPublishStatus(context.Context, string, string) (*VideoPublishStatus, error)
}
