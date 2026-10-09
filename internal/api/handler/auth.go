package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

const oauthTransactionLifetime = 10 * time.Minute

// OAuthTransaction holds all server-side state for one in-flight OAuth flow.
// It is created at initiation time (when the user is authenticated) and
// atomically consumed at callback time, so the callback never needs the
// auth_token cookie to identify the user.
type OAuthTransaction struct {
	// State is the cryptographically random OAuth state parameter.
	State string
	// UserID is the authenticated Blink user who initiated the flow.
	UserID string
	// Platform is the connector slug (e.g. "twitter", "youtube").
	Platform string
	// PKCEVerifier is the PKCE code_verifier. Only populated for platforms
	// that use PKCE (e.g. Twitter/X). Empty string means "no PKCE".
	PKCEVerifier string
	// ExpiresAt is when this transaction becomes invalid.
	ExpiresAt time.Time
}

// OAuthTransactionStore is the interface for storing and atomically consuming
// OAuth transactions. The in-memory implementation below satisfies this
// interface; a Redis-backed implementation can be substituted without changing
// the handler logic.
//
// Future work: implement a Redis-backed store for multi-instance deployments.
// The in-memory store works correctly for a single backend process (the common
// case during development and on a single production node), but will not share
// state across horizontally scaled replicas.
type OAuthTransactionStore interface {
	// Create stores a new OAuth transaction.
	Create(tx OAuthTransaction) error
	// Consume atomically retrieves and removes the transaction for the given
	// state token. Returns (tx, true) on success, or (zero, false) if the
	// state is unknown, expired, or already consumed.
	Consume(state string) (OAuthTransaction, bool)
}

// memOAuthTransactionStore is the default in-memory implementation of
// OAuthTransactionStore. It is safe for concurrent use.
type memOAuthTransactionStore struct {
	mu    sync.Mutex
	items map[string]OAuthTransaction
}

func newMemOAuthTransactionStore() *memOAuthTransactionStore {
	return &memOAuthTransactionStore{items: make(map[string]OAuthTransaction)}
}

func (s *memOAuthTransactionStore) Create(tx OAuthTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Opportunistic cleanup of expired transactions.
	now := time.Now()
	for k, v := range s.items {
		if now.After(v.ExpiresAt) {
			delete(s.items, k)
		}
	}

	s.items[tx.State] = tx
	return nil
}

func (s *memOAuthTransactionStore) Consume(state string) (OAuthTransaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, exists := s.items[state]
	if !exists {
		return OAuthTransaction{}, false
	}

	// Atomically remove the transaction so it can never be reused.
	delete(s.items, state)

	if time.Now().After(tx.ExpiresAt) {
		// Expired — treat as not found (already deleted above).
		return OAuthTransaction{}, false
	}

	return tx, true
}

// AuthHandler handles the browser-redirect OAuth flow for any platform that
// implements connectors.OAuthConnector. It has no dependency on concrete
// platform packages — adding a new platform only requires registering its
// connector in the router.
type AuthHandler struct {
	connector     connectors.OAuthConnector
	accounts      *storage.SocialAccountRepository
	encryptionKey string
	frontendURL   string
	secureCookie  bool
	store         OAuthTransactionStore
}

// NewAuthHandler creates a handler for a single OAuth platform.
func NewAuthHandler(
	connector connectors.OAuthConnector,
	accounts *storage.SocialAccountRepository,
	encryptionKey string,
	frontendURL string,
	secureCookie bool,
) *AuthHandler {
	return &AuthHandler{
		connector:     connector,
		accounts:      accounts,
		encryptionKey: encryptionKey,
		frontendURL:   frontendURL,
		secureCookie:  secureCookie,
		store:         newMemOAuthTransactionStore(),
	}
}

// NewAuthHandlerWithStore creates a handler with an injected transaction store.
// This is used in tests to inject a custom store, or in production to inject
// a Redis-backed store.
func NewAuthHandlerWithStore(
	connector connectors.OAuthConnector,
	accounts *storage.SocialAccountRepository,
	encryptionKey string,
	frontendURL string,
	secureCookie bool,
	store OAuthTransactionStore,
) *AuthHandler {
	return &AuthHandler{
		connector:     connector,
		accounts:      accounts,
		encryptionKey: encryptionKey,
		frontendURL:   frontendURL,
		secureCookie:  secureCookie,
		store:         store,
	}
}

// OAuthStart redirects the authenticated user to the platform's authorization
// page. It requires a valid Blink session (auth_token cookie) because the user
// must be identified at initiation time. The authenticated user ID is stored
// inside the server-side transaction so the callback does not need the cookie.
func (h *AuthHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	// User must be authenticated to initiate OAuth.
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	state, err := generateRandomState()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate OAuth state")
		http.Error(w, "Unable to start authentication", http.StatusInternalServerError)
		return
	}

	platform := h.connector.PlatformID()

	// Generate a PKCE verifier. For connectors that do not use PKCE (YouTube),
	// AuthCodeURL ignores the verifier argument. We always generate one so the
	// transaction struct is uniform; the connector decides whether to use it.
	verifier := oauth2.GenerateVerifier()

	tx := OAuthTransaction{
		State:        state,
		UserID:       userID,
		Platform:     platform,
		PKCEVerifier: verifier,
		ExpiresAt:    time.Now().Add(oauthTransactionLifetime),
	}
	if err := h.store.Create(tx); err != nil {
		log.Error().Err(err).Str("platform", platform).Str("user_id", userID).
			Msg("Failed to store OAuth transaction")
		http.Error(w, "Unable to start authentication", http.StatusInternalServerError)
		return
	}

	authURL := h.connector.AuthCodeURL(state, verifier)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// TwitterAuth is an alias for OAuthStart for backward compatibility.
func (h *AuthHandler) TwitterAuth(w http.ResponseWriter, r *http.Request) {
	h.OAuthStart(w, r)
}

// OAuthCallback handles the provider callback. It no longer requires the
// auth_token cookie — user identity is resolved through the server-side
// OAuth transaction, which was created at initiation by the authenticated user.
//
// Security properties:
//   - Unknown state → 400 (no transaction)
//   - Expired state → 400 (transaction consumed but time.After check fails)
//   - Reused state → 400 (transaction already deleted on first consume)
//   - Wrong provider → 400 (platform mismatch check)
//   - Missing code → 400
//   - Provider OAuth error → 400
//   - Invalid PKCE → provider returns error during exchange
func (h *AuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	platform := h.connector.PlatformID()

	// 1. Check for provider-reported OAuth errors (e.g. user denied access).
	if errorCode := r.URL.Query().Get("error"); errorCode != "" {
		log.Info().
			Str("oauth_error", errorCode).
			Str("platform", platform).
			Msg("OAuth authorization was denied by provider or user")
		http.Error(w, fmt.Sprintf("%s authorization was denied", platform), http.StatusBadRequest)
		return
	}

	// 2. Validate required parameters.
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "Missing OAuth state", http.StatusBadRequest)
		return
	}

	// 3. Atomically consume the transaction.
	//    This is the single point where the state is validated, the transaction
	//    is removed, and the user identity is recovered — all under one lock.
	tx, ok := h.store.Consume(state)
	if !ok {
		log.Warn().
			Str("platform", platform).
			Msg("OAuth callback: unknown, expired, or already-consumed state")
		http.Error(w, "Invalid or expired OAuth state", http.StatusBadRequest)
		return
	}

	// 4. Verify that the state is bound to the correct platform.
	//    Prevents a Twitter state from being accepted by the YouTube callback.
	if tx.Platform != platform {
		log.Warn().
			Str("transaction_platform", tx.Platform).
			Str("callback_platform", platform).
			Str("user_id", tx.UserID).
			Msg("OAuth callback: platform mismatch")
		http.Error(w, "OAuth state platform mismatch", http.StatusBadRequest)
		return
	}

	userID := tx.UserID

	// 5. Exchange the authorization code for tokens.
	//    The PKCE verifier is passed for platforms that require it (Twitter).
	//    Connectors that do not use PKCE (YouTube) ignore the verifier field.
	authResult, err := h.connector.Authenticate(r.Context(), connectors.AuthParams{
		Code:         code,
		CodeVerifier: tx.PKCEVerifier,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("platform", platform).
			Msg("OAuth authentication failed during token exchange")
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("user_id", userID).
		Str("platform", platform).
		Str("platform_user_id", authResult.PlatformUserID).
		Msg("OAuth authentication successful")

	// 6. Encrypt tokens before persisting.
	encryptedAccessToken, err := auth.EncryptToken(authResult.AccessToken, h.encryptionKey)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("platform", platform).
			Msg("Failed to encrypt access token")
		http.Error(w, "Failed to secure access token", http.StatusInternalServerError)
		return
	}

	// Refresh tokens may not always be returned.
	// Keep empty when the platform does not provide one so the repository
	// preserves the existing refresh token during an upsert.
	encryptedRefreshToken := ""
	if authResult.RefreshToken != "" {
		encryptedRefreshToken, err = auth.EncryptToken(authResult.RefreshToken, h.encryptionKey)
		if err != nil {
			log.Error().Err(err).Str("user_id", userID).Str("platform", platform).
				Msg("Failed to encrypt refresh token")
			http.Error(w, "Failed to secure refresh token", http.StatusInternalServerError)
			return
		}
	}

	if h.accounts == nil {
		log.Error().Str("user_id", userID).Msg("Social account repository is unavailable")
		http.Error(w, "Account storage is unavailable", http.StatusInternalServerError)
		return
	}

	// 7. Upsert the social account.
	//    The ownership is taken from transaction.UserID — never from the
	//    provider callback or a query parameter.
	if err := h.accounts.Upsert(
		r.Context(),
		userID,
		platform,
		authResult.PlatformUserID,
		encryptedAccessToken,
		encryptedRefreshToken,
		authResult.Expiry,
	); err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("platform", platform).
			Msg("Failed to save social account")
		http.Error(w, fmt.Sprintf("Failed to save %s account", platform), http.StatusInternalServerError)
		return
	}

	// 8. Redirect to the configured frontend path.
	//    The redirect destination is always server-configured (FRONTEND_URL);
	//    no user-supplied redirect parameter is accepted (open-redirect prevention).
	http.Redirect(w, r, fmt.Sprintf("%s/accounts?%s=connected", h.frontendURL, platform), http.StatusSeeOther)
}

// TwitterCallback is an alias for OAuthCallback for backward compatibility.
func (h *AuthHandler) TwitterCallback(w http.ResponseWriter, r *http.Request) {
	h.OAuthCallback(w, r)
}

// generateRandomState generates a cryptographically secure OAuth state token.
// It uses 32 bytes from crypto/rand, which provides 256 bits of entropy —
// far in excess of the minimum required to resist brute-force guessing.
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
