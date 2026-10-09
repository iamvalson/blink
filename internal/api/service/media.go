package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

const (
	MaxVideoSize = 2 * 1024 * 1024 * 1024 // 2 GiB
	MaxImageSize = 50 * 1024 * 1024       // 50 MiB
)

var allowedContentTypes = map[string]bool{
	"video/mp4":       true,
	"video/quicktime": true,
	"video/webm":      true,
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
}

var (
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	ErrFileTooLarge         = errors.New("file exceeds maximum allowed size")
	ErrEmptyFile            = errors.New("file is empty")
	ErrMediaNotFound        = storage.ErrMediaNotFound
	ErrMediaIncomplete      = errors.New("media upload is not complete")
	ErrMediaUnauthorized    = errors.New("media does not belong to this user")
)

type MediaService struct {
	media *storage.MediaRepository
	store mediastore.Store
}

func NewMediaService(media *storage.MediaRepository, store mediastore.Store) *MediaService {
	return &MediaService{
		media: media,
		store: store,
	}
}

type UploadInput struct {
	UserID           uuid.UUID
	OriginalFilename string
	ContentType      string
	Size             int64
	Body             io.Reader
}

func (s *MediaService) Upload(ctx context.Context, input UploadInput) (*model.Media, error) {
	cleanType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if !allowedContentTypes[cleanType] {
		return nil, ErrUnsupportedMediaType
	}

	maxSize := int64(MaxVideoSize)
	if strings.HasPrefix(cleanType, "image/") {
		maxSize = MaxImageSize
	}
	if input.Size > maxSize {
		return nil, ErrFileTooLarge
	}
	if input.Size == 0 {
		return nil, ErrEmptyFile
	}

	cleanName := filepath.Base(input.OriginalFilename)
	if cleanName == "." || cleanName == "" {
		cleanName = "upload"
	}

	ext := strings.ToLower(filepath.Ext(cleanName))
	if ext == "" {
		exts, _ := mime.ExtensionsByType(cleanType)
		if len(exts) > 0 {
			ext = exts[0]
		}
	}

	objectID := uuid.New()
	storageKey := fmt.Sprintf("%s/%s%s", input.UserID, objectID, ext)

	record, err := s.media.CreatePendingMedia(
		ctx,
		input.UserID,
		storageKey,
		cleanName,
		cleanType,
		input.Size,
	)
	if err != nil {
		return nil, fmt.Errorf("create pending media record: %w", err)
	}

	n, err := s.store.Put(ctx, storageKey, input.Body, cleanType)
	if err != nil {
		_ = s.media.MarkFailed(ctx, record.ID)
		return nil, fmt.Errorf("store media: %w", err)
	}

	if n == 0 {
		_ = s.media.MarkFailed(ctx, record.ID)
		_ = s.store.Delete(ctx, storageKey)
		return nil, ErrEmptyFile
	}

	if err := s.media.MarkComplete(ctx, record.ID, n); err != nil {
		return nil, fmt.Errorf("mark media complete: %w", err)
	}

	record.UploadStatus = "complete"
	record.FileSize = n
	return record, nil
}

func (s *MediaService) GetMedia(ctx context.Context, mediaID, userID uuid.UUID) (*model.Media, error) {
	return s.media.GetMedia(ctx, mediaID, userID)
}

