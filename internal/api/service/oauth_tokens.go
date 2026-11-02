package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

type OAuthTokenService struct {
	accounts      *storage.SocialAccountRepository
	encryptionKey string
}

func NewOAuthTokenService(
	accounts *storage.SocialAccountRepository,
	encryptionKey string,
) *OAuthTokenService {
	return &OAuthTokenService{
		accounts:      accounts,
		encryptionKey: encryptionKey,
	}
}

func (s *OAuthTokenService) RefreshAccessToken(
	ctx context.Context,
	account model.SocialAccount,
	connector connectors.OAuthTokenRefresher,
) (string, error) {
	if account.RefreshToken == nil {
		return "", fmt.Errorf("refresh token not found")
	}

	decryptedRefreshToken, err := auth.DecryptToken(*account.RefreshToken, s.encryptionKey)

	if err != nil {
		return "", fmt.Errorf("failed to decrypt RefreshToken: %w", err)
	}

	tokens, err := connector.RefreshToken(ctx, decryptedRefreshToken)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	if tokens.AccessToken == "" {
		return "", errors.New("refresh returned an empty access token")
	}

	// Encrypted access token
	encryptedAccessToken, err := auth.EncryptToken(tokens.AccessToken, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt AccessToken: %w", err)
	}

	// Encrypted refresh token
	var encryptedRefreshToken *string

	if tokens.RefreshToken != nil && *tokens.RefreshToken != "" {
		encrypted, err := auth.EncryptToken(*tokens.RefreshToken, s.encryptionKey)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt RefreshToken: %w", err)
		}
		encryptedRefreshToken = &encrypted
	}

	err = s.accounts.UpdateOAuthTokens(ctx, account.ID, encryptedAccessToken, tokens.ExpiresAt, encryptedRefreshToken)
	if err != nil {
		return "", fmt.Errorf("failed to persist refreshed credentials: %w", err)
	}

	return tokens.AccessToken, nil
}
