package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamvalson/blink/internal/metrics"
)

func TestMetricsEndpoint(t *testing.T) {
	metrics.JobProcessed(metrics.PlatformTwitter)
	metrics.ObservePublishDuration(metrics.PlatformTwitter, 0.01)
	router := NewRouter(nil, nil, nil, "", "", nil, nil, nil, nil, nil, false, true, http.SameSiteNoneMode)

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
	router := NewRouter(nil, nil, nil, "", "", nil, nil, nil, nil, origins, false, true, http.SameSiteNoneMode)

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
