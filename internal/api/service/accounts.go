package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/tiktok"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

var (
	ErrNotTikTokAccount = errors.New("account is not a TikTok account")
)

type AccountTokenRevoker interface {
	RevokeTokens(ctx context.Context, accessToken, refreshToken string) error
}

type AccountsService struct {
	revokers        map[string]AccountTokenRevoker
	accounts        *storage.SocialAccountRepository
	tiktokConnector *tiktok.Connector
	tokenManager    *TokenManagerService
}

func NewAccountsService(accounts *storage.SocialAccountRepository, tiktokConnector *tiktok.Connector, tokenManager *TokenManagerService, revokers map[string]AccountTokenRevoker) *AccountsService {
	return &AccountsService{accounts: accounts, tiktokConnector: tiktokConnector, tokenManager: tokenManager, revokers: revokers}
}

func (s *AccountsService) ListAccounts(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	return s.accounts.ListByUserID(ctx, userID)
}

func (s *AccountsService) DisconnectAccount(ctx context.Context, userID string, platform string) error {
	switch platform {
	case connectors.PlatformTwitter, connectors.PlatformYoutube, connectors.PlatformTikTok:
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	revoker := s.revokers[platform]
	if revoker == nil || s.tokenManager == nil {
		return fmt.Errorf("%s revocation is not configured", platform)
	}

	if platform == connectors.PlatformTikTok {
		if s.tiktokConnector == nil {
			return errors.New("TikTok token refresh is not configured")
		}
		// TikTok requires an access token. Refresh through the token manager first
		// so a rotated refresh token stays saved even if revocation later fails.
		id, err := s.accounts.GetAccountID(ctx, userID, platform)
		if errors.Is(err, storage.ErrSocialAccountNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find TikTok account: %w", err)
		}
		_, err = s.tokenManager.GetValidAccessToken(ctx, id, userID, s.tiktokConnector)
		if errors.Is(err, storage.ErrSocialAccountNotFound) {
			return nil
		}
		if err != nil && !errors.Is(err, connectors.ErrInvalidRefreshToken) {
			return fmt.Errorf("prepare TikTok revocation: %w", err)
		}
	}

	return s.accounts.RevokeAndDeleteAccount(ctx, userID, platform, func(encryptedAccess string, encryptedRefresh *string) error {
		key := s.tokenManager.encryptionKey
		access, err := auth.DecryptToken(encryptedAccess, key)
		if err != nil {
			return fmt.Errorf("decrypt %s access token: %w", platform, err)
		}
		var refresh string
		if platform != connectors.PlatformTikTok && encryptedRefresh != nil && *encryptedRefresh != "" {
			refresh, err = auth.DecryptToken(*encryptedRefresh, key)
			if err != nil {
				return fmt.Errorf("decrypt %s refresh token: %w", platform, err)
			}
		}
		return revoker.RevokeTokens(ctx, access, refresh)
	})
}

func (s *AccountsService) GetTikTokCreatorInfo(ctx context.Context, userID string, accountID string) (*tiktok.TikTokQueryCreatorDataResponse, error) {
	if s.accounts == nil {
		return nil, errors.New("accounts repository is not configured")
	}
	if s.tiktokConnector == nil {
		return nil, errors.New("tiktok connector is not configured")
	}
	if s.tokenManager == nil {
		return nil, errors.New(
			"token manager is not configured",
		)
	}

	// Verify that the account belongs to the user
	// and is connected to TikTok.
	account, err := s.accounts.GetByIDAndUserID(ctx, userID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	if account.Platform != "tiktok" {
		return nil, ErrNotTikTokAccount
	}

	// Retrieve a valid decrypted access token.
	accessToken, err := s.tokenManager.GetValidAccessToken(ctx, accountID, userID, s.tiktokConnector)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get valid TikTok access token: %w",
			err,
		)
	}

	// Query TikTok using the valid access token.
	data, err := s.tiktokConnector.QueryCreatorInfo(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get TikTok creator info: %w", err)
	}

	return data, nil
}
