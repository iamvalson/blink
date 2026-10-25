package service

import (
	"context"
	"testing"
)

func TestAccountsService_DisconnectAccount_UnsupportedPlatform(t *testing.T) {
	svc := NewAccountsService(nil, nil, "")
	err := svc.DisconnectAccount(context.Background(), "user-1", "unsupported")
	if err == nil {
		t.Fatal("expected error for unsupported platform, got nil")
	}
}

func TestAccountsService_GetTikTokCreatorInfo_Unconfigured(t *testing.T) {
	t.Run("nil accounts repo", func(t *testing.T) {
		svc := NewAccountsService(nil, nil, "")
		_, err := svc.GetTikTokCreatorInfo(context.Background(), "user-1", "acc-1")
		if err == nil {
			t.Fatal("expected error for nil accounts repository, got nil")
		}
	})

	t.Run("nil connector", func(t *testing.T) {
		// Even if accounts repo was present, if connector is nil it should guard
		svc := &AccountsService{accounts: nil, tiktokConnector: nil, encryptionKey: ""}
		_, err := svc.GetTikTokCreatorInfo(context.Background(), "user-1", "acc-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

