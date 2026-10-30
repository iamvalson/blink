package tests

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type concurrentTestRefresher struct {
	mu           sync.Mutex
	refreshCalls atomic.Int32
	delay        time.Duration
	tokenPrefix  string
	failWith     error
}

func (r *concurrentTestRefresher) RefreshToken(ctx context.Context, rt string) (connectors.TokenResult, error) {
	call := r.refreshCalls.Add(1)
	if r.delay > 0 {
		select {
		case <-ctx.Done():
			return connectors.TokenResult{}, ctx.Err()
		case <-time.After(r.delay):
		}
	}
	if r.failWith != nil {
		return connectors.TokenResult{}, r.failWith
	}
	newExpiry := time.Now().Add(1 * time.Hour)
	prefix := r.tokenPrefix
	if prefix == "" {
		prefix = "refreshed"
	}
	newToken := fmt.Sprintf("%s-access-token-%d", prefix, call)
	newRT := fmt.Sprintf("%s-refresh-token-%d", prefix, call)
	return connectors.TokenResult{
		AccessToken:  newToken,
		ExpiresAt:    &newExpiry,
		RefreshToken: &newRT,
	}, nil
}

func createTestUserAndAccount(t *testing.T, ctx context.Context, db *pgxpool.Pool, accessPlain string, refreshPlain *string, expiresAt *time.Time) (string, string) {
	t.Helper()
	userID := uuid.New().String()
	_, err := db.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES ($1, $2, 'hash', 'Test User')
	`, userID, fmt.Sprintf("user-%s@test.local", userID[:8]))
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	accountID := uuid.New().String()
	encAccess, err := auth.EncryptToken(accessPlain, testEncryptionKey)
	if err != nil {
		t.Fatalf("encrypt access token: %v", err)
	}

	var encRefresh *string
	if refreshPlain != nil {
		enc, err := auth.EncryptToken(*refreshPlain, testEncryptionKey)
		if err != nil {
			t.Fatalf("encrypt refresh token: %v", err)
		}
		encRefresh = &enc
	}

	_, err = db.Exec(ctx, `
		INSERT INTO social_accounts (id, user_id, platform, platform_user_id, access_token, refresh_token, expires_at)
		VALUES ($1, $2, 'twitter', 'tw-test', $3, $4, $5)
	`, accountID, userID, encAccess, encRefresh, expiresAt)
	if err != nil {
		t.Fatalf("insert test social account: %v", err)
	}

	return userID, accountID
}

// 1. Valid access token: does not trigger refresh
func TestTokenManager_ValidToken_NoRefresh(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()

	validExpiry := time.Now().Add(2 * time.Hour)
	rf := "refresh-val"
	userID, accountID := createTestUserAndAccount(t, ctx, db, "valid-plain-token", &rf, &validExpiry)

	accountRepo := storage.NewSocialAccountRepository(db)
	tm := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)
	refresher := &concurrentTestRefresher{}

	token, err := tm.GetValidAccessToken(ctx, accountID, userID, refresher)
	if err != nil {
		t.Fatalf("GetValidAccessToken: %v", err)
	}
	if token != "valid-plain-token" {
		t.Errorf("expected valid-plain-token, got %q", token)
	}
	if calls := refresher.refreshCalls.Load(); calls != 0 {
		t.Errorf("expected 0 refresh calls for valid token, got %d", calls)
	}
}

// 2. Nil expiry: requires refresh
func TestTokenManager_NilExpiry_Refreshes(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()

	rf := "refresh-val"
	userID, accountID := createTestUserAndAccount(t, ctx, db, "nil-exp-token", &rf, nil)

	accountRepo := storage.NewSocialAccountRepository(db)
	tm := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)
	refresher := &concurrentTestRefresher{}

	token, err := tm.GetValidAccessToken(ctx, accountID, userID, refresher)
	if err != nil {
		t.Fatalf("GetValidAccessToken: %v", err)
	}
	if token != "refreshed-access-token-1" {
		t.Errorf("expected refreshed-access-token-1, got %q", token)
	}
	if calls := refresher.refreshCalls.Load(); calls != 1 {
		t.Errorf("expected 1 refresh call for nil expiry, got %d", calls)
	}
}

// 3. Missing refresh token: returns ErrInvalidRefreshToken
func TestTokenManager_MissingRefreshToken_Fails(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	userID, accountID := createTestUserAndAccount(t, ctx, db, "expired-token", nil, &past)

	accountRepo := storage.NewSocialAccountRepository(db)
	tm := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)
	refresher := &concurrentTestRefresher{}

	_, err := tm.GetValidAccessToken(ctx, accountID, userID, refresher)
	if err == nil {
		t.Fatal("expected error for missing refresh token, got nil")
	}
	if !errors.Is(err, connectors.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
	}
	if calls := refresher.refreshCalls.Load(); calls != 0 {
		t.Errorf("refresher should not be called when refresh token is missing, got %d", calls)
	}
}

// 4. Refresh failure: transaction rolls back and original credentials remain
func TestTokenManager_RefreshFailure_Rollback(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	rf := "refresh-val"
	userID, accountID := createTestUserAndAccount(t, ctx, db, "orig-token", &rf, &past)

	accountRepo := storage.NewSocialAccountRepository(db)
	tm := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)
	refresher := &concurrentTestRefresher{failWith: connectors.ErrTokenRefreshFailed}

	_, err := tm.GetValidAccessToken(ctx, accountID, userID, refresher)
	if err == nil {
		t.Fatal("expected refresh error, got nil")
	}
	if !errors.Is(err, connectors.ErrTokenRefreshFailed) {
		t.Fatalf("expected ErrTokenRefreshFailed, got %v", err)
	}

	// Verify original credentials remain intact in DB
	acc, err := accountRepo.GetByIDAndUserID(ctx, userID, accountID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	dec, err := auth.DecryptToken(acc.AccessToken, testEncryptionKey)
	if err != nil || dec != "orig-token" {
		t.Errorf("expected orig-token preserved after failure, got %q, err=%v", dec, err)
	}
}

// 5. API and Worker sharing the same database coordination
func TestTokenManager_APIAndWorker_DatabaseCoordination(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()

	past := time.Now().Add(-10 * time.Minute)
	rf := "initial-refresh"
	userID, accountID := createTestUserAndAccount(t, ctx, db, "expired-token", &rf, &past)

	accountRepo := storage.NewSocialAccountRepository(db)

	// Simulate API TokenManager instance and Worker TokenManager instance
	apiTokenManager := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)
	workerTokenManager := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)

	refresher := &concurrentTestRefresher{delay: 40 * time.Millisecond}

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	var apiToken, workerToken string
	var apiErr, workerErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-startGate
		apiToken, apiErr = apiTokenManager.GetValidAccessToken(ctx, accountID, userID, refresher)
	}()
	go func() {
		defer wg.Done()
		<-startGate
		workerToken, workerErr = workerTokenManager.GetValidAccessToken(ctx, accountID, userID, refresher)
	}()

	close(startGate)
	wg.Wait()

	if apiErr != nil {
		t.Fatalf("API token manager error: %v", apiErr)
	}
	if workerErr != nil {
		t.Fatalf("Worker token manager error: %v", workerErr)
	}

	if apiToken != workerToken {
		t.Errorf("API (%q) and Worker (%q) got different tokens", apiToken, workerToken)
	}
	if apiToken != "refreshed-access-token-1" {
		t.Errorf("expected refreshed-access-token-1, got %q", apiToken)
	}

	// Exactly 1 external refresh must occur across both processes
	if calls := refresher.refreshCalls.Load(); calls != 1 {
		t.Fatalf("expected exactly 1 refresh call between API and Worker, got %d", calls)
	}
}

// 6. Overlapping concurrent refresh and force-refresh test
func TestTokenManager_ConcurrentRefresh_SingleExecution(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()

	past := time.Now().Add(-10 * time.Minute)
	rf := "old-refresh-token"
	userID, accountID := createTestUserAndAccount(t, ctx, db, "old-access-token", &rf, &past)

	accountRepo := storage.NewSocialAccountRepository(db)
	tm := service.NewTokenManagerService(accountRepo, db, testEncryptionKey)
	refresher := &concurrentTestRefresher{delay: 50 * time.Millisecond}

	// 2 concurrent requests for the same expired account
	const concurrency = 2
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	tokens := make([]string, concurrency)
	errorsList := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startGate
			tok, err := tm.GetValidAccessToken(ctx, accountID, userID, refresher)
			tokens[idx] = tok
			errorsList[idx] = err
		}(i)
	}

	close(startGate)
	wg.Wait()

	for i, err := range errorsList {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
		if tokens[i] != "refreshed-access-token-1" {
			t.Errorf("goroutine %d: expected refreshed-access-token-1, got %q", i, tokens[i])
		}
	}

	// Exactly 1 refresh call
	if calls := refresher.refreshCalls.Load(); calls != 1 {
		t.Fatalf("expected exactly 1 refresh call across concurrent requests, got %d", calls)
	}

	// Verify persisted database state
	acc, err := accountRepo.GetByIDAndUserID(ctx, userID, accountID)
	if err != nil {
		t.Fatalf("fetch refreshed account: %v", err)
	}
	decryptedAccess, err := auth.DecryptToken(acc.AccessToken, testEncryptionKey)
	if err != nil {
		t.Fatalf("decrypt persisted access token: %v", err)
	}
	if decryptedAccess != "refreshed-access-token-1" {
		t.Errorf("persisted access token want 'refreshed-access-token-1', got %q", decryptedAccess)
	}
	if acc.RefreshToken == nil {
		t.Fatal("expected rotated refresh token to be persisted")
	}
	decryptedRefresh, err := auth.DecryptToken(*acc.RefreshToken, testEncryptionKey)
	if err != nil {
		t.Fatalf("decrypt persisted refresh token: %v", err)
	}
	if decryptedRefresh != "refreshed-refresh-token-1" {
		t.Errorf("persisted refresh token want 'refreshed-refresh-token-1', got %q", decryptedRefresh)
	}

	// Concurrent ForceRefreshAccessToken with the rejected token
	refresherForce := &concurrentTestRefresher{delay: 50 * time.Millisecond, tokenPrefix: "force"}
	startGateForce := make(chan struct{})
	var wgForce sync.WaitGroup
	tokensForce := make([]string, concurrency)
	errorsForce := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wgForce.Add(1)
		go func(idx int) {
			defer wgForce.Done()
			<-startGateForce
			tok, err := tm.ForceRefreshAccessToken(ctx, accountID, userID, "refreshed-access-token-1", refresherForce)
			tokensForce[idx] = tok
			errorsForce[idx] = err
		}(i)
	}

	close(startGateForce)
	wgForce.Wait()

	for i, err := range errorsForce {
		if err != nil {
			t.Fatalf("force refresh goroutine %d failed: %v", i, err)
		}
		if tokensForce[i] != "force-access-token-1" {
			t.Errorf("force refresh goroutine %d: expected force-access-token-1, got %q", i, tokensForce[i])
		}
	}

	if calls := refresherForce.refreshCalls.Load(); calls != 1 {
		t.Fatalf("expected exactly 1 refresh call across concurrent force-refresh requests, got %d", calls)
	}

	// Final database state has the force-refreshed token
	accAfterForce, err := accountRepo.GetByIDAndUserID(ctx, userID, accountID)
	if err != nil {
		t.Fatalf("fetch force-refreshed account: %v", err)
	}
	decryptedAccessForce, err := auth.DecryptToken(accAfterForce.AccessToken, testEncryptionKey)
	if err != nil {
		t.Fatalf("decrypt persisted force access token: %v", err)
	}
	if decryptedAccessForce != "force-access-token-1" {
		t.Errorf("persisted force access token want 'force-access-token-1', got %q", decryptedAccessForce)
	}
}
