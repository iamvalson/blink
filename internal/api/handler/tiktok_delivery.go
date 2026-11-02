package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/model"
)

type deliveryMediaRepository interface {
	GetMediaByID(context.Context, uuid.UUID) (*model.Media, error)
}
type deliveryStore interface {
	Get(context.Context, string) (io.ReadCloser, error)
}

// Delivery bypasses request-URI loggers. The signed URL itself is the credential.
type TikTokDeliveryHandler struct {
	signer *mediastore.DeliverySigner
	repo   deliveryMediaRepository
	store  deliveryStore
	slots  chan struct{}
}

func NewTikTokDeliveryHandler(s *mediastore.DeliverySigner, r deliveryMediaRepository, store deliveryStore) *TikTokDeliveryHandler {
	return &TikTokDeliveryHandler{s, r, store, make(chan struct{}, 8)}
}
func (h *TikTokDeliveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", 405)
		return
	}
	id, err := h.signer.Verify(r.URL, r.Host, time.Now())
	if err != nil {
		http.Error(w, "invalid or expired authorization", 403)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-r.Context().Done():
		return
	}
	media, err := h.repo.GetMediaByID(r.Context(), id)
	if err != nil || media.UploadStatus != "complete" || !strings.HasPrefix(media.ContentType, "video/") || media.FileSize <= 0 {
		http.NotFound(w, r)
		return
	}
	start, end, status := int64(0), media.FileSize-1, http.StatusOK
	if value := r.Header.Get("Range"); value != "" {
		var valid bool
		start, end, valid = deliveryRange(value, media.FileSize)
		if !valid {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", media.FileSize))
			w.WriteHeader(416)
			return
		}
		status = 206
	}
	var body io.ReadCloser
	if r.Method == http.MethodGet {
		body, err = h.store.Get(r.Context(), media.StorageKey)
		if err != nil {
			http.Error(w, "media temporarily unavailable", 503)
			return
		}
		defer body.Close()
		// Bounded-memory fallback for Store, which intentionally has no Range API.
		if start > 0 {
			if _, err = io.CopyN(io.Discard, body, start); err != nil {
				http.Error(w, "media temporarily unavailable", 503)
				return
			}
		}
	}
	w.Header().Set("Content-Type", media.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	if status == 206 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, media.FileSize))
	}
	w.WriteHeader(status)
	if body != nil {
		_, _ = io.CopyN(w, body, end-start+1)
	}
}
func deliveryRange(value string, size int64) (int64, int64, bool) {
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	if parts[0] == "" {
		n, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	}
	start, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if parts[1] != "" {
		end, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
}
