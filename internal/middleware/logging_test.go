package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestRequestLoggerDoesNotRecordSecrets(t *testing.T) {
	var output bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })
	req := httptest.NewRequest(http.MethodGet, "/auth/youtube/callback?code=secret-code&state=secret-state&error_description=secret-error", strings.NewReader("secret-body"))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "auth_token=secret-cookie")
	rec := httptest.NewRecorder()
	RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	})).ServeHTTP(rec, req)
	for _, secret := range []string{"secret-code", "secret-state", "secret-error", "secret-body", "secret-token", "secret-cookie", "?"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("request logger leaked %q", secret)
		}
	}
	decoder := json.NewDecoder(&output)
	for i := 0; i < 2; i++ {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry["path"] != "/auth/youtube/callback" {
			t.Fatalf("unexpected path: %v", entry["path"])
		}
		if i == 1 && (entry["status"] != float64(202) || entry["bytes"] != float64(2)) {
			t.Fatalf("missing response details: %v", entry)
		}
	}
}

func TestRequestLoggerPreservesStreaming(t *testing.T) {
	rec := httptest.NewRecorder()
	RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: ok\n\n"))
		w.(http.Flusher).Flush()
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if !rec.Flushed {
		t.Fatal("logging wrapper broke streaming")
	}
}
