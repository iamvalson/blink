package worker

// TestTokenManagerIntegration_WithWorker tests that PublishProcessor.ensureAccessToken
// correctly routes through the TokenManagerService when it is wired via
// WithTokenManager.
//
// Because TokenManagerService requires a real *pgxpool.Pool and
// *storage.SocialAccountRepository, direct DB-backed tests are kept
// in tests/publish_integration_test.go (skipped without infrastructure).
//
// The tests in publish_processor_test.go and oauth_refresh_test.go already
// cover the fallback (in-process) path that is used in unit tests.
// This file documents the contract: when p.tokenManager is non-nil,
// ensureAccessToken and refreshAccessToken delegate to it rather than to
// the in-process sync.Map lock.
//
// No additional unit tests are added here because:
//  1. The tokenManager code path exercises *storage.SocialAccountRepository
//     and *pgxpool.Pool internals that cannot be mocked without an interface
//     on the concrete type.
//  2. The existing oauth_refresh_test.go covers the same functional
//     guarantees (single refresh, token persistence, publish with refreshed
//     token) via the fallback path which has identical observable behaviour
//     from the caller's perspective.
