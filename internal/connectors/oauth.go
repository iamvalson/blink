package connectors

import (
	"context"
	"errors"
	"time"
)

const DefaultRefreshSkew = time.Minute

// TokenNeedsRefresh returns true only when expiry is known and the token is
// expired or close enough to expiry that an API request could race it.
func TokenNeedsRefresh(expiresAt *time.Time, now time.Time, skew time.Duration) bool {
	if expiresAt == nil || expiresAt.IsZero() {
		return false
	}
	return !expiresAt.After(now.Add(skew))
}

type TokenResult struct {
	AccessToken  string
	ExpiresAt    *time.Time
	RefreshToken *string
}

// OAuthTokenRefresher is implemented by connectors that support refresh-token
// authentication. The worker depends only on this contract, never a provider.
type OAuthTokenRefresher interface {
	RefreshToken(ctx context.Context, refreshToken string) (TokenResult, error)
}

var (
	ErrInvalidRefreshToken = errors.New("refresh token is invalid or revoked")
	ErrTokenRefreshFailed  = errors.New("oauth token refresh failed")
	ErrTokenDecryptFailed  = errors.New("oauth token decryption failed")
	ErrTokenPersistence    = errors.New("oauth token persistence failed")
)
