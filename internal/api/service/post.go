package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

var (
	ErrInvalidIdempotencyKey   = errors.New("invalid idempotency key")
	ErrNoTargets               = errors.New("at least one target is required")
	ErrPostNotFound            = storage.ErrPostNotFound
	ErrPostDeleteForbidden     = storage.ErrPostDeleteForbidden
	ErrPostCannotBeRescheduled = storage.ErrPostCannotBeRescheduled
	ErrInvalidScheduledTime    = storage.ErrInvalidScheduledTime
	ErrPostCannotBeCancelled   = storage.ErrPostCannotBeCancelled
	ErrPostCannotBeRetried     = storage.ErrPostCannotBeRetried

	// ErrPostValidation is returned for any client-correctable input error
	// during post creation. Handlers should map this to HTTP 400.
	ErrPostValidation = errors.New("post validation failed")
)

type PostService struct {
	posts *storage.PostRepository
}

func NewPostService(posts *storage.PostRepository) *PostService {
	return &PostService{
		posts: posts,
	}
}

func (s *PostService) CreatePost(
	ctx context.Context,
	userID uuid.UUID,
	idempotencyKey string,
	input model.CreatePostInput,
) (*model.Post, error) {
	// Validate idempotency key
	if idempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency key is required", ErrPostValidation)
	}

	// Validate targets
	if len(input.Targets) == 0 {
		return nil, fmt.Errorf("%w: at least one target is required", ErrPostValidation)
	}

	// Validate input
	if input.Caption == nil || *input.Caption == "" {
		return nil, fmt.Errorf("%w: caption is required", ErrPostValidation)
	}

	// Call repository to create post. Storage-layer validation errors (platform
	// rules, media checks, etc.) are wrapped so the handler can route them to
	// HTTP 400 without inspecting error message strings.
	post, err := s.posts.CreatePost(ctx, userID, idempotencyKey, input)
	if err != nil {
		if isStorageValidationError(err) {
			return nil, fmt.Errorf("%w: %v", ErrPostValidation, err)
		}
		return nil, fmt.Errorf("create post: %w", err)
	}

	return post, nil
}

// isStorageValidationError returns true for errors that originate from
// business-rule or data-validation checks in the storage layer and should
// therefore be surfaced to the caller as a 400 Bad Request.
func isStorageValidationError(err error) bool {
	// Explicit sentinel errors defined in storage
	if errors.Is(err, storage.ErrInvalidScheduledTime) {
		return true
	}
	// The storage layer returns plain fmt.Errorf strings for platform-specific
	// and media validation failures; detect them by unwrapping to their root
	// message categories rather than by inspecting the full string.
	// As new sentinel errors are added to storage, move them above.
	msg := err.Error()
	validationPhrases := []string{
		"YouTube publishing requires",
		"Twitter/X posts can include",
		"Twitter/X does not allow",
		"unsupported media type for Twitter/X",
		"media not found",
		"media upload is not complete",
		"media does not belong to user",
		"social account",
		"browser blob:",
		"remote media URLs without durable",
		"only one media attachment",
		"media_id and media_ids must",
		"invalid content_format",
	}
	for _, phrase := range validationPhrases {
		if len(msg) >= len(phrase) {
			// Use a simple prefix/contains check — this stays inside the service
			// layer and is the single place where such inspection occurs.
			for i := 0; i <= len(msg)-len(phrase); i++ {
				if msg[i:i+len(phrase)] == phrase {
					return true
				}
			}
		}
	}
	return false
}

func (s *PostService) GetPost(
	ctx context.Context,
	userID uuid.UUID,
	postID uuid.UUID,
) (*model.PostWithDetails, error) {
	post, err := s.posts.GetPostWithDetails(ctx, userID, postID)
	if err != nil {
		return nil, err
	}
	return post, nil
}

func (s *PostService) ListPosts(
	ctx context.Context,
	userID uuid.UUID,
	filter storage.ListPostsFilter,
) ([]model.PostWithDetails, int, error) {
	return s.posts.ListPosts(ctx, userID, filter)
}

func (s *PostService) DeletePost(
	ctx context.Context,
	userID uuid.UUID,
	postID uuid.UUID,
) error {
	return s.posts.DeletePost(ctx, userID, postID)
}

func (s *PostService) RetryPost(
	ctx context.Context,
	userID uuid.UUID,
	postID uuid.UUID,
) (*model.PostWithDetails, error) {
	if err := s.posts.RetryPost(ctx, userID, postID); err != nil {
		return nil, err
	}
	return s.posts.GetPostWithDetails(ctx, userID, postID)
}

func (s *PostService) ReschedulePost(
	ctx context.Context,
	userID uuid.UUID,
	postID uuid.UUID,
	newScheduledAt time.Time,
) (*model.PostWithDetails, error) {
	return s.posts.ReschedulePost(ctx, userID, postID, newScheduledAt)
}

func (s *PostService) CancelSchedule(
	ctx context.Context,
	userID uuid.UUID,
	postID uuid.UUID,
) (*model.PostWithDetails, error) {
	return s.posts.CancelSchedule(ctx, userID, postID)
}

func (s *PostService) ListCalendarPosts(
	ctx context.Context,
	userID uuid.UUID,
	from, to time.Time,
	status, platform string,
) ([]model.PostWithDetails, error) {
	return s.posts.ListCalendarPosts(ctx, userID, from, to, status, platform)
}

func (s *PostService) ListUpcomingPosts(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int,
) ([]model.PostWithDetails, int, error) {
	return s.posts.ListUpcomingPosts(ctx, userID, limit, offset)
}