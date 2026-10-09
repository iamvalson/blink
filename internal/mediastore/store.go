package mediastore

import (
	"context"
	"io"
)

// Store provides an abstraction for durable media storage.
type Store interface {
	// Put streams content from reader into storage at key.
	// Returns bytes written or error.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (int64, error)

	// Get retrieves content for key. Caller must close the returned ReadCloser.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete deletes object at key.
	Delete(ctx context.Context, key string) error
}

