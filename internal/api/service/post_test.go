package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
)

func TestCreatePostValidationErrors(t *testing.T) {
	svc := NewPostService(nil)
	ctx := context.Background()
	userID := uuid.New()

	t.Run("missing idempotency key returns ErrPostValidation", func(t *testing.T) {
		caption := "Hello world"
		_, err := svc.CreatePost(ctx, userID, "", model.CreatePostInput{
			Caption: &caption,
			Targets: []uuid.UUID{uuid.New()},
		})
		if !errors.Is(err, ErrPostValidation) {
			t.Fatalf("expected ErrPostValidation, got %v", err)
		}
	})

	t.Run("missing targets returns ErrPostValidation", func(t *testing.T) {
		caption := "Hello world"
		_, err := svc.CreatePost(ctx, userID, "key-123", model.CreatePostInput{
			Caption: &caption,
			Targets: []uuid.UUID{},
		})
		if !errors.Is(err, ErrPostValidation) {
			t.Fatalf("expected ErrPostValidation, got %v", err)
		}
	})

	t.Run("missing caption returns ErrPostValidation", func(t *testing.T) {
		_, err := svc.CreatePost(ctx, userID, "key-123", model.CreatePostInput{
			Targets: []uuid.UUID{uuid.New()},
		})
		if !errors.Is(err, ErrPostValidation) {
			t.Fatalf("expected ErrPostValidation, got %v", err)
		}
	})

	t.Run("empty caption returns ErrPostValidation", func(t *testing.T) {
		empty := ""
		_, err := svc.CreatePost(ctx, userID, "key-123", model.CreatePostInput{
			Caption: &empty,
			Targets: []uuid.UUID{uuid.New()},
		})
		if !errors.Is(err, ErrPostValidation) {
			t.Fatalf("expected ErrPostValidation, got %v", err)
		}
	})
}

