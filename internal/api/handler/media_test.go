package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/model"
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
		WithArgs(userID, pgxmock.AnyArg(), pgxmock.AnyArg(), "video.mp4", "video/mp4", int64(12)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_provider", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).
			AddRow(mediaID, userID, "local", userID.String()+"/video.mp4", "video.mp4", "video/mp4", int64(12), "pending", now, now))

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
	_, _ = part.Write([]byte("\x00\x00\x00\x0cftypisom"))
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

func TestMediaHandler_InitiateUpload(t *testing.T) {
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

	t.Run("unauthenticated rejected", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"filename":"video.mp4","contentType":"video/mp4","size":100}`))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads", body)
		w := httptest.NewRecorder()
		h.InitiateUpload(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("invalid content type rejected", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"filename":"app.exe","contentType":"application/x-msdownload","size":100}`))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads", body)
		req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))
		w := httptest.NewRecorder()
		h.InitiateUpload(w, req)
		if w.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", w.Code)
		}
	})

	t.Run("oversized upload rejected", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"filename":"huge.png","contentType":"image/png","size":60000000}`))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads", body)
		req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))
		w := httptest.NewRecorder()
		h.InitiateUpload(w, req)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413, got %d", w.Code)
		}
	})

	t.Run("valid initiation returns presigned url and media id", func(t *testing.T) {
		mockDB.ExpectQuery(`INSERT INTO media`).
			WithArgs(userID, pgxmock.AnyArg(), pgxmock.AnyArg(), "video.mp4", "video/mp4", int64(10485760)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_provider", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).
				AddRow(mediaID, userID, "local", "media/"+userID.String()+"/video.mp4", "video.mp4", "video/mp4", int64(10485760), "pending", now, now))

		body := bytes.NewReader([]byte(`{"filename":"video.mp4","contentType":"video/mp4","size":10485760}`))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/uploads", body)
		req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))
		w := httptest.NewRecorder()
		h.InitiateUpload(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var resp model.InitiateUploadResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.ID != mediaID {
			t.Fatalf("expected media id %s, got %s", mediaID, resp.ID)
		}
		if resp.UploadURL == "" {
			t.Fatal("expected non-empty upload_url")
		}
	})
}

func TestMediaHandler_CompleteUpload(t *testing.T) {
	mockDB, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("create pgxmock: %v", err)
	}
	defer mockDB.Close()

	tempDir := t.TempDir()
	store, err := mediastore.NewLocalStore(tempDir)
	if err != nil {
		t.Fatalf("create local store: %v", err)
	}

	mediaRepo := storage.NewMediaRepository(mockDB)
	mediaSvc := service.NewMediaService(mediaRepo, store)
	h := handler.NewMediaHandler(mediaSvc)

	userID := uuid.New()
	mediaID := uuid.New()
	storageKey := "media/" + userID.String() + "/video.mp4"
	now := time.Now()

	// Write mock media file so Head and signature check succeed
	mockVideoBytes := append([]byte{0x00, 0x00, 0x00, 0x18}, []byte("ftypisom")...)
	mockVideoBytes = append(mockVideoBytes, bytes.Repeat([]byte{0x00}, 100)...)
	_, _ = store.Put(context.Background(), storageKey, bytes.NewReader(mockVideoBytes), "video/mp4")

	mockDB.ExpectQuery(`SELECT id, user_id, storage_provider, storage_key, original_filename, content_type, file_size, upload_status, created_at, updated_at FROM media WHERE id = \$1 AND user_id = \$2`).
		WithArgs(mediaID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_provider", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).
			AddRow(mediaID, userID, "local", storageKey, "video.mp4", "video/mp4", int64(100), "pending", now, now))

	mockDB.ExpectExec(`UPDATE media SET upload_status = 'complete'`).
		WithArgs(mediaID, int64(len(mockVideoBytes)), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	r := chi.NewRouter()
	r.Post("/api/v1/media/{id}/complete", h.CompleteUpload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+mediaID.String()+"/complete", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.MediaUploadResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != mediaID {
		t.Fatalf("expected id %s, got %s", mediaID, resp.ID)
	}
	if resp.UploadStatus != "complete" {
		t.Fatalf("expected upload_status complete, got %s", resp.UploadStatus)
	}

	t.Run("another user's media cannot be completed", func(t *testing.T) {
		otherUser := uuid.New()
		reqOther := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+mediaID.String()+"/complete", nil)
		reqOther = reqOther.WithContext(middleware.ContextWithUserID(reqOther.Context(), otherUser.String()))
		wOther := httptest.NewRecorder()

		mockDB.ExpectQuery(`SELECT id, user_id, storage_provider, storage_key, original_filename, content_type, file_size, upload_status, created_at, updated_at FROM media WHERE id = \$1 AND user_id = \$2`).
			WithArgs(mediaID, otherUser).
			WillReturnError(storage.ErrMediaNotFound)

		r.ServeHTTP(wOther, reqOther)
		if wOther.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for another user's media, got %d", wOther.Code)
		}
	})

	t.Run("upload completion verifies object existence", func(t *testing.T) {
		missingMediaID := uuid.New()
		missingKey := "media/" + userID.String() + "/missing.mp4"

		mockDB.ExpectQuery(`SELECT id, user_id, storage_provider, storage_key, original_filename, content_type, file_size, upload_status, created_at, updated_at FROM media WHERE id = \$1 AND user_id = \$2`).
			WithArgs(missingMediaID, userID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_provider", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).
				AddRow(missingMediaID, userID, "local", missingKey, "missing.mp4", "video/mp4", int64(100), "pending", now, now))

		mockDB.ExpectExec(`UPDATE media SET upload_status = 'failed'`).
			WithArgs(missingMediaID, pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		reqMissing := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+missingMediaID.String()+"/complete", nil)
		reqMissing = reqMissing.WithContext(middleware.ContextWithUserID(reqMissing.Context(), userID.String()))
		wMissing := httptest.NewRecorder()

		r.ServeHTTP(wMissing, reqMissing)
		if wMissing.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request when object missing in storage, got %d", wMissing.Code)
		}
	})
}
