package worker

import (
	"errors"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

const (
	MaxAttempts   = 3
	MaxRetryDelay = time.Hour
)

type RetryPolicy struct{}

func (RetryPolicy) ShouldRetry(err error, attemptCount int) bool {
	return attemptCount < MaxAttempts && connectors.ClassifyError(err) == connectors.ErrorRetryable
}

func (RetryPolicy) ResolveRetryDelay(err error, attemptCount int) time.Duration {
	var rateLimitErr *connectors.RateLimitError
	if errors.As(err, &rateLimitErr) {
		if rateLimitErr.RetryAfter > 0 {
			return minRetryDelay(rateLimitErr.RetryAfter)
		}
		if rateLimitErr.ResetAt != nil {
			delay := time.Until(*rateLimitErr.ResetAt)
			if delay < 0 {
				delay = 0
			}
			return minRetryDelay(delay)
		}
	}
	return minRetryDelay(RetryPolicy{}.NextRetryDelay(attemptCount))
}

func (RetryPolicy) NextRetryDelay(attemptCount int) time.Duration {
	var delay time.Duration
	switch attemptCount {
	case 1:
		delay = time.Minute
	case 2:
		delay = 5 * time.Minute
	default:
		delay = 30 * time.Minute
	}
	return minRetryDelay(delay)
}

func minRetryDelay(delay time.Duration) time.Duration {
	if delay < 0 {
		return 0
	}
	if delay > MaxRetryDelay {
		return MaxRetryDelay
	}
	return delay
}

var retryPolicy RetryPolicy

type publishError struct {
	classification connectors.ErrorClass
	err            error
}

func (e *publishError) Error() string { return e.err.Error() }
func (e *publishError) Unwrap() error { return e.err }

func newPublishError(classification connectors.ErrorClass, err error) error {
	if err == nil {
		return nil
	}
	return &publishError{classification: classification, err: err}
}

func publishClassification(err error) connectors.ErrorClass {
	var classified *publishError
	if errors.As(err, &classified) {
		return classified.classification
	}
	return connectors.ClassifyError(err)
}
