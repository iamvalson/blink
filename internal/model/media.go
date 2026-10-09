package model

import (
	"time"

	"github.com/google/uuid"
)

// Media represents an uploaded media asset.
type Media struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	StorageKey       string    `json:"storage_key"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	UploadStatus     string    `json:"upload_status"` // pending, complete, failed
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// MediaUploadResponse is returned to the client after a successful upload.
type MediaUploadResponse struct {
	ID          uuid.UUID `json:"id"`
	ContentType string    `json:"content_type"`
	FileSize    int64     `json:"file_size"`
	Filename    string    `json:"filename"`
}

