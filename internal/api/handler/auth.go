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

// AuthHandler handles the browser-redirect OAuth flow for any platform that
// implements connectors.OAuthConnector. It has no dependency on concrete
// platform packages — adding a new platform only requires registering its
// connector in the router.
type AuthHandler struct {
	connector      connectors.OAuthConnector
	accounts       *storage.SocialAccountRepository
	encryptionKey  string
	transactions   map[string]oauthTransaction
	transactionsMu sync.Mutex
}

type oauthTransaction struct {
	verifier  string
	expiresAt time.Time
}

// NewAuthHandler creates a handler for a single OAuth platform.
func NewAuthHandler(connector connectors.OAuthConnector, accounts *storage.SocialAccountRepository, encryptionKey string) *AuthHandler {
	return &AuthHandler{
		connector:     connector,
		accounts:      accounts,
		encryptionKey: encryptionKey,
		transactions:  make(map[string]oauthTransaction),
	}
}

// OAuthStart redirects the user to the platform's authorization page.
func (h *AuthHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	state, err := generateRandomState()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate OAuth state")
		http.Error(w, "Unable to start authentication", http.StatusInternalServerError)
		return
	}

	verifier := oauth2.GenerateVerifier()
	now := time.Now()

	h.transactionsMu.Lock()
	for storedState, tx := range h.transactions {
		if now.After(tx.expiresAt) {
			delete(h.transactions, storedState)
		}
	}
	h.transactions[state] = oauthTransaction{
		verifier:  verifier,
		expiresAt: now.Add(oauthTransactionLifetime),
	}
	h.transactionsMu.Unlock()

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthTransactionLifetime.Seconds()),
	})

	authURL := h.connector.AuthCodeURL(state, verifier)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// TwitterAuth is an alias for OAuthStart for backward compatibility.
func (h *AuthHandler) TwitterAuth(w http.ResponseWriter, r *http.Request) {
	h.OAuthStart(w, r)
}

// OAuthCallback handles the OAuth callback for any platform.
func (h *AuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	defer clearOAuthCookies(w, r)

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User is not authenticated", http.StatusUnauthorized)
		return
	}

	platform := h.connector.PlatformID()

	if errorCode := r.URL.Query().Get("error"); errorCode != "" {
		log.Info().
			Str("oauth_error", errorCode).
			Str("user_id", userID).
			Str("platform", platform).
			Msg("OAuth authorization was denied")
		http.Error(w, fmt.Sprintf("%s authorization was denied", platform), http.StatusBadRequest)
		return
	}

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

	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Error(w, "Missing OAuth state cookie", http.StatusBadRequest)
		return
	}

	if !secureCompare(stateCookie.Value, state) {
		http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
		return
	}

	transaction, ok := h.consumeTransaction(state)
	if !ok {
		http.Error(w, "Invalid or expired OAuth state", http.StatusBadRequest)
		return
	}

	authResult, err := h.connector.Authenticate(r.Context(), connectors.AuthParams{
		Code:         code,
		CodeVerifier: transaction.verifier,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("platform", platform).
			Msg("OAuth authentication failed")
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("user_id", userID).
		Str("platform", platform).
		Str("platform_user_id", authResult.PlatformUserID).
		Msg("OAuth authentication successful")

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

	if err := h.accounts.Upsert(r.Context(), userID, platform, authResult.PlatformUserID, encryptedAccessToken, encryptedRefreshToken, authResult.Expiry); err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("platform", platform).
			Msg("Failed to save social account")
		http.Error(w, fmt.Sprintf("Failed to save %s account", platform), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard?%s=connected", platform), http.StatusSeeOther)
}

// TwitterCallback is an alias for OAuthCallback for backward compatibility.
func (h *AuthHandler) TwitterCallback(w http.ResponseWriter, r *http.Request) {
	h.OAuthCallback(w, r)
}

// clearOAuthCookies removes the temporary OAuth state cookies.
func clearOAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	for _, name := range []string{"oauth_state", "oauth_verifier"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}
}

// consumeTransaction retrieves and removes an OAuth transaction (single-use).
func (h *AuthHandler) consumeTransaction(state string) (oauthTransaction, bool) {
	h.transactionsMu.Lock()
	defer h.transactionsMu.Unlock()

	tx, exists := h.transactions[state]
	if !exists {
		return oauthTransaction{}, false
	}
	delete(h.transactions, state)

	if time.Now().After(tx.expiresAt) {
		return oauthTransaction{}, false
	}
	return tx, true
}

// generateRandomState generates a cryptographically secure OAuth state token.
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// secureCompare performs a constant-time string comparison.
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

