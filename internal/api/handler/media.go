package handler

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
)

// maxUploadBytes is 2 GiB + overhead
const maxUploadBytes = 2*1024*1024*1024 + 1024

type MediaHandler struct {
	mediaService *service.MediaService
}

func NewMediaHandler(mediaService *service.MediaService) *MediaHandler {
	return &MediaHandler{
		mediaService: mediaService,
	}
}

// InitiateUpload handles POST /api/v1/media/uploads and /api/media/uploads
func (h *MediaHandler) InitiateUpload(w http.ResponseWriter, r *http.Request) {
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

	var req model.InitiateUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.mediaService.InitiateUpload(r.Context(), service.InitiateUploadInput{
		UserID:      userID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		Size:        req.Size,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUnsupportedMediaType):
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		case errors.Is(err, service.ErrFileTooLarge):
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		case errors.Is(err, service.ErrEmptyFile):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			log.Error().Err(err).Str("user_id", userID.String()).Msg("initiate upload failed")
			http.Error(w, "Failed to initiate upload", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("failed to encode initiate upload response")
	}
}

// CompleteUpload handles POST /api/v1/media/{id}/complete and /api/media/{id}/complete
func (h *MediaHandler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
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

	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		idParam = chi.URLParam(r, "mediaID")
	}
	mediaID, err := uuid.Parse(idParam)
	if err != nil {
		http.Error(w, "Invalid media ID", http.StatusBadRequest)
		return
	}

	record, err := h.mediaService.CompleteUpload(r.Context(), userID, mediaID)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrMediaNotFound):
			http.Error(w, "Media not found", http.StatusNotFound)
		case errors.Is(err, service.ErrMediaUnauthorized), errors.Is(err, storage.ErrMediaForbidden):
			http.Error(w, "Access to media forbidden", http.StatusForbidden)
		case errors.Is(err, service.ErrFileTooLarge):
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		case errors.Is(err, service.ErrEmptyFile):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, service.ErrUnsupportedMediaType):
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		default:
			log.Error().Err(err).Str("user_id", userID.String()).Str("media_id", mediaID.String()).Msg("complete upload failed")
			http.Error(w, "Upload confirmation failed: "+err.Error(), http.StatusBadRequest)
		}
		return
	}

	resp := model.MediaUploadResponse{
		ID:           record.ID,
		ContentType:  record.ContentType,
		FileSize:     record.FileSize,
		Filename:     record.OriginalFilename,
		UploadStatus: record.UploadStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("failed to encode complete upload response")
	}
}

// GetMedia handles GET /api/v1/media/{id} and /api/media/{id}
func (h *MediaHandler) GetMedia(w http.ResponseWriter, r *http.Request) {
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

	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		idParam = chi.URLParam(r, "mediaID")
	}
	mediaID, err := uuid.Parse(idParam)
	if err != nil {
		http.Error(w, "Invalid media ID", http.StatusBadRequest)
		return
	}

	record, err := h.mediaService.GetMedia(r.Context(), mediaID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrMediaNotFound) {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get media", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(record); err != nil {
		log.Error().Err(err).Msg("failed to encode media response")
	}
}

// DeleteMedia handles DELETE /api/v1/media/{id} and /api/media/{id}
func (h *MediaHandler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
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

	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		idParam = chi.URLParam(r, "mediaID")
	}
	mediaID, err := uuid.Parse(idParam)
	if err != nil {
		http.Error(w, "Invalid media ID", http.StatusBadRequest)
		return
	}

	if err := h.mediaService.DeleteMedia(r.Context(), mediaID, userID); err != nil {
		switch {
		case errors.Is(err, storage.ErrMediaNotFound):
			http.Error(w, "Media not found", http.StatusNotFound)
		case errors.Is(err, storage.ErrMediaInUse):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			log.Error().Err(err).Str("user_id", userID.String()).Str("media_id", mediaID.String()).Msg("delete media failed")
			http.Error(w, "Failed to delete media", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Upload handles legacy POST /api/v1/media (and /api/media)
func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 * 1024); err != nil {
		http.Error(w, "Invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	if !strings.HasPrefix(mediaType, "video/") && !strings.HasPrefix(mediaType, "image/") {
		http.Error(w, "Only image and video files are accepted", http.StatusUnsupportedMediaType)
		return
	}

	declaredSize := header.Size
	if declaredSize <= 0 {
		if cl := header.Header.Get("Content-Length"); cl != "" {
			if n, parseErr := strconv.ParseInt(cl, 10, 64); parseErr == nil {
				declaredSize = n
			}
		}
	}

	record, err := h.mediaService.Upload(r.Context(), service.UploadInput{
		UserID:           userID,
		OriginalFilename: header.Filename,
		ContentType:      mediaType,
		Size:             declaredSize,
		Body:             file,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUnsupportedMediaType):
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		case errors.Is(err, service.ErrFileTooLarge):
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		case errors.Is(err, service.ErrEmptyFile):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			log.Error().Err(err).Str("user_id", userID.String()).Msg("media upload failed")
			http.Error(w, "Upload failed", http.StatusInternalServerError)
		}
		return
	}

	resp := model.MediaUploadResponse{
		ID:           record.ID,
		ContentType:  record.ContentType,
		FileSize:     record.FileSize,
		Filename:     record.OriginalFilename,
		UploadStatus: record.UploadStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("failed to encode media upload response")
	}
}
