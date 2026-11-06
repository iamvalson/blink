package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
)

type sessionVersionFunc func(context.Context, string) (int64, error)

func (f sessionVersionFunc) SessionVersion(ctx context.Context, id string) (int64, error) {
	return f(ctx, id)
}

func TestSessionVersionsRevokeTokensAndFailClosed(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	service := NewJWTService(private, public)
	old, err := service.CreateAccessToken("user", true)
	if err != nil {
		t.Fatal(err)
	}
	service.WithSessionVersionStore(sessionVersionFunc(func(context.Context, string) (int64, error) { return 1, nil }))
	if _, err := service.ValidateAccessToken(old); err == nil {
		t.Fatal("legacy session accepted after reset")
	}
	current, err := service.CreateAccessTokenVersion("user", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateAccessToken(current); err != nil {
		t.Fatal(err)
	}
	service.WithSessionVersionStore(sessionVersionFunc(func(context.Context, string) (int64, error) { return 0, errors.New("database down") }))
	if _, err := service.ValidateAccessToken(current); err == nil {
		t.Fatal("database failure bypassed session check")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.WithSessionVersionStore(sessionVersionFunc(func(ctx context.Context, _ string) (int64, error) { return 1, ctx.Err() }))
	if _, err := service.ValidateAccessTokenContext(ctx, current); err == nil {
		t.Fatal("request cancellation was not propagated")
	}
}
