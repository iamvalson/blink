package handler_test

import (
	"context"
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
	"github.com/iamvalson/blink/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestMediaContentPreview(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, byteRange, uploadStatus string
		authenticated, owner                       bool
		status                                     int
		body                                       string
	}{
		{"video", "video/mp4", "", "complete", true, true, 200, "0123456789"},
		{"image", "image/png", "", "complete", true, true, 200, "0123456789"},
		{"seek video", "video/mp4", "bytes=3-6", "complete", true, true, 206, "3456"},
		{"suffix range", "video/mp4", "bytes=-3", "complete", true, true, 206, "789"},
		{"invalid range", "video/mp4", "bytes=99-", "complete", true, true, 416, ""},
		{"unauthenticated", "video/mp4", "", "complete", false, true, 401, ""},
		{"another owner", "video/mp4", "", "complete", true, false, 404, ""},
		{"unfinished upload", "video/mp4", "", "pending", true, true, 404, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, err := mediastore.NewLocalStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Put(context.Background(), "preview", strings.NewReader("0123456789"), tc.contentType)
			if err != nil {
				t.Fatal(err)
			}
			userID, mediaID := uuid.New(), uuid.New()
			if tc.authenticated {
				expected := db.ExpectQuery(`SELECT .* FROM media WHERE id = \$1 AND user_id = \$2`).WithArgs(mediaID, userID)
				if tc.owner {
					now := time.Now()
					expected.WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "storage_provider", "storage_key", "original_filename", "content_type", "file_size", "upload_status", "created_at", "updated_at"}).AddRow(mediaID, userID, "local", "preview", "preview.mp4", tc.contentType, int64(10), tc.uploadStatus, now, now))
				} else {
					expected.WillReturnError(pgx.ErrNoRows)
				}
			}
			h := handler.NewMediaHandler(service.NewMediaService(storage.NewMediaRepository(db), store))
			router := chi.NewRouter()
			router.Get("/api/v1/media/{id}/content", h.GetMediaContent)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID.String()+"/content", nil)
			if tc.authenticated {
				req = req.WithContext(middleware.ContextWithUserID(req.Context(), userID.String()))
			}
			if tc.byteRange != "" {
				req.Header.Set("Range", tc.byteRange)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
			if tc.body != "" && response.Body.String() != tc.body {
				t.Fatalf("body=%q want=%q", response.Body.String(), tc.body)
			}
			if tc.status == 200 || tc.status == 206 {
				if response.Header().Get("Content-Type") != tc.contentType || response.Header().Get("Accept-Ranges") != "bytes" {
					t.Fatalf("missing preview headers: %v", response.Header())
				}
				if response.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatal("preview must remain private")
				}
			}
			if tc.status == 206 && response.Header().Get("Content-Range") == "" {
				t.Fatal("missing Content-Range")
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
