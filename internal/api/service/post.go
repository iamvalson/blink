package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

var (
    ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")
    ErrNoTargets             = errors.New("at least one target is required")
    ErrPostNotFound          = storage.ErrPostNotFound
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
        return nil, ErrInvalidIdempotencyKey
    }

    // Validate targets
    if len(input.Targets) == 0 {
        return nil, ErrNoTargets
    }

    // Validate input
    if input.Caption == nil || *input.Caption == "" {
        return nil, errors.New("caption is required")
    }

    // Call repository to create post
    post, err := s.posts.CreatePost(ctx, userID, idempotencyKey, input)
    if err != nil {
        return nil, fmt.Errorf("create post: %w", err)
    }

    return post, nil
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