package connectors

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRateLimitErrorIncludesRetryMetadata(t *testing.T) {
	resetAt := time.Unix(1700000000, 0).UTC()
	err := NewRateLimitError(PlatformTwitter, http.StatusTooManyRequests, 120*time.Second, &resetAt, "X API rate limit exceeded")
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatal("expected errors.As to find RateLimitError")
	}
	if rateLimitErr.Platform != PlatformTwitter {
		t.Fatalf("platform = %q, want %q", rateLimitErr.Platform, PlatformTwitter)
	}
	if rateLimitErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rateLimitErr.StatusCode, http.StatusTooManyRequests)
	}
	if rateLimitErr.RetryAfter != 120*time.Second {
		t.Fatalf("retry after = %s, want %s", rateLimitErr.RetryAfter, 120*time.Second)
	}
	if rateLimitErr.ResetAt == nil || !rateLimitErr.ResetAt.Equal(resetAt) {
		t.Fatalf("reset_at = %v, want %v", rateLimitErr.ResetAt, &resetAt)
	}
	if !rateLimitErr.Retryable {
		t.Fatal("rate limit errors must be retryable")
	}
}

func TestParseRetryAfterHeaderRejectsInvalidValues(t *testing.T) {
	if got := ParseRetryAfterHeader("abc"); got != 0 {
		t.Fatalf("ParseRetryAfterHeader(abc) = %s, want 0", got)
	}
	if got := ParseRetryAfterHeader("-10"); got != 0 {
		t.Fatalf("ParseRetryAfterHeader(-10) = %s, want 0", got)
	}
}
