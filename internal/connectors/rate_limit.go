package connectors

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitError represents a platform-originated rate-limit failure that is
// retryable and can carry server-provided timing information.
type RateLimitError struct {
	Platform   string
	StatusCode int
	RetryAfter time.Duration
	ResetAt    *time.Time
	Message    string
	Retryable  bool
}

func (e *RateLimitError) Error() string {
	if e == nil {
		return "platform rate limit exceeded"
	}
	if e.Message != "" {
		return e.Message
	}
	return "platform rate limit exceeded"
}

func (e *RateLimitError) Unwrap() error {
	if e == nil {
		return ErrRateLimited
	}
	return ErrRateLimited
}

func NewRateLimitError(platform string, statusCode int, retryAfter time.Duration, resetAt *time.Time, message string) error {
	if statusCode == 0 {
		statusCode = http.StatusTooManyRequests
	}
	if retryAfter < 0 {
		retryAfter = 0
	}
	if resetAt != nil && retryAfter == 0 {
		delay := time.Until(*resetAt)
		if delay < 0 {
			delay = 0
		}
		retryAfter = delay
	}
	if message == "" {
		message = "platform rate limit exceeded"
	}
	return &RateLimitError{
		Platform:   platform,
		StatusCode: statusCode,
		RetryAfter: retryAfter,
		ResetAt:    resetAt,
		Message:    message,
		Retryable:  true,
	}
}

// RateLimitPolicy allows platform connector implementations to translate
// platform-specific rate-limit responses into a common structured error.
type RateLimitPolicy interface {
	Classify(statusCode int, headers http.Header) *RateLimitError
}

type PlatformRateLimitPolicy struct {
	Platform string
}

func (p PlatformRateLimitPolicy) Classify(statusCode int, headers http.Header) *RateLimitError {
	if statusCode != http.StatusTooManyRequests {
		return nil
	}
	if headers == nil {
		headers = http.Header{}
	}
	retryAfter := ParseRetryAfterHeader(headers.Get("Retry-After"))
	if retryAfter == 0 {
		if resetAt := ParseRateLimitResetHeader(headers.Get("X-RateLimit-Reset")); resetAt != nil {
			retryAfter = time.Until(*resetAt)
			if retryAfter < 0 {
				retryAfter = 0
			}
		}
	}
	return &RateLimitError{
		Platform:   p.Platform,
		StatusCode: statusCode,
		RetryAfter: retryAfter,
		ResetAt:    ParseRateLimitResetHeader(headers.Get("X-RateLimit-Reset")),
		Message:    fmt.Sprintf("%s rate limit exceeded", p.Platform),
		Retryable:  true,
	}
}

func ParseRetryAfterHeader(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if sec, err := strconv.Atoi(trimmed); err == nil {
		if sec < 0 {
			return 0
		}
		return time.Duration(sec) * time.Second
	}
	if duration, err := time.ParseDuration(trimmed); err == nil {
		if duration < 0 {
			return 0
		}
		return duration
	}
	if ts, err := http.ParseTime(trimmed); err == nil {
		delay := time.Until(ts)
		if delay < 0 {
			return 0
		}
		return delay
	}
	return 0
}

func ParseRateLimitResetHeader(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if secs, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if secs < 0 {
			return nil
		}
		t := time.Unix(secs, 0).UTC()
		return &t
	}
	if ts, err := http.ParseTime(trimmed); err == nil {
		return &ts
	}
	return nil
}
