package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Media represents an uploaded media asset.
type Media struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	StorageProvider  string    `json:"storage_provider"`
	StorageKey       string    `json:"storage_key"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	UploadStatus     string    `json:"upload_status"` // pending, complete, failed
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// MediaUploadResponse is returned to the client after a successful upload or confirmation.
type MediaUploadResponse struct {
	ID           uuid.UUID `json:"id"`
	ContentType  string    `json:"content_type"`
	FileSize     int64     `json:"file_size"`
	Filename     string    `json:"filename"`
	UploadStatus string    `json:"upload_status,omitempty"`
}

// InitiateUploadRequest contains client metadata to authorize a presigned upload.
type InitiateUploadRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

// UnmarshalJSON supports both camelCase and snake_case for contentType/content_type.
func (r *InitiateUploadRequest) UnmarshalJSON(data []byte) error {
	type Alias InitiateUploadRequest
	aux := &struct {
		AltContentType string `json:"content_type"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if r.ContentType == "" && aux.AltContentType != "" {
		r.ContentType = aux.AltContentType
	}
	return nil
}

// InitiateUploadResponse is returned after authorizing a presigned upload.
type InitiateUploadResponse struct {
	ID         uuid.UUID         `json:"id"`
	StorageKey string            `json:"storage_key"`
	UploadURL  string            `json:"upload_url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	ExpiresAt  time.Time         `json:"expires_at"`
}
