package twitter

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

func TestRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("unexpected refresh form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer server.Close()

	connector := New(TwitterConfig{ClientID: "client", ClientSecret: "secret"})
	connector.oauthConfig.Endpoint.TokenURL = server.URL + "/token"
	result, err := connector.RefreshToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken == nil || *result.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected token result: %+v", result)
	}
	if result.ExpiresAt == nil || time.Until(*result.ExpiresAt) <= 0 {
		t.Fatal("expected a future expiry")
	}
}

func TestRefreshTokenInvalidGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()

	connector := New(TwitterConfig{})
	connector.oauthConfig.Endpoint.TokenURL = server.URL
	_, err := connector.RefreshToken(context.Background(), "revoked")
	if err != connectors.ErrInvalidRefreshToken {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
}

func TestOAuthConfigScopes(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		CallbackURL:  "https://example.com/callback",
	}
	oauthCfg := NewOAuthConfig(cfg)
	expectedScopes := []string{"tweet.read", "tweet.write", "users.read", "offline.access", "media.write"}
	if len(oauthCfg.Scopes) != len(expectedScopes) {
		t.Fatalf("expected %d scopes, got %d: %v", len(expectedScopes), len(oauthCfg.Scopes), oauthCfg.Scopes)
	}
	for i, scope := range expectedScopes {
		if oauthCfg.Scopes[i] != scope {
			t.Errorf("scope[%d] = %s, want %s", i, oauthCfg.Scopes[i], scope)
		}
	}
}

func TestGetAuthURL_PKCEChallenge(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "client-123",
		ClientSecret: "secret-456",
		CallbackURL:  "https://example.com/callback",
	}
	oauthCfg := NewOAuthConfig(cfg)
	state := "state-abc"
	verifier := "plain-verifier-string-1234567890"

	rawURL := GetAuthURL(oauthCfg, state, verifier)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "x.com" || parsed.Path != "/i/oauth2/authorize" {
		t.Fatalf("unexpected endpoint: %s", rawURL)
	}

	q := parsed.Query()
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %s, want code", q.Get("response_type"))
	}
	if q.Get("client_id") != "client-123" {
		t.Errorf("client_id = %s, want client-123", q.Get("client_id"))
	}
	if q.Get("state") != state {
		t.Errorf("state = %s, want %s", q.Get("state"), state)
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %s, want S256", q.Get("code_challenge_method"))
	}

	// Verify S256 challenge calculation
	h := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if q.Get("code_challenge") != expectedChallenge {
		t.Errorf("code_challenge = %s, want %s", q.Get("code_challenge"), expectedChallenge)
	}
}

func TestExchangeCodeForToken(t *testing.T) {
	var receivedVerifier string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = r.ParseForm()
		receivedVerifier = r.Form.Get("code_verifier")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "tw-access-token",
			"refresh_token": "tw-refresh-token",
			"token_type": "bearer",
			"expires_in": 7200,
			"scope": "tweet.read tweet.write"
		}`))
	}))
	defer server.Close()

	oauthCfg := NewOAuthConfig(TwitterConfig{ClientID: "cid", CallbackURL: "http://localhost/cb"})
	oauthCfg.Endpoint.TokenURL = server.URL

	token, err := ExchangeCodeForToken(context.Background(), oauthCfg, "code-123", "my-pkce-verifier")
	if err != nil {
		t.Fatalf("ExchangeCodeForToken failed: %v", err)
	}

	if receivedVerifier != "my-pkce-verifier" {
		t.Errorf("code_verifier = %s, want my-pkce-verifier", receivedVerifier)
	}
	if token.AccessToken != "tw-access-token" {
		t.Errorf("access_token = %s, want tw-access-token", token.AccessToken)
	}
	if token.RefreshToken != "tw-refresh-token" {
		t.Errorf("refresh_token = %s, want tw-refresh-token", token.RefreshToken)
	}
}

func TestGetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"tw-user-999","name":"Test User","username":"testuser"}}`))
	}))
	defer server.Close()

	origURL := userInfoURL
	userInfoURL = server.URL
	defer func() { userInfoURL = origURL }()

	token := &oauth2.Token{AccessToken: "test-tok"}
	info, err := GetUserInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("GetUserInfo failed: %v", err)
	}
	if info.ID != "tw-user-999" {
		t.Errorf("info.ID = %s, want tw-user-999", info.ID)
	}
	if info.Username != "testuser" {
		t.Errorf("info.Username = %s, want testuser", info.Username)
	}
}

func TestGetUserInfo_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Forbidden"}]}`))
	}))
	defer server.Close()

	origURL := userInfoURL
	userInfoURL = server.URL
	defer func() { userInfoURL = origURL }()

	token := &oauth2.Token{AccessToken: "bad-tok"}
	_, err := GetUserInfo(context.Background(), token)
	if err == nil {
		t.Fatal("expected error on 403 status")
	}

	// Missing ID in response
	serverEmptyID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"name":"No ID User"}}`))
	}))
	defer serverEmptyID.Close()

	userInfoURL = serverEmptyID.URL
	_, err = GetUserInfo(context.Background(), token)
	if err == nil || !strings.Contains(err.Error(), "missing user id") {
		t.Fatalf("expected missing user id error, got %v", err)
	}
}

func TestGetUserInfo_InjectedClient(t *testing.T) {
	var customCalled bool
	customClient := &http.Client{
		Transport: roundTripFuncTwitter(func(r *http.Request) (*http.Response, error) {
			customCalled = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"custom-user-1","username":"custom"}}`)),
			}, nil
		}),
	}

	token := &oauth2.Token{AccessToken: "some-tok"}
	info, err := GetUserInfo(context.Background(), token, customClient)
	if err != nil {
		t.Fatalf("GetUserInfo failed: %v", err)
	}
	if !customCalled {
		t.Fatal("expected injected client to be used for GetUserInfo")
	}
	if info.ID != "custom-user-1" {
		t.Errorf("info.ID = %s, want custom-user-1", info.ID)
	}
}

type roundTripFuncTwitter func(*http.Request) (*http.Response, error)

func (f roundTripFuncTwitter) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
