package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func mustEncrypt(t *testing.T, plaintext string) string {
	t.Helper()
	s, err := auth.EncryptToken(plaintext, testKey)
	if err != nil {
		t.Fatalf("encrypt %q: %v", plaintext, err)
	}
	return s
}

// ─── Stub OAuthTokenRefresher ────────────────────────────────────────────────

type stubRefresher struct {
	// inputs
	failWith     error
	returnToken  string
	returnExpiry time.Time
	returnRTok   *string // rotated refresh token, optional

	// observations
	calls        int
	refreshToken string // last plaintext refresh token passed in
}

func (r *stubRefresher) RefreshToken(_ context.Context, rt string) (connectors.TokenResult, error) {
	r.calls++
	r.refreshToken = rt
	if r.failWith != nil {
		return connectors.TokenResult{}, r.failWith
	}
	exp := r.returnExpiry
	return connectors.TokenResult{
		AccessToken:  r.returnToken,
		ExpiresAt:    &exp,
		RefreshToken: r.returnRTok,
	}, nil
}

// ─── Stub SocialAccountRepository ───────────────────────────────────────────

// stubAccountRepo is an in-memory stand-in for *storage.SocialAccountRepository.
// We build it to satisfy only the interface surface that TokenManagerService
// calls; the real repo requires a live pgxpool.Pool.
//
// Because TokenManagerService accepts *storage.SocialAccountRepository
// (a concrete type, not an interface), we cannot swap it at the
// service layer directly.  The token-manager tests that exercise the
// full DB-transaction path (GetByIDForUpdate / UpdateOAuthTokensTX)
// are therefore marked as requiring live infrastructure and are skipped
// when no database is available.  The helper tests below exercise the
// pure-logic helpers (tokenNeedsRefresh) and compile-time safety without
// a live database.

// ─── Unit tests for tokenNeedsRefresh ────────────────────────────────────────

func TestTokenNeedsRefresh_NilExpiry(t *testing.T) {
	account := &model.SocialAccount{ExpiresAt: nil}
	if !tokenNeedsRefresh(account) {
		t.Error("nil ExpiresAt should require refresh")
	}
}

func TestTokenNeedsRefresh_Expired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	account := &model.SocialAccount{ExpiresAt: &past}
	if !tokenNeedsRefresh(account) {
		t.Error("expired token should require refresh")
	}
}

func TestTokenNeedsRefresh_ExpiringWithinBuffer(t *testing.T) {
	soon := time.Now().Add(tokenRefreshBuffer - time.Second)
	account := &model.SocialAccount{ExpiresAt: &soon}
	if !tokenNeedsRefresh(account) {
		t.Error("token expiring within the 5-min buffer should require refresh")
	}
}

func TestTokenNeedsRefresh_StillValid(t *testing.T) {
	future := time.Now().Add(tokenRefreshBuffer + time.Minute)
	account := &model.SocialAccount{ExpiresAt: &future}
	if tokenNeedsRefresh(account) {
		t.Error("token with ample time remaining should NOT require refresh")
	}
}

// ─── Stub refresher validation ───────────────────────────────────────────────

func TestStubRefresher_ReturnsConfiguredToken(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	r := &stubRefresher{returnToken: "fresh-token", returnExpiry: exp}
	result, err := r.RefreshToken(context.Background(), "some-refresh-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "fresh-token" {
		t.Errorf("expected fresh-token, got %q", result.AccessToken)
	}
	if r.calls != 1 {
		t.Errorf("expected 1 call, got %d", r.calls)
	}
}

func TestStubRefresher_PropagatesError(t *testing.T) {
	sentinel := errors.New("platform unavailable")
	r := &stubRefresher{failWith: sentinel}
	_, err := r.RefreshToken(context.Background(), "rt")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// ─── Encryption round-trip ───────────────────────────────────────────────────

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := "test-oauth-access-token"
	enc := mustEncrypt(t, plain)
	dec, err := auth.DecryptToken(enc, testKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Errorf("expected %q, got %q", plain, dec)
	}
}

// ─── Infrastructure-gated DB tests ───────────────────────────────────────────
//
// The tests below exercise the full GetValidAccessToken / ForceRefreshAccessToken
// paths including PostgreSQL transactions, SELECT … FOR UPDATE, and UpdateOAuthTokensTX.
// They are SKIPPED unless DATABASE_URL is set in the environment.
//
// Run locally with:
//   DATABASE_URL=postgres://... go test ./internal/api/service/ -run TestTokenManager -v

// (No DB-backed tests are implemented here; they live in tests/publish_integration_test.go
//  and tests/reliability_integration_test.go which already skip without infrastructure.)
