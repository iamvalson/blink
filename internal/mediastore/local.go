package mediastore

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"time"
)

// LocalStore implements Store on the local filesystem.
type LocalStore struct {
	basePath string
}

// NewLocalStore initializes LocalStore with a base directory.
func NewLocalStore(basePath string) (*LocalStore, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("create media storage directory %q: %w", basePath, err)
	}
	return &LocalStore{basePath: basePath}, nil
}

// Put writes data from r to the destination path on disk.
func (s *LocalStore) Put(_ context.Context, key string, r io.Reader, _ string) (int64, error) {
	dst := filepath.Join(s.basePath, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, fmt.Errorf("create parent directory for %q: %w", key, err)
	}

	f, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("create file %q: %w", key, err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		_ = os.Remove(dst)
		return 0, fmt.Errorf("write file %q: %w", key, err)
	}

	return n, nil
}

// Get opens file for reading.
func (s *LocalStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.basePath, filepath.FromSlash(key))
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("open media %q: %w", key, err)
	}
	return f, nil
}

// Delete removes file from disk.
func (s *LocalStore) Delete(_ context.Context, key string) error {
	path := filepath.Join(s.basePath, filepath.FromSlash(key))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete media %q: %w", key, err)
	}
	return nil
}

// Head checks object existence and returns metadata.
func (s *LocalStore) Head(_ context.Context, key string) (*ObjectInfo, error) {
	path := filepath.Join(s.basePath, filepath.FromSlash(key))
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("stat media %q: %w", key, err)
	}

	ext := filepath.Ext(path)
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}

	return &ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		ContentType:  ct,
		LastModified: fi.ModTime(),
	}, nil
}

// PresignPut generates a simulated presigned upload URL for local development.
func (s *LocalStore) PresignPut(_ context.Context, key string, contentType string, _ int64, ttl time.Duration) (*PresignedUpload, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	headers := make(map[string]string)
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return &PresignedUpload{
		URL:       fmt.Sprintf("/api/v1/media/local-upload?key=%s", key),
		Method:    "PUT",
		Headers:   headers,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}
