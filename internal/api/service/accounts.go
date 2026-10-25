package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors/tiktok"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

var (
	ErrNotTikTokAccount = errors.New("account is not a TikTok account")
)

type AccountsService struct {
	accounts        *storage.SocialAccountRepository
	tiktokConnector *tiktok.Connector
	encryptionKey   string
}

func NewAccountsService(accounts *storage.SocialAccountRepository, tiktokConnector *tiktok.Connector, encryptionKey string) *AccountsService {
	return &AccountsService{accounts: accounts, tiktokConnector: tiktokConnector, encryptionKey: encryptionKey}
}

func (s *AccountsService) ListAccounts(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	return s.accounts.ListByUserID(ctx, userID)
}

func (s *AccountsService) DisconnectAccount(ctx context.Context, userID string, platform string) error {
	// Only valid platforms (we can expand this if needed, or rely on frontend routing/existing integrations)
	if platform != "twitter" && platform != "youtube" && platform != "tiktok" {
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	return s.accounts.DeleteByUserIDAndPlatform(ctx, userID, platform)
}

func (s *AccountsService) GetTikTokCreatorInfo(ctx context.Context, userID string, accountID string) (*tiktok.TikTokQueryCreatorDataResponse, error) {
	if s.accounts == nil {
		return nil, errors.New("accounts repository is not configured")
	}
	if s.tiktokConnector == nil {
		return nil, errors.New("tiktok connector is not configured")
	}

	account, err := s.accounts.GetByIDAndUserID(ctx, userID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	if account.Platform != "tiktok" {
		return nil, ErrNotTikTokAccount
	}

	decryptedToken, err := auth.DecryptToken(account.AccessToken, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	data, err := s.tiktokConnector.QueryCreatorInfo(ctx, decryptedToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get TikTok creator info: %w", err)
	}

	return data, nil
}
