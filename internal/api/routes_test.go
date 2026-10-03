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
	router := NewRouter(nil, nil, nil, "", nil, nil, nil)

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
