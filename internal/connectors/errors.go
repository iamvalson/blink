package connectors

import "errors"

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
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Class
	}
	if errors.Is(err, ErrRateLimited) {
		return ErrorRetryable
	}
	if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrAuthFailed) {
		return ErrorPermanent
	}
	return ErrorAmbiguous
}

func HTTPError(status int, message string) error {
	class := ErrorPermanent
	if status == 429 || status >= 500 {
		class = ErrorRetryable
	}
	classified := &ClassifiedError{Class: class, Code: "HTTP_ERROR", StatusCode: status, Err: errors.New(message)}
	return classified
}

var (
	ErrPublishFailed = errors.New("failed to publish to platform")
	ErrAuthFailed    = errors.New("authentication failed")
	ErrUploadFailed  = errors.New("media upload failed")
	ErrInvalidToken  = errors.New("invalid or expired token")
	ErrRateLimited   = errors.New("platform rate limit exceeded")
	ErrTokenExpired  = errors.New("token expired")
)
