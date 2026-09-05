package handler

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

const oauthTransactionLifetime = 10 * time.Minute

type AuthHandler struct {
	twitterConnector *twitter.Connector
	accounts         *storage.SocialAccountRepository
	encryptionKey    string
	transactions     map[string]oauthTransaction
	transactionsMu   sync.Mutex
}

type oauthTransaction struct {
	verifier	string
	expiresAt	time.Time
}

func NewAuthHandler(tc *twitter.Connector, accounts *storage.SocialAccountRepository, encryptionKey string) *AuthHandler {
	return &AuthHandler{
		twitterConnector: tc,
		accounts:         accounts,
		encryptionKey:    encryptionKey,
		transactions:     make(map[string]oauthTransaction),
	}
}

// TwitterAuth redirects user to Twitter OAuth
func (h *AuthHandler) TwitterAuth(w http.ResponseWriter, r *http.Request) {
	// Generate random state for CSRF protection
	state, err := generateRandomState()
	if err != nil {
		http.Error(w, "Unable to start authentication", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()
	now := time.Now()

	h.transactionsMu.Lock()
	
	for storedState, transaction := range h.transactions{
		if now.After(transaction.expiresAt) {
			delete(h.transactions, storedState)
		}
	}

	h.transactions[state] = oauthTransaction{
		verifier: verifier,
		expiresAt: now.Add(oauthTransactionLifetime),
	}

	h.transactionsMu.Unlock()

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	
	

	// Store state in session/cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthTransactionLifetime.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Value:    verifier,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthTransactionLifetime.Seconds()),
	})

	// Redirect to Twitter
	authURL := twitter.GetAuthURL(h.twitterConnector.GetOAuthConfig(), state, verifier)
	http.Redirect(w, r, authURL, http.StatusFound)
}




// TwitterCallback handles OAuth callback
func (h *AuthHandler) TwitterCallback(w http.ResponseWriter, r *http.Request) {
	// Always remove temporary OAuth cookies after the callback finishes,
    // whether it succeeds or fails.
    defer clearOAuthCookies(w, r)

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "User is not authenticated", http.StatusUnauthorized)
		return
	}

	// Twitter sends this when the user rejects authorization.
    if errorCode := r.URL.Query().Get("error"); errorCode != "" {
        log.Info().
            Str("oauth_error", errorCode).
            Str("user_id", userID).
            Msg("Twitter authorization was denied")

        http.Error(w, "Twitter authorization was denied", http.StatusBadRequest)
        return
    }

	// Get Auth code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Verify state and recover the PKCE verifier from the server-side transaction.
	state := r.URL.Query().Get("state")
	if state == "" {
        http.Error(w, "Missing OAuth state", http.StatusBadRequest)
        return
    }

	// Validate the callback state against the browser cookie.
    stateCookie, err := r.Cookie("oauth_state")
    if err != nil || stateCookie.Value != state {
        http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
        return
    }

	// Look up and consume the server-side transaction.
    h.transactionsMu.Lock()
    transaction, exists := h.transactions[state]

    if exists {
        delete(h.transactions, state)
    }

    h.transactionsMu.Unlock()

    if !exists || time.Now().After(transaction.expiresAt) {
        http.Error(w, "Invalid or expired state", http.StatusBadRequest)
        return
    }

	// Exchange code for token
	authResult, err := h.twitterConnector.Authenticate(r.Context(), connectors.AuthParams{
		Code: code,
		CodeVerifier: transaction.verifier,	
	})
	if err != nil {
		log.Error().Err(err).Msg("Twitter authentication failed")
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	log.Info().Str("platform_user_id", authResult.PlatformUserID).Msg("Twitter auth successful")



	encryptedAccessToken, err := auth.EncryptToken(authResult.AccessToken, h.encryptionKey)
	if err != nil{
		http.Error(w, "Failed to secure access token", http.StatusInternalServerError)
		return
	}

	encryptedRefreshToken := ""
	if authResult.RefreshToken != "" {
		encryptedRefreshToken, err = auth.EncryptToken(authResult.RefreshToken, h.encryptionKey)
		if err != nil{
			http.Error(w, "Failed to secure refresh token", http.StatusInternalServerError)
			return
		}
	}

	if h.accounts == nil {
		http.Error(w, "Account storage is unavailable", http.StatusInternalServerError)
		return
	}
	if err := h.accounts.Upsert(r.Context(), userID, authResult.PlatformUserID, encryptedAccessToken, encryptedRefreshToken, authResult.Expiry); err != nil {
		log.Error().Err(err).Msg("Failed to save Twitter account")
		http.Error(w, "Failed to save Twitter account", http.StatusInternalServerError)
		return
	}


	// TODO: Redirect to dashboard

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Authentication successful! Redirect to dashboard...")); err != nil {
		log.Error().Err(err).Msg("Failed to write authentication response")
	}
}

func clearOAuthCookies(w http.ResponseWriter, r *http.Request) {
    secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

    for _, name := range []string{
        "oauth_state",
        "oauth_verifier",
    } {
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

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
