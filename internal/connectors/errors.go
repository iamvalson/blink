package connectors

import (
	"errors"
	"strings"
)

type ErrorClass string

const (
	ErrorRetryable ErrorClass = "RETRYABLE"
	ErrorPermanent ErrorClass = "PERMANENT"
	ErrorAmbiguous ErrorClass = "AMBIGUOUS"
)

type ClassifiedError struct {
	Class      ErrorClass
	Code       string
	StatusCode int
	Err        error
}

func (e *ClassifiedError) Error() string { return e.Err.Error() }
func (e *ClassifiedError) Unwrap() error { return e.Err }

func NewClassifiedError(class ErrorClass, code string, err error) error {
	return &ClassifiedError{Class: class, Code: code, Err: err}
}

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorPermanent
	}
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Class
	}
	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		if rateLimitErr.Retryable {
			return ErrorRetryable
		}
		return ErrorPermanent
	}
	if errors.Is(err, ErrRateLimited) {
		return ErrorRetryable
	}
	if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrAuthFailed) {
		return ErrorPermanent
	}
	if errors.Is(err, ErrInvalidRefreshToken) {
		return ErrorPermanent
	}
	if errors.Is(err, ErrTokenRefreshFailed) || errors.Is(err, ErrTokenPersistence) {
		return ErrorRetryable
	}

	// Validation and media requirement errors are strictly permanent
	if errors.Is(err, ErrMediaRequired) || errors.Is(err, ErrUnsupportedMedia) ||
		errors.Is(err, ErrValidationFailed) || errors.Is(err, ErrMediaNotFound) ||
		errors.Is(err, ErrMediaIncomplete) {
		return ErrorPermanent
	}

	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "requires a video") ||
		strings.Contains(errMsg, "media required") ||
		strings.Contains(errMsg, "media not found") ||
		strings.Contains(errMsg, "media upload incomplete") ||
		strings.Contains(errMsg, "unsupported media") ||
		strings.Contains(errMsg, "invalid media") ||
		strings.Contains(errMsg, "cannot be used for remote publishing") ||
		strings.Contains(errMsg, "maximum publication attempts") {
		return ErrorPermanent
	}

	return ErrorAmbiguous
}

func HTTPError(status int, message string) error {
	if status == 401 || status == 403 {
		return &ClassifiedError{Class: ErrorPermanent, Code: "AUTH_FAILED", StatusCode: status, Err: errors.Join(ErrAuthFailed, errors.New("platform rejected credentials"))}
	}
	if status == 429 {
		return NewRateLimitError("unknown", status, 0, nil, message)
	}
	class := ErrorPermanent
	if status >= 500 {
		class = ErrorRetryable
	}
	classified := &ClassifiedError{Class: class, Code: "HTTP_ERROR", StatusCode: status, Err: errors.New(message)}
	return classified
}

var (
	ErrPublishFailed    = errors.New("failed to publish to platform")
	ErrAuthFailed       = errors.New("authentication failed")
	ErrUploadFailed     = errors.New("media upload failed")
	ErrInvalidToken     = errors.New("invalid or expired token")
	ErrRateLimited      = errors.New("platform rate limit exceeded")
	ErrTokenExpired     = errors.New("token expired")
	ErrMediaRequired    = errors.New("media required")
	ErrUnsupportedMedia = errors.New("unsupported media type")
	ErrValidationFailed = errors.New("validation failed")
	ErrMediaNotFound    = errors.New("media not found")
	ErrMediaIncomplete  = errors.New("media incomplete")
)
