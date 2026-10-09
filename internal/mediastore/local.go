package mediastore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
			return nil, fmt.Errorf("media not found: %q", key)
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

