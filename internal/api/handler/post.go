package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
	"github.com/rs/zerolog/log"
)

type PostHandler struct {
    postService *service.PostService
}

func NewPostHandler(postService *service.PostService) *PostHandler {
    return &PostHandler{
        postService: postService,
    }
}

func (h *PostHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
    // Extract user ID from context
    userIDStr, ok := middleware.UserIDFromContext(r.Context())
    if !ok {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        http.Error(w, "Invalid user ID", http.StatusBadRequest)
        return
    }

    // Read idempotency key from header
    idempotencyKey := r.Header.Get("Idempotency-Key")
    if idempotencyKey == "" {
        http.Error(w, "Idempotency-Key header is required", http.StatusBadRequest)
        return
    }

    // Parse request body
    var input model.CreatePostInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    // Validate input
    if input.Caption == nil || *input.Caption == "" {
        http.Error(w, "caption is required", http.StatusBadRequest)
        return
    }

    if len(input.Targets) == 0 {
        http.Error(w, "at least one target is required", http.StatusBadRequest)
        return
    }

    // Create post
    post, err := h.postService.CreatePost(r.Context(), userID, idempotencyKey, input)
    if err != nil {
        log.Error().
            Err(err).
            Str("user_id", userID.String()).
            Msg("Failed to create post")
        
        http.Error(w, "Failed to create post", http.StatusInternalServerError)
        return
    }

    // Return 202 Accepted
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusAccepted)
    if err := json.NewEncoder(w).Encode(post); err != nil {
        log.Error().Err(err).Msg("Failed to encode post response")
    }
}

func (h *PostHandler) GetPost(w http.ResponseWriter, r *http.Request) {
    userIDStr, ok := middleware.UserIDFromContext(r.Context())
    if !ok {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        http.Error(w, "Invalid user ID", http.StatusBadRequest)
        return
    }

    postIDStr := chi.URLParam(r, "id")
    postID, err := uuid.Parse(postIDStr)
    if err != nil {
        http.Error(w, "Invalid post ID", http.StatusBadRequest)
        return
    }

    post, err := h.postService.GetPost(r.Context(), userID, postID)
    if err != nil {
        if errors.Is(err, service.ErrPostNotFound) {
            http.Error(w, "Post not found", http.StatusNotFound)
            return
        }
        log.Error().
            Err(err).
            Str("user_id", userID.String()).
            Str("post_id", postID.String()).
            Msg("Failed to get post")

        http.Error(w, "Failed to get post", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    if err := json.NewEncoder(w).Encode(post); err != nil {
        log.Error().Err(err).Msg("Failed to encode post response")
    }
}