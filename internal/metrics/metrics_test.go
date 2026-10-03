package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestHelpersNormalizeLabelsAndRecordMetrics(t *testing.T) {
	JobProcessed("twitter")
	JobSucceeded("twitter")
	JobFailed("twitter")
	JobRetried("twitter")
	JobPermanentlyFailed("twitter")
	ObservePublishDuration("twitter", 0.2)
	RateLimitEvent("twitter")
	TokenRefresh("twitter", "success")
	TokenRefresh("twitter", "unexpected")
	QueueFailure("unexpected")
	WorkerJobStarted()
	WorkerJobFinished()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, name := range []string{
		"blink_jobs_processed_total",
		"blink_jobs_succeeded_total",
		"blink_jobs_failed_total",
		"blink_jobs_retried_total",
		"blink_jobs_permanently_failed_total",
		"blink_publish_duration_seconds",
		"blink_rate_limit_events_total",
		"blink_token_refreshes_total",
		"blink_queue_failures_total",
		"blink_worker_jobs_in_progress",
	} {
		found := false
		for _, family := range families {
			if family.GetName() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected metric family %q", name)
		}
	}

	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "blink_") {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "job_id" || label.GetName() == "post_id" || label.GetName() == "user_id" || label.GetName() == "error" {
					t.Errorf("high-cardinality label %q found on %s", label.GetName(), family.GetName())
				}
			}
		}
	}
}

func TestNormalizePlatform(t *testing.T) {
	if got := NormalizePlatform("custom-platform"); got != PlatformUnknown {
		t.Fatalf("expected unknown platform, got %q", got)
	}
}
