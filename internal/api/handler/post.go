package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
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
		log.Warn().
			Err(err).
			Str("user_id", userID.String()).
			Msg("Post creation rejected")

		errMsg := err.Error()
		if strings.Contains(errMsg, "requires a video") ||
			strings.Contains(errMsg, "media") ||
			strings.Contains(errMsg, "target") ||
			strings.Contains(errMsg, "caption") ||
			strings.Contains(errMsg, "idempotency") ||
			strings.Contains(errMsg, "content_format") ||
			strings.Contains(errMsg, "invalid content") {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

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

// ListPostsResponse is the paginated response for listing posts.
type ListPostsResponse struct {
	Posts  []model.PostWithDetails `json:"posts"`
	Total  int                     `json:"total"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
}

func (h *PostHandler) ListPosts(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	filter := storage.ListPostsFilter{
		Status:   q.Get("status"),
		Platform: q.Get("platform"),
		Search:   q.Get("search"),
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = n
		}
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = n
		}
	}

	posts, total, err := h.postService.ListPosts(r.Context(), userID, filter)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.String()).Msg("Failed to list posts")
		http.Error(w, "Failed to list posts", http.StatusInternalServerError)
		return
	}

	// Return empty slice rather than null
	if posts == nil {
		posts = []model.PostWithDetails{}
	}

	resp := ListPostsResponse{
		Posts:  posts,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("Failed to encode posts response")
	}
}

func (h *PostHandler) DeletePost(w http.ResponseWriter, r *http.Request) {
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

	if err := h.postService.DeletePost(r.Context(), userID, postID); err != nil {
		if errors.Is(err, service.ErrPostNotFound) {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrPostDeleteForbidden) {
			http.Error(w, "Post cannot be deleted while publishing is in progress", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("user_id", userID.String()).Str("post_id", postID.String()).Msg("Failed to delete post")
		http.Error(w, "Failed to delete post", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PostHandler) RetryPost(w http.ResponseWriter, r *http.Request) {
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

	post, err := h.postService.RetryPost(r.Context(), userID, postID)
	if err != nil {
		if errors.Is(err, service.ErrPostNotFound) {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "only FAILED posts") {
			http.Error(w, errMsg, http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("user_id", userID.String()).Str("post_id", postID.String()).Msg("Failed to retry post")
		http.Error(w, "Failed to retry post", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(post); err != nil {
		log.Error().Err(err).Msg("Failed to encode post response")
	}
}

// ReschedulePost sets or changes the scheduled_at time for a SCHEDULED post.
func (h *PostHandler) ReschedulePost(w http.ResponseWriter, r *http.Request) {
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

	var input model.ReschedulePostInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if input.ScheduledAt.IsZero() {
		http.Error(w, "scheduled_at is required", http.StatusBadRequest)
		return
	}
	if input.ScheduledAt.Before(time.Now().Add(30 * time.Second)) {
		http.Error(w, "scheduled_at must be at least 30 seconds in the future", http.StatusBadRequest)
		return
	}

	post, err := h.postService.ReschedulePost(r.Context(), userID, postID, input.ScheduledAt)
	if err != nil {
		if errors.Is(err, service.ErrPostNotFound) {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrPostCannotBeRescheduled) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, service.ErrInvalidScheduledTime) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Error().Err(err).Str("user_id", userID.String()).Str("post_id", postID.String()).Msg("Failed to reschedule post")
		http.Error(w, "Failed to reschedule post", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(post); err != nil {
		log.Error().Err(err).Msg("Failed to encode post response")
	}
}

// CancelSchedule removes the scheduled publishing of a post (transitions back to DRAFT/QUEUED).
func (h *PostHandler) CancelSchedule(w http.ResponseWriter, r *http.Request) {
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

	post, err := h.postService.CancelSchedule(r.Context(), userID, postID)
	if err != nil {
		if errors.Is(err, service.ErrPostNotFound) {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrPostCannotBeCancelled) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("user_id", userID.String()).Str("post_id", postID.String()).Msg("Failed to cancel scheduled post")
		http.Error(w, "Failed to cancel schedule", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(post); err != nil {
		log.Error().Err(err).Msg("Failed to encode post response")
	}
}

// ListCalendarPosts returns posts within a date range for the calendar view.
func (h *PostHandler) ListCalendarPosts(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	fromStr := q.Get("from")
	toStr := q.Get("to")

	if fromStr == "" || toStr == "" {
		http.Error(w, "from and to query params are required (RFC3339)", http.StatusBadRequest)
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid from: %v", err), http.StatusBadRequest)
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid to: %v", err), http.StatusBadRequest)
		return
	}

	posts, err := h.postService.ListCalendarPosts(r.Context(), userID, from, to, q.Get("status"), q.Get("platform"))
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.String()).Msg("Failed to list calendar posts")
		http.Error(w, "Failed to list calendar posts", http.StatusInternalServerError)
		return
	}
	if posts == nil {
		posts = []model.PostWithDetails{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(posts); err != nil {
		log.Error().Err(err).Msg("Failed to encode calendar posts")
	}
}

// ListUpcomingPosts returns upcoming scheduled posts for the sidebar/feed.
func (h *PostHandler) ListUpcomingPosts(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	limit := 20
	offset := 0
	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if s := q.Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}

	posts, total, err := h.postService.ListUpcomingPosts(r.Context(), userID, limit, offset)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.String()).Msg("Failed to list upcoming posts")
		http.Error(w, "Failed to list upcoming posts", http.StatusInternalServerError)
		return
	}
	if posts == nil {
		posts = []model.PostWithDetails{}
	}

	resp := ListPostsResponse{
		Posts:  posts,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("Failed to encode upcoming posts")
	}
}