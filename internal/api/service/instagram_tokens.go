package service

import (
	"context"
	"errors"
	"time"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/instagram"
)

const instagramRenewalWindow = 7 * 24 * time.Hour

// Instagram renews a live access token. Coordinate renewal with callbacks and
// workers by locking the same account row used by the other providers.
func (s *TokenManagerService) InstagramAccessToken(ctx context.Context, id, user string, client *instagram.Connector) (string, error) {
	tx, err := s.pgx.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	account, err := s.account.GetByIDForUpdate(ctx, tx, user, id)
	if err != nil {
		return "", err
	}
	if account.Platform != instagram.Platform || account.ExpiresAt == nil || !account.ExpiresAt.After(time.Now()) {
		return "", connectors.ErrInvalidRefreshToken
	}
	token, err := auth.DecryptToken(account.AccessToken, s.encryptionKey)
	if err != nil {
		return "", connectors.ErrTokenDecryptFailed
	}
	if time.Until(*account.ExpiresAt) > instagramRenewalWindow {
		return token, nil
	}
	result, err := client.RefreshAccessToken(ctx, token)
	if err != nil {
		return "", err
	}
	encrypted, err := auth.EncryptToken(result.AccessToken, s.encryptionKey)
	if err != nil {
		return "", connectors.ErrTokenPersistence
	}
	if err = s.account.UpdateOAuthTokensTX(ctx, tx, encrypted, *result.ExpiresAt, nil, id); err != nil {
		return "", errors.Join(connectors.ErrTokenPersistence, err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", errors.Join(connectors.ErrTokenPersistence, err)
	}
	return result.AccessToken, nil
}

func (s *TokenManagerService) MaintainInstagramTokens(ctx context.Context, client *instagram.Connector) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		accounts, err := s.account.InstagramAccountsDue(ctx, time.Now().Add(instagramRenewalWindow))
		if err == nil {
			for _, a := range accounts {
				if ctx.Err() != nil {
					return
				}
				_, _ = s.InstagramAccessToken(ctx, a.ID.String(), a.UserID.String(), client)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
