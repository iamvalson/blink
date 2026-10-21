package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
)

func TestPostHandlerCreatePostValidationError(t *testing.T) {
	svc := service.NewPostService(nil)
	h := NewPostHandler(svc)

	userID := uuid.New()
	caption := "Test Post"
	input := model.CreatePostInput{
		Caption: &caption,
		Targets: []uuid.UUID{}, // empty targets triggers validation error
	}
	body, _ := json.Marshal(input)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "idem-123")
	ctx := middleware.ContextWithUserID(req.Context(), userID.String())
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.CreatePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for validation failure, got %d. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestPostHandlerPostStatusSSEUnauthorized(t *testing.T) {
	svc := service.NewPostService(nil)
	h := NewPostHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/123/status", nil)
	rec := httptest.NewRecorder()

	h.PostStatusSSE(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestPostHandlerPostStatusSSEInvalidPostID(t *testing.T) {
	svc := service.NewPostService(nil)
	h := NewPostHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/invalid-uuid/status", nil)
	ctx := middleware.ContextWithUserID(context.Background(), uuid.New().String())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.PostStatusSSE(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid post id, got %d", rec.Code)
	}
}

