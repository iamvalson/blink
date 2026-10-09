package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrMediaNotFound = errors.New("media not found")

type mediaDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// MediaRepository manages persistence of uploaded media assets.
type MediaRepository struct {
	db mediaDB
}

func NewMediaRepository(db mediaDB) *MediaRepository {
	return &MediaRepository{db: db}
}

// CreatePendingMedia registers a media record in 'pending' status.
func (r *MediaRepository) CreatePendingMedia(
	ctx context.Context,
	userID uuid.UUID,
	storageKey string,
	originalFilename string,
	contentType string,
	fileSize int64,
) (*model.Media, error) {
	var m model.Media
	err := r.db.QueryRow(ctx, `
		INSERT INTO media (user_id, storage_key, original_filename, content_type, file_size, upload_status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING id, user_id, storage_key, original_filename, content_type, file_size, upload_status, created_at, updated_at
	`, userID, storageKey, originalFilename, contentType, fileSize).Scan(
		&m.ID, &m.UserID, &m.StorageKey, &m.OriginalFilename, &m.ContentType,
		&m.FileSize, &m.UploadStatus, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create pending media: %w", err)
	}
	return &m, nil
}

// MarkComplete sets upload_status to 'complete' and updates final file_size.
func (r *MediaRepository) MarkComplete(ctx context.Context, id uuid.UUID, fileSize int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE media
		SET upload_status = 'complete', file_size = $2, updated_at = $3
		WHERE id = $1
	`, id, fileSize, time.Now())
	if err != nil {
		return fmt.Errorf("mark media complete: %w", err)
	}
	return nil
}

// MarkFailed sets upload_status to 'failed'.
func (r *MediaRepository) MarkFailed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE media
		SET upload_status = 'failed', updated_at = $2
		WHERE id = $1
	`, id, time.Now())
	if err != nil {
		return fmt.Errorf("mark media failed: %w", err)
	}
	return nil
}

// GetMedia retrieves media by ID verifying ownership by userID.
func (r *MediaRepository) GetMedia(ctx context.Context, id, userID uuid.UUID) (*model.Media, error) {
	var m model.Media
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, storage_key, original_filename, content_type, file_size, upload_status, created_at, updated_at
		FROM media
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&m.ID, &m.UserID, &m.StorageKey, &m.OriginalFilename, &m.ContentType,
		&m.FileSize, &m.UploadStatus, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("get media: %w", err)
	}
	return &m, nil
}

// GetMediaByID retrieves media by ID without ownership constraint (used by publish worker).
func (r *MediaRepository) GetMediaByID(ctx context.Context, id uuid.UUID) (*model.Media, error) {
	var m model.Media
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, storage_key, original_filename, content_type, file_size, upload_status, created_at, updated_at
		FROM media
		WHERE id = $1
	`, id).Scan(
		&m.ID, &m.UserID, &m.StorageKey, &m.OriginalFilename, &m.ContentType,
		&m.FileSize, &m.UploadStatus, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("get media by id: %w", err)
	}
	return &m, nil
}

