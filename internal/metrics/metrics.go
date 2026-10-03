// Package metrics defines Blink's low-cardinality Prometheus metrics.
package metrics

import "github.com/prometheus/client_golang/prometheus"

const (
	PlatformTwitter = "twitter"
	PlatformYouTube = "youtube"
	PlatformUnknown = "unknown"
)

var (
	jobsProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_jobs_processed_total",
		Help: "Total number of publish-job executions started by the worker.",
	}, []string{"platform"})
	jobsSucceeded = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_jobs_succeeded_total",
		Help: "Total number of publish-job executions that succeeded.",
	}, []string{"platform"})
	jobsFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_jobs_failed_total",
		Help: "Total number of publish-job executions that failed.",
	}, []string{"platform"})
	jobsRetried = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_jobs_retried_total",
		Help: "Total number of failed publish executions scheduled for another attempt.",
	}, []string{"platform"})
	jobsPermanentlyFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_jobs_permanently_failed_total",
		Help: "Total number of publish executions finalized as permanent failures.",
	}, []string{"platform"})
	publishDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "blink_publish_duration_seconds",
		Help:    "Duration of external platform publish operations in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"platform"})
	rateLimitEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_rate_limit_events_total",
		Help: "Total number of platform rate-limit events detected.",
	}, []string{"platform"})
	tokenRefreshes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_token_refreshes_total",
		Help: "Total number of OAuth token refresh attempts by result.",
	}, []string{"platform", "result"})
	queueFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "blink_queue_failures_total",
		Help: "Total number of queue infrastructure failures by stage.",
	}, []string{"reason"})
	workerJobsInProgress = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "blink_worker_jobs_in_progress",
		Help: "Number of publish executions currently being processed.",
	})
)

func init() {
	prometheus.MustRegister(
		jobsProcessed,
		jobsSucceeded,
		jobsFailed,
		jobsRetried,
		jobsPermanentlyFailed,
		publishDuration,
		rateLimitEvents,
		tokenRefreshes,
		queueFailures,
		workerJobsInProgress,
	)
}

func NormalizePlatform(platform string) string {
	switch platform {
	case PlatformTwitter, PlatformYouTube:
		return platform
	default:
		return PlatformUnknown
	}
}

func JobProcessed(platform string) { jobsProcessed.WithLabelValues(NormalizePlatform(platform)).Inc() }
func JobSucceeded(platform string) { jobsSucceeded.WithLabelValues(NormalizePlatform(platform)).Inc() }
func JobFailed(platform string)    { jobsFailed.WithLabelValues(NormalizePlatform(platform)).Inc() }
func JobRetried(platform string)   { jobsRetried.WithLabelValues(NormalizePlatform(platform)).Inc() }
func JobPermanentlyFailed(platform string) {
	jobsPermanentlyFailed.WithLabelValues(NormalizePlatform(platform)).Inc()
}
func ObservePublishDuration(platform string, seconds float64) {
	publishDuration.WithLabelValues(NormalizePlatform(platform)).Observe(seconds)
}
func RateLimitEvent(platform string) {
	rateLimitEvents.WithLabelValues(NormalizePlatform(platform)).Inc()
}
func TokenRefresh(platform, result string) {
	if result != "success" {
		result = "failure"
	}
	tokenRefreshes.WithLabelValues(NormalizePlatform(platform), result).Inc()
}
func WorkerJobStarted()  { workerJobsInProgress.Inc() }
func WorkerJobFinished() { workerJobsInProgress.Dec() }

func QueueFailure(reason string) {
	switch reason {
	case "enqueue", "processing", "deserialization", "handler", "scheduling":
	default:
		reason = "unknown"
	}
	queueFailures.WithLabelValues(reason).Inc()
}
