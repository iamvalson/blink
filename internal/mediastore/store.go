package mediastore

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound            = errors.New("media object not found")
	ErrPresignNotSupported = errors.New("presigned uploads not supported by this storage driver")
)

// PresignedUpload contains direct upload instructions for the client.
type PresignedUpload struct {
	URL       string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ObjectInfo contains metadata for an object in storage.
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
}

// Store provides an abstraction for durable media storage.
type Store interface {
	// Put streams content from reader into storage at key.
	// Returns bytes written or error.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (int64, error)

	// Get retrieves content for key. Caller must close the returned ReadCloser.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete deletes object at key.
	Delete(ctx context.Context, key string) error

	// Head retrieves metadata about the object at key.
	Head(ctx context.Context, key string) (*ObjectInfo, error)

	// PresignPut generates a presigned URL for direct client upload.
	PresignPut(ctx context.Context, key string, contentType string, size int64, ttl time.Duration) (*PresignedUpload, error)
}
