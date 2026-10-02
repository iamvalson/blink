package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func TestRetryPolicySchedule(t *testing.T) {
	policy := RetryPolicy{}
	if got := policy.NextRetryDelay(1); got != time.Minute {
		t.Fatalf("attempt 1 delay = %s, want 1m", got)
	}
	if got := policy.NextRetryDelay(2); got != 5*time.Minute {
		t.Fatalf("attempt 2 delay = %s, want 5m", got)
	}
	if got := policy.NextRetryDelay(99); got > MaxRetryDelay {
		t.Fatalf("delay = %s, exceeds maximum %s", got, MaxRetryDelay)
	}
}

func TestRetryPolicyClassifiesAndBoundsAttempts(t *testing.T) {
	policy := RetryPolicy{}
	retryable := connectors.NewClassifiedError(connectors.ErrorRetryable, "HTTP_503", errors.New("temporarily unavailable"))
	permanent := connectors.NewClassifiedError(connectors.ErrorPermanent, "HTTP_400", errors.New("invalid request"))
	ambiguous := connectors.NewClassifiedError(connectors.ErrorAmbiguous, "TIMEOUT", errors.New("request outcome unknown"))

	if !policy.ShouldRetry(retryable, 1) || !policy.ShouldRetry(retryable, 2) {
		t.Fatal("retryable errors should retry while attempts remain")
	}
	if policy.ShouldRetry(retryable, 3) {
		t.Fatal("attempt 3 must not schedule a fourth delivery")
	}
	if policy.ShouldRetry(permanent, 1) || policy.ShouldRetry(ambiguous, 1) {
		t.Fatal("permanent and ambiguous errors must not use the direct retry path")
	}
}
