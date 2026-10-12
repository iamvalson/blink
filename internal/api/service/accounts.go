package service

import (
	"context"
	"fmt"

	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

type AccountsService struct {
	accounts *storage.SocialAccountRepository
}

func NewAccountsService(accounts *storage.SocialAccountRepository) *AccountsService {
	return &AccountsService{accounts: accounts}
}

func (s *AccountsService) ListAccounts(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	return s.accounts.ListByUserID(ctx, userID)
}

func (s *AccountsService) DisconnectAccount(ctx context.Context, userID string, platform string) error {
	// Only valid platforms (we can expand this if needed, or rely on frontend routing/existing integrations)
	if platform != "twitter" && platform != "youtube" {
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	return s.accounts.DeleteByUserIDAndPlatform(ctx, userID, platform)
}

