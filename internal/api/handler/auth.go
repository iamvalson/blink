package handler

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"

	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

type AuthHandler struct {
	twitterConnector *twitter.Connector
	transactions     map[string]string
	transactionsMu   sync.Mutex
}

func NewAuthHandler(tc *twitter.Connector) *AuthHandler {
	return &AuthHandler{
		twitterConnector: tc,
		transactions:     make(map[string]string),
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
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	h.transactionsMu.Lock()
	h.transactions[state] = verifier
	h.transactionsMu.Unlock()

	// Store state in session/cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Value:    verifier,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	// Redirect to Twitter
	authURL := twitter.GetAuthURL(h.twitterConnector.GetOAuthConfig(), state, verifier)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// TwitterCallback handles OAuth callback
func (h *AuthHandler) TwitterCallback(w http.ResponseWriter, r *http.Request) {
	// Get Auth code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Verify state and recover the PKCE verifier from the server-side transaction.
	state := r.URL.Query().Get("state")
	h.transactionsMu.Lock()
	verifier, ok := h.transactions[state]
	if ok {
		delete(h.transactions, state)
	}
	h.transactionsMu.Unlock()
	if !ok {
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}

	// Exchange code for token
	platformUserID, err := h.twitterConnector.Authenticate(r.Context(), code, verifier)
	if err != nil {
		log.Error().Err(err).Msg("Twitter authentication failed")
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	log.Info().Str("platform_user_id", platformUserID).Msg("Twitter auth successful")

	// TODO: Save encrypted token to DB
	// TODO: Create/update social_accounts record
	// TODO: Set user session cookie
	// TODO: Redirect to dashboard

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Authentication successful! Redirect to dashboard...")); err != nil {
		log.Error().Err(err).Msg("Failed to write authentication response")
	}
}

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
