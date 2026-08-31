package connectors

import (
	"errors"
)

var (
	ErrPublishFailed   = errors.New("failed to publish to platform")
	ErrAuthFailed      = errors.New("authentication failed")
	ErrUploadFailed    = errors.New("media upload failed")
	ErrInvalidToken    = errors.New("invalid or expired token")
	ErrRateLimited     = errors.New("platform rate limit exceeded")
	ErrTokenExpired    = errors.New("token expired")
)