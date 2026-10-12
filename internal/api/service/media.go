package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

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
	ErrMediaInUse           = storage.ErrMediaInUse
)

type MediaConfig struct {
	StorageProvider string
	MaxUploadBytes  int64
	PresignTTL      time.Duration
}

type MediaService struct {
	media *storage.MediaRepository
	store mediastore.Store
	cfg   MediaConfig
}

func NewMediaService(media *storage.MediaRepository, store mediastore.Store, cfgs ...MediaConfig) *MediaService {
	cfg := MediaConfig{
		StorageProvider: "r2",
		MaxUploadBytes:  int64(MaxVideoSize),
		PresignTTL:      15 * time.Minute,
	}
	if len(cfgs) > 0 {
		if cfgs[0].StorageProvider != "" {
			cfg.StorageProvider = cfgs[0].StorageProvider
		}
		if cfgs[0].MaxUploadBytes > 0 {
			cfg.MaxUploadBytes = cfgs[0].MaxUploadBytes
		}
		if cfgs[0].PresignTTL > 0 {
			cfg.PresignTTL = cfgs[0].PresignTTL
		}
	}
	return &MediaService{
		media: media,
		store: store,
		cfg:   cfg,
	}
}

type InitiateUploadInput struct {
	UserID      uuid.UUID
	Filename    string
	ContentType string
	Size        int64
}

// InitiateUpload validates client metadata, creates a pending media record,
// and issues a short-lived presigned PUT URL.
func (s *MediaService) InitiateUpload(ctx context.Context, input InitiateUploadInput) (*model.InitiateUploadResponse, error) {
	if input.UserID == uuid.Nil {
		return nil, ErrMediaUnauthorized
	}

	cleanType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if !allowedContentTypes[cleanType] {
		return nil, ErrUnsupportedMediaType
	}

	maxSize := int64(MaxVideoSize)
	if strings.HasPrefix(cleanType, "image/") {
		maxSize = MaxImageSize
	}
	if s.cfg.MaxUploadBytes > 0 && s.cfg.MaxUploadBytes < maxSize {
		maxSize = s.cfg.MaxUploadBytes
	}

	if input.Size > maxSize {
		return nil, ErrFileTooLarge
	}
	if input.Size <= 0 {
		return nil, ErrEmptyFile
	}

	cleanName := filepath.Base(input.Filename)
	if cleanName == "." || cleanName == "" {
		cleanName = "upload"
	}

	ext := strings.ToLower(filepath.Ext(cleanName))
	if ext == "" {
		exts, _ := mime.ExtensionsByType(cleanType)
		if len(exts) > 0 {
			ext = exts[0]
		} else {
			if cleanType == "video/mp4" {
				ext = ".mp4"
			} else if cleanType == "image/jpeg" {
				ext = ".jpg"
			} else if cleanType == "image/png" {
				ext = ".png"
			}
		}
	}

	objectID := uuid.New()
	storageKey := fmt.Sprintf("media/%s/%s%s", input.UserID, objectID, ext)

	provider := s.cfg.StorageProvider
	if provider == "" {
		provider = "r2"
	}

	record, err := s.media.CreatePendingMedia(
		ctx,
		input.UserID,
		provider,
		storageKey,
		cleanName,
		cleanType,
		input.Size,
	)
	if err != nil {
		return nil, fmt.Errorf("create pending media record: %w", err)
	}

	presigned, err := s.store.PresignPut(ctx, storageKey, cleanType, input.Size, s.cfg.PresignTTL)
	if err != nil {
		_ = s.media.MarkFailed(ctx, record.ID)
		return nil, fmt.Errorf("generate presigned upload url: %w", err)
	}

	return &model.InitiateUploadResponse{
		ID:         record.ID,
		StorageKey: storageKey,
		UploadURL:  presigned.URL,
		Method:     presigned.Method,
		Headers:    presigned.Headers,
		ExpiresAt:  presigned.ExpiresAt,
	}, nil
}

// CompleteUpload verifies that the object was uploaded to storage, validates
// actual size and content signature, and marks the media available.
func (s *MediaService) CompleteUpload(ctx context.Context, userID, mediaID uuid.UUID) (*model.Media, error) {
	record, err := s.media.GetMedia(ctx, mediaID, userID)
	if err != nil {
		return nil, err
	}

	if record.UploadStatus == "complete" {
		return record, nil
	}

	// Verify object exists in storage
	info, err := s.store.Head(ctx, record.StorageKey)
	if err != nil {
		if errors.Is(err, mediastore.ErrNotFound) {
			_ = s.media.MarkFailed(ctx, record.ID)
			return nil, fmt.Errorf("media file not found in storage: %w", err)
		}
		return nil, fmt.Errorf("inspect stored media object: %w", err)
	}

	if info.Size <= 0 {
		_ = s.media.MarkFailed(ctx, record.ID)
		return nil, ErrEmptyFile
	}

	maxSize := int64(MaxVideoSize)
	if strings.HasPrefix(record.ContentType, "image/") {
		maxSize = MaxImageSize
	}
	if s.cfg.MaxUploadBytes > 0 && s.cfg.MaxUploadBytes < maxSize {
		maxSize = s.cfg.MaxUploadBytes
	}
	if info.Size > maxSize {
		_ = s.media.MarkFailed(ctx, record.ID)
		return nil, ErrFileTooLarge
	}

	// Validate content signature by inspecting header bytes
	if err := s.validateFileSignature(ctx, record.StorageKey, record.ContentType); err != nil {
		_ = s.media.MarkFailed(ctx, record.ID)
		return nil, err
	}

	if err := s.media.MarkComplete(ctx, record.ID, info.Size); err != nil {
		return nil, fmt.Errorf("mark media complete: %w", err)
	}

	record.UploadStatus = "complete"
	record.FileSize = info.Size
	return record, nil
}

func (s *MediaService) validateFileSignature(ctx context.Context, key, expectedType string) error {
	rc, err := s.store.Get(ctx, key)
	if err != nil {
		// If streaming read fails, head check was already verified.
		return nil
	}
	defer rc.Close()

	header := make([]byte, 512)
	n, readErr := io.ReadFull(rc, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
		return nil
	}
	if n == 0 {
		return ErrEmptyFile
	}

	detected := http.DetectContentType(header[:n])

	// Executable and script blocks
	if strings.HasPrefix(detected, "application/x-dosexec") ||
		strings.HasPrefix(detected, "text/html") ||
		strings.HasPrefix(detected, "application/x-sh") {
		return fmt.Errorf("%w: prohibited file content (%s)", ErrUnsupportedMediaType, detected)
	}

	// For images, detected type should match image/
	if strings.HasPrefix(expectedType, "image/") {
		if !strings.HasPrefix(detected, "image/") && detected != "application/octet-stream" {
			return fmt.Errorf("%w: expected image but detected %s", ErrUnsupportedMediaType, detected)
		}
	}

	// For MP4 videos, check for standard ftyp box or detected video type
	if expectedType == "video/mp4" {
		if n >= 8 && !bytes.Equal(header[4:8], []byte("ftyp")) && !strings.HasPrefix(detected, "video/") && detected != "application/octet-stream" {
			return fmt.Errorf("%w: invalid MP4 header signature", ErrUnsupportedMediaType)
		}
	}

	return nil
}

// GetMedia retrieves media by ID verifying ownership by userID.
func (s *MediaService) GetMedia(ctx context.Context, mediaID, userID uuid.UUID) (*model.Media, error) {
	return s.media.GetMedia(ctx, mediaID, userID)
}

// DeleteMedia deletes media verifying ownership and post reference constraints.
func (s *MediaService) DeleteMedia(ctx context.Context, mediaID, userID uuid.UUID) error {
	record, err := s.media.GetMedia(ctx, mediaID, userID)
	if err != nil {
		return err
	}

	inUse, err := s.media.IsReferencedByPost(ctx, mediaID)
	if err != nil {
		return err
	}
	if inUse {
		return ErrMediaInUse
	}

	_ = s.store.Delete(ctx, record.StorageKey)
	return s.media.DeleteMedia(ctx, mediaID, userID)
}

type UploadInput struct {
	UserID           uuid.UUID
	OriginalFilename string
	ContentType      string
	Size             int64
	Body             io.Reader
}

// Upload handles legacy direct multipart file upload.
func (s *MediaService) Upload(ctx context.Context, input UploadInput) (*model.Media, error) {
	cleanType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if !allowedContentTypes[cleanType] {
		return nil, ErrUnsupportedMediaType
	}

	maxSize := int64(MaxVideoSize)
	if strings.HasPrefix(cleanType, "image/") {
		maxSize = MaxImageSize
	}
	if s.cfg.MaxUploadBytes > 0 && s.cfg.MaxUploadBytes < maxSize {
		maxSize = s.cfg.MaxUploadBytes
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
	storageKey := fmt.Sprintf("media/%s/%s%s", input.UserID, objectID, ext)

	provider := s.cfg.StorageProvider
	if provider == "" {
		provider = "local"
	}

	record, err := s.media.CreatePendingMedia(
		ctx,
		input.UserID,
		provider,
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
