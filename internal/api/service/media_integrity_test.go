package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/pashagolub/pgxmock/v4"
)

type integrityStore struct {
	mediastore.Store
	body    []byte
	readErr error
	getErr  error
	deleted bool
}

func (s *integrityStore) Put(_ context.Context, _ string, r io.Reader, _ string) (int64, error) {
	b, err := io.ReadAll(r)
	s.body = b
	return int64(len(b)), err
}
func (s *integrityStore) Delete(context.Context, string) error { s.deleted = true; return nil }
func (s *integrityStore) Get(context.Context, string) (io.ReadCloser, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.readErr != nil {
		return io.NopCloser(errorReader{s.readErr}), nil
	}
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestSignatureReadFailuresAreRejected(t *testing.T) {
	boom := errors.New("storage unavailable")
	for _, store := range []*integrityStore{{getErr: boom}, {readErr: boom}} {
		svc := NewMediaService(nil, store)
		if err := svc.validateFileSignature(context.Background(), "key", "video/mp4"); !errors.Is(err, boom) {
			t.Fatalf("read failure accepted: %v", err)
		}
	}
}

func TestDirectUploadChecksActualSizeAndContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   []byte
		getErr error
		want   error
	}{
		{"oversized actual body", bytes.Repeat([]byte{'a'}, 100), nil, ErrFileTooLarge},
		{"invalid signature", []byte("<html>bad</html>"), nil, ErrUnsupportedMediaType},
		{"unreadable stored object", []byte("\x00\x00\x00\x0cftypisom"), errors.New("storage unavailable"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			user, id := uuid.New(), uuid.New()
			now := time.Now()
			store := &integrityStore{getErr: tc.getErr}
			db.ExpectQuery("INSERT INTO media").WithArgs(user, pgxmock.AnyArg(), pgxmock.AnyArg(), "test.mp4", "video/mp4", int64(1)).WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_provider", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).AddRow(id, user, "local", "key", "test.mp4", "video/mp4", int64(1), "pending", now, now))
			db.ExpectExec("UPDATE media SET upload_status = 'failed'").WithArgs(id, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			svc := NewMediaService(storage.NewMediaRepository(db), store, MediaConfig{MaxUploadBytes: 32})
			_, err = svc.Upload(context.Background(), UploadInput{UserID: user, OriginalFilename: "test.mp4", ContentType: "video/mp4", Size: 1, Body: bytes.NewReader(tc.body)})
			want := tc.want
			if want == nil {
				want = tc.getErr
			}
			if !errors.Is(err, want) {
				t.Fatalf("upload error=%v want=%v", err, want)
			}
			if !store.deleted {
				t.Fatal("rejected object was not removed")
			}
			if len(store.body) > 33 {
				t.Fatal("upload did not limit streamed bytes")
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
