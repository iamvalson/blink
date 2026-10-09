package handler_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/pashagolub/pgxmock/v4"
)

func TestMediaHandler_Unauthenticated(t *testing.T) {
	h := handler.NewMediaHandler(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", nil)
	w := httptest.NewRecorder()

	h.Upload(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestMediaHandler_UnsupportedMimeType(t *testing.T) {
	mockDB, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("create pgxmock: %v", err)
	}
	defer mockDB.Close()

	store, err := mediastore.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("create local store: %v", err)
	}

	mediaRepo := storage.NewMediaRepository(mockDB)
	mediaSvc := service.NewMediaService(mediaRepo, store)
	h := handler.NewMediaHandler(mediaSvc)

	userID := uuid.New()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.exe")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write([]byte("not a video"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))

	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 StatusUnsupportedMediaType, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaHandler_ValidUpload(t *testing.T) {
	mockDB, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("create pgxmock: %v", err)
	}
	defer mockDB.Close()

	store, err := mediastore.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("create local store: %v", err)
	}

	mediaRepo := storage.NewMediaRepository(mockDB)
	mediaSvc := service.NewMediaService(mediaRepo, store)
	h := handler.NewMediaHandler(mediaSvc)

	userID := uuid.New()
	mediaID := uuid.New()
	now := time.Now()

	mockDB.ExpectQuery(`INSERT INTO media`).
		WithArgs(userID, pgxmock.AnyArg(), "video.mp4", "video/mp4", int64(12)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).
			AddRow(mediaID, userID, userID.String()+"/video.mp4", "video.mp4", "video/mp4", int64(12), "pending", now, now))

	mockDB.ExpectExec(`UPDATE media SET upload_status = 'complete'`).
		WithArgs(mediaID, int64(12), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	hPart := make(map[string][]string)
	hPart["Content-Disposition"] = []string{`form-data; name="file"; filename="video.mp4"`}
	hPart["Content-Type"] = []string{"video/mp4"}
	part, err := writer.CreatePart(hPart)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write([]byte("dummy video!"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))

	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), mediaID.String()) {
		t.Fatalf("expected response to contain media id, got %s", w.Body.String())
	}
}

