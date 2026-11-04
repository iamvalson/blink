package api

import (
	"bytes"
	"context"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/metrics"
)

func TestMetricsEndpoint(t *testing.T) {
	metrics.JobProcessed(metrics.PlatformTwitter)
	metrics.ObservePublishDuration(metrics.PlatformTwitter, 0.01)
	router := NewRouter(nil, nil, nil, "", nil, "", nil, nil, nil, nil, nil, false, true, http.SameSiteNoneMode, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	for _, name := range []string{
		"blink_jobs_processed_total",
		"blink_publish_duration_seconds",
		"blink_worker_jobs_in_progress",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics response does not contain %q", name)
		}
	}
}

func TestCORSOptionsPreflight(t *testing.T) {
	origins := []string{"http://localhost:3000"}
	router := NewRouter(nil, nil, nil, "", nil, "", nil, nil, nil, nil, origins, false, true, http.SameSiteNoneMode, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK && resp.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 200 or 204, got %d", resp.Code)
	}

	allowOrigin := resp.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin to be 'http://localhost:3000', got %q", allowOrigin)
	}

	allowCredentials := resp.Header().Get("Access-Control-Allow-Credentials")
	if allowCredentials != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials to be 'true', got %q", allowCredentials)
	}
}

func TestCORSOptionsPreflightIncludesLastEventID(t *testing.T) {
	origins := []string{"http://localhost:3000"}
	router := NewRouter(nil, nil, nil, "", nil, "", nil, nil, nil, nil, origins, false, true, http.SameSiteNoneMode, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/posts/123/status", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Last-Event-ID")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK && resp.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 200 or 204, got %d", resp.Code)
	}
}

func TestCORSOptionsPreflightIncludesNgrokSkipBrowserWarning(t *testing.T) {
	origins := []string{"http://localhost:3000"}
	router := NewRouter(nil, nil, nil, "", nil, "", nil, nil, nil, nil, origins, false, true, http.SameSiteNoneMode, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/auth/me", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, ngrok-skip-browser-warning")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK && resp.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 200 or 204, got %d", resp.Code)
	}

	allowOrigin := resp.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin to be 'http://localhost:3000', got %q", allowOrigin)
	}
}

type deniedAuthStore struct{}

func (deniedAuthStore) Take(context.Context, string, int, time.Duration) (time.Duration, error) {
	return 1500 * time.Millisecond, nil
}

func TestAuthRoutesEnforceProtection(t *testing.T) {
	protection, err := middleware.NewAuthProtection(deniedAuthStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(nil, nil, nil, "", nil, "", nil, nil, nil, nil, []string{"https://app.example.com"}, false, true, http.SameSiteLaxMode, nil, protection, nil)
	for _, path := range []string{"/auth/login", "/auth/signup", "/auth/forgot-password", "/auth/reset-password"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Origin", "https://app.example.com")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" {
			t.Fatalf("%s missing protection: code=%d retry=%q", path, rec.Code, rec.Header().Get("Retry-After"))
		}
		if !strings.Contains(rec.Header().Get("Access-Control-Expose-Headers"), "Retry-After") {
			t.Fatal("cross-origin clients cannot read the retry interval")
		}
	}
}

func TestRouterRequestLogsExcludeQueries(t *testing.T) {
	var structured, legacy bytes.Buffer
	previousLogger, previousChiLogger := log.Logger, chiMiddleware.DefaultLogger
	log.Logger = zerolog.New(&structured)
	chiMiddleware.DefaultLogger = chiMiddleware.RequestLogger(&chiMiddleware.DefaultLogFormatter{
		Logger: stdlog.New(&legacy, "", 0), NoColor: true,
	})
	t.Cleanup(func() { log.Logger = previousLogger; chiMiddleware.DefaultLogger = previousChiLogger })
	router := NewRouter(nil, nil, nil, "", nil, "", nil, nil, nil, nil, nil, false, true, http.SameSiteLaxMode, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/youtube/callback?code=secret-code&state=secret-state", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)
	for _, output := range []string{structured.String(), legacy.String()} {
		if strings.Contains(output, "secret-code") || strings.Contains(output, "secret-state") {
			t.Fatal("router access logging leaked OAuth parameters")
		}
	}
	if !strings.Contains(structured.String(), "/auth/youtube/callback") {
		t.Fatal("request no longer logged")
	}
}
