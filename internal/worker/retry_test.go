package worker

import (
	"errors"
	"strings"
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

func TestFailureDetailsPreservePlatformMetadataWithoutSecrets(t *testing.T) {
	err := connectors.NewClassifiedError(
		connectors.ErrorPermanent,
		"invalid_token",
		errors.New("HTTP 401 access_token=secret refresh_token=refresh-secret invalid token"),
	)
	classified := err.(*connectors.ClassifiedError)
	classified.StatusCode = 401

	failureType, code, reason, response := failureDetails(err)
	if failureType != "AUTHENTICATION" {
		t.Fatalf("failure type = %q, want AUTHENTICATION", failureType)
	}
	if code != "invalid_token" {
		t.Fatalf("error code = %q, want invalid_token", code)
	}
	if strings.Contains(reason, "secret") || strings.Contains(reason, "refresh-secret") {
		t.Fatalf("failure reason contains a secret: %q", reason)
	}
	if !strings.Contains(string(response), `"status_code":401`) || !strings.Contains(string(response), "invalid_token") {
		t.Fatalf("platform response did not preserve safe metadata: %s", response)
	}
}
