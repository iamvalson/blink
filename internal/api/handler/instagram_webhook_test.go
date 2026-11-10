package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstagramWebhookVerification(t *testing.T) {
	h := InstagramWebhook{VerifyToken: "sample-token"}
	for _, tt := range []struct {
		query  string
		status int
	}{
		{"hub.mode=subscribe&hub.verify_token=sample-token&hub.challenge=00123", 200},
		{"hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=00123", 403},
		{"hub.mode=subscribe&hub.verify_token=sample-token", 400},
	} {
		r := httptest.NewRequest(http.MethodGet, "/webhooks/instagram?"+tt.query, nil)
		w := httptest.NewRecorder()
		h.Verify(w, r)
		if w.Code != tt.status || (w.Code == 200 && w.Body.String() != "00123") {
			t.Fatalf("unexpected response %d %q", w.Code, w.Body.String())
		}
	}
}

func TestInstagramWebhookRejectsTamperedEvent(t *testing.T) {
	h := InstagramWebhook{AppSecret: "sample-secret"}
	payload := `{"object":"instagram","entry":[]}`
	mac := hmac.New(sha256.New, []byte(h.AppSecret))
	_, _ = mac.Write([]byte(payload))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	for _, tt := range []struct {
		body   string
		status int
	}{{payload, 200}, {payload + " ", 401}} {
		r := httptest.NewRequest(http.MethodPost, "/webhooks/instagram", strings.NewReader(tt.body))
		r.Header.Set("X-Hub-Signature-256", signature)
		w := httptest.NewRecorder()
		h.Receive(w, r)
		if w.Code != tt.status {
			t.Fatalf("got %d, want %d", w.Code, tt.status)
		}
	}
}
