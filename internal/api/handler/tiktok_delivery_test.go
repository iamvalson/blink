package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/model"
)

type deliveryFixture struct {
	media model.Media
	reads int
}

func (f *deliveryFixture) GetMediaByID(context.Context, uuid.UUID) (*model.Media, error) {
	return &f.media, nil
}
func (f *deliveryFixture) Get(context.Context, string) (io.ReadCloser, error) {
	f.reads++
	return io.NopCloser(strings.NewReader("0123456789")), nil
}
func TestSignedDeliveryStreamsOnlyAuthorizedVideo(t *testing.T) {
	signer, e := mediastore.NewDeliverySigner("https://media.example/tiktok-media", strings.Repeat("k", 32), 2*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.New()
	f := &deliveryFixture{media: model.Media{ID: id, FileSize: 10, ContentType: "video/mp4", UploadStatus: "complete"}}
	h := NewTikTokDeliveryHandler(signer, f, f)
	for _, tc := range []struct {
		method, rng string
		status      int
		body        string
	}{{"GET", "", 200, "0123456789"}, {"HEAD", "", 200, ""}, {"GET", "bytes=2-5", 206, "2345"}, {"GET", "bytes=-3", 206, "789"}, {"GET", "bytes=99-", 416, ""}, {"POST", "", 405, "method not allowed\n"}} {
		req := httptest.NewRequest(tc.method, signer.URL(id, time.Now()), nil)
		req.Header.Set("Range", tc.rng)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status || w.Body.String() != tc.body {
			t.Fatalf("%s %s: %d %q", tc.method, tc.rng, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("cache enabled")
		}
	}
	before := f.reads
	req := httptest.NewRequest("GET", signer.URL(uuid.New(), time.Now()), nil)
	req.URL.Path = mediastore.DeliveryPath + id.String()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 || f.reads != before {
		t.Fatal("unauthorized storage read")
	}
}
