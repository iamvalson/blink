package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	tokenRefreshBuffer    = 5 * time.Minute
	defaultRefreshTimeout = 30 * time.Second
)

type TokenManagerService struct {
	account       *storage.SocialAccountRepository
	pgx           *pgxpool.Pool
	encryptionKey string
}

func NewTokenManagerService(
	account *storage.SocialAccountRepository,
	pgx *pgxpool.Pool,
	encryptionKey string,
) *TokenManagerService {
	return &TokenManagerService{
		account:       account,
		pgx:           pgx,
		encryptionKey: encryptionKey,
	}
}

// tokenNeedsRefresh checks whether the token is expired
// or will expire in the next five minutes
func tokenNeedsRefresh(account *model.SocialAccount) bool {
	if account.ExpiresAt == nil {
		return true
	}

	return time.Until(*account.ExpiresAt) <= tokenRefreshBuffer
}

// GetValidAccessToken return a decrypted access token,
// refreshing and persisting credentials when necessary
func (s *TokenManagerService) GetValidAccessToken(
	ctx context.Context,
	accountID string,
	userID string,
	connector connectors.OAuthTokenRefresher,
) (string, error) {
	account, err := s.account.GetByIDAndUserID(ctx, userID, accountID)
	if err != nil {
		return "", fmt.Errorf("failed to get account: %w", err)
	}

	// Check token is still valid
	if !tokenNeedsRefresh(account) {
		token, err := auth.DecryptToken(
			account.AccessToken,
			s.encryptionKey,
		)
		if err != nil {
			return "", fmt.Errorf("%w: %v", connectors.ErrTokenDecryptFailed, err)
		}
		return token, nil
	}

	// Begin db transaction
	tx, err := s.pgx.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Lock the account row and read fresh credentials
	account, err = s.account.GetByIDForUpdate(ctx, tx, userID, accountID)
	if err != nil {
		return "", fmt.Errorf("failed to lock account: %w", err)
	}

	// Check expiry again after acquiring the lock
	// Another process might have refreshed the token already
	if !tokenNeedsRefresh(account) {
		accessToken, err := auth.DecryptToken(account.AccessToken, s.encryptionKey)
		if err != nil {
			return "", fmt.Errorf("%w: failed to decrypt access token: %v", connectors.ErrTokenDecryptFailed, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("%w: failed to commit transaction: %v", connectors.ErrTokenPersistence, err)
		}

		return accessToken, nil
	}

	// Check a refresh token exists
	if account.RefreshToken == nil || *account.RefreshToken == "" {
		return "", connectors.ErrInvalidRefreshToken
	}

	// Decrypt the refreshToken
	refreshToken, err := auth.DecryptToken(*account.RefreshToken, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decrypt refresh token: %v", connectors.ErrTokenDecryptFailed, err)
	}

	// Request new token from platform with bounded timeout so row locks are not held indefinitely.
	refreshCtx, cancel := context.WithTimeout(ctx, defaultRefreshTimeout)
	defer cancel()
	result, err := connector.RefreshToken(refreshCtx, refreshToken)
	if err != nil {
		if errors.Is(err, connectors.ErrInvalidRefreshToken) {
			return "", err
		}
		return "", fmt.Errorf("%w: %v", connectors.ErrTokenRefreshFailed, err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("%w: refresh returned an empty access token", connectors.ErrTokenRefreshFailed)
	}

	if result.ExpiresAt == nil {
		return "", fmt.Errorf("%w: refresh returned no access token expiry", connectors.ErrTokenRefreshFailed)
	}

	// Encrypt new access token
	encryptedAccessToken, err := auth.EncryptToken(
		result.AccessToken,
		s.encryptionKey,
	)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encrypt access token: %v", connectors.ErrTokenPersistence, err)
	}

	// Encrypt the rotated refresh token, if provided.
	// Nil preserves the existing refresh token in PostgreSQL.
	var encryptedRefreshToken *string

	if result.RefreshToken != nil && *result.RefreshToken != "" {
		encrypted, err := auth.EncryptToken(
			*result.RefreshToken,
			s.encryptionKey,
		)
		if err != nil {
			return "", fmt.Errorf("%w: failed to encrypt refresh token: %v", connectors.ErrTokenPersistence, err)
		}

		encryptedRefreshToken = &encrypted
	}

	// Update credentials within the SAME transaction.
	err = s.account.UpdateOAuthTokensTX(
		ctx,
		tx,
		encryptedAccessToken,
		*result.ExpiresAt,
		encryptedRefreshToken,
		accountID,
	)
	if err != nil {
		return "", fmt.Errorf("%w: failed to persist refreshed credentials: %v", connectors.ErrTokenPersistence, err)
	}

	// Commit the transaction and release the row lock.
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("%w: failed to commit refreshed credentials: %v", connectors.ErrTokenPersistence, err)
	}

	return result.AccessToken, nil
}

func (s *TokenManagerService) ForceRefreshAccessToken(
	ctx context.Context,
	accountID string,
	userID string,
	rejectedAccessToken string,
	connector connectors.OAuthTokenRefresher,
) (string, error) {
	tx, err := s.pgx.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin token refresh transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	account, err := s.account.GetByIDForUpdate(ctx, tx, userID, accountID)
	if err != nil {
		return "", fmt.Errorf("lock social account: %w", err)
	}

	currentToken, err := auth.DecryptToken(
		account.AccessToken,
		s.encryptionKey,
	)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt access token: %v", connectors.ErrTokenDecryptFailed, err)
	}

	// Another process may already have replaced the rejected token.
	if currentToken != rejectedAccessToken && !tokenNeedsRefresh(account) {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("%w: commit transaction: %v", connectors.ErrTokenPersistence, err)
		}
		return currentToken, nil
	}

	if account.RefreshToken == nil || *account.RefreshToken == "" {
		return "", connectors.ErrInvalidRefreshToken
	}

	refreshToken, err := auth.DecryptToken(
		*account.RefreshToken,
		s.encryptionKey,
	)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt refresh token: %v", connectors.ErrTokenDecryptFailed, err)
	}

	// Bounded timeout for external refresh call
	refreshCtx, cancel := context.WithTimeout(ctx, defaultRefreshTimeout)
	defer cancel()
	result, err := connector.RefreshToken(refreshCtx, refreshToken)
	if err != nil {
		if errors.Is(err, connectors.ErrInvalidRefreshToken) {
			return "", err
		}
		return "", fmt.Errorf("%w: %v", connectors.ErrTokenRefreshFailed, err)
	}
	if result.AccessToken == "" || result.ExpiresAt == nil {
		return "", connectors.ErrTokenRefreshFailed
	}

	encryptedAccess, err := auth.EncryptToken(
		result.AccessToken,
		s.encryptionKey,
	)
	if err != nil {
		return "", fmt.Errorf("%w: encrypt access token: %v", connectors.ErrTokenPersistence, err)
	}

	var encryptedRefresh *string
	if result.RefreshToken != nil && *result.RefreshToken != "" {
		value, err := auth.EncryptToken(
			*result.RefreshToken,
			s.encryptionKey,
		)
		if err != nil {
			return "", fmt.Errorf("%w: encrypt refresh token: %v", connectors.ErrTokenPersistence, err)
		}
		encryptedRefresh = &value
	}

	if err := s.account.UpdateOAuthTokensTX(
		ctx,
		tx,
		encryptedAccess,
		*result.ExpiresAt,
		encryptedRefresh,
		accountID,
	); err != nil {
		return "", fmt.Errorf("%w: persist refreshed tokens: %v", connectors.ErrTokenPersistence, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("%w: commit refreshed tokens: %v", connectors.ErrTokenPersistence, err)
	}

	return result.AccessToken, nil
}
