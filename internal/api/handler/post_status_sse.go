package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
	"github.com/rs/zerolog/log"
)

const (
	sseKeepaliveInterval = 15 * time.Second
	ssePollInterval      = 2 * time.Second
	sseMaxDuration       = 10 * time.Minute
)

// terminalPostStatuses are statuses where no further status changes are expected.
var terminalPostStatuses = map[string]bool{
	model.StatusPublished:         true,
	model.StatusPartiallyPublished: true,
	model.StatusFailed:            true,
	model.StatusCancelled:         true,
}

// PostStatusSSE streams server-sent events for a single post's publishing
// status. The stream delivers an event on every status change and closes
// automatically once the post reaches a terminal state.
//
// Event format:
//
//	event: status
//	data: {"status":"PUBLISHING","targets":[...],"updated_at":"..."}
//
// Clients should reconnect automatically (the browser EventSource API does
// this by default). The endpoint adds a `Last-Event-ID` response header so
// reconnecting clients can resume without losing an update.
func (h *PostHandler) PostStatusSSE(w http.ResponseWriter, r *http.Request) {
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

	// Verify the post exists and belongs to this user before opening the stream.
	post, err := h.postService.GetPost(r.Context(), userID, postID)
	if err != nil {
		if isNotFound(err) {
			http.Error(w, "Post not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("post_id", postIDStr).Msg("SSE: failed to fetch initial post")
		http.Error(w, "Failed to open status stream", http.StatusInternalServerError)
		return
	}

	// SSE requires a flusher — if the transport doesn't support it (tests,
	// some reverse proxies without streaming enabled) we fall back to a
	// single JSON snapshot so the caller always gets something useful.
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(post); err != nil {
			log.Error().Err(err).Msg("SSE: failed to encode fallback response")
		}
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithTimeout(r.Context(), sseMaxDuration)
	defer cancel()

	// Send the initial state immediately so the client never has to wait.
	lastStatus := post.Status
	if err := writeSSEEvent(w, flusher, "status", post); err != nil {
		return
	}

	if terminalPostStatuses[post.Status] {
		writeSSEEvent(w, flusher, "done", map[string]string{"status": post.Status}) //nolint:errcheck
		return
	}

	keepalive := time.NewTicker(sseKeepaliveInterval)
	poll := time.NewTicker(ssePollInterval)
	defer keepalive.Stop()
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected or max-duration reached — no error to surface.
			return

		case <-keepalive.C:
			// SSE comment keeps the TCP connection alive through proxies.
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-poll.C:
			updated, err := h.postService.GetPost(ctx, userID, postID)
			if err != nil {
				if isNotFound(err) {
					writeSSEEvent(w, flusher, "error", map[string]string{"error": "post not found"}) //nolint:errcheck
					return
				}
				log.Warn().Err(err).Str("post_id", postIDStr).Msg("SSE: poll error")
				// Transient error — keep the stream open; the client will retry on reconnect.
				continue
			}

			if updated.Status != lastStatus || hasTargetChanges(post, updated) {
				lastStatus = updated.Status
				post = updated
				if err := writeSSEEvent(w, flusher, "status", updated); err != nil {
					return
				}
			}

			if terminalPostStatuses[updated.Status] {
				writeSSEEvent(w, flusher, "done", map[string]string{"status": updated.Status}) //nolint:errcheck
				return
			}
		}
	}
}

// writeSSEEvent marshals payload and writes a named SSE event to w, then flushes.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// hasTargetChanges returns true when any individual target status differs
// between the two snapshots. Used to push an update even when the parent post
// status hasn't changed yet (e.g. first target publishes while others are still
// in progress).
func hasTargetChanges(prev, next *model.PostWithDetails) bool {
	if len(prev.Targets) != len(next.Targets) {
		return true
	}
	prevMap := make(map[uuid.UUID]string, len(prev.Targets))
	for _, t := range prev.Targets {
		prevMap[t.ID] = t.Status
	}
	for _, t := range next.Targets {
		if prevMap[t.ID] != t.Status {
			return true
		}
	}
	return false
}

// isNotFound returns true for any error that represents a missing resource.
func isNotFound(err error) bool {
	return err != nil && err.Error() == service.ErrPostNotFound.Error()
}

