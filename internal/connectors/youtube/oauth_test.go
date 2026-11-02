package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

func TestRefreshTokenPreservesRotationSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "existing" {
			t.Fatalf("unexpected refresh form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"google-access","expires_in":1800}`))
	}))
	defer server.Close()

	connector := New(YouTubeConfig{ClientID: "client", ClientSecret: "secret"})
	connector.oauthConfig.Endpoint.TokenURL = server.URL + "/token"
	result, err := connector.RefreshToken(context.Background(), "existing")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if result.AccessToken != "google-access" || result.RefreshToken != nil || result.ExpiresAt == nil {
		t.Fatalf("unexpected token result: %+v", result)
	}
}

func TestRefreshTokenInvalidGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()

	connector := New(YouTubeConfig{})
	connector.oauthConfig.Endpoint.TokenURL = server.URL
	_, err := connector.RefreshToken(context.Background(), "revoked")
	if err != connectors.ErrInvalidRefreshToken {
		t.Fatalf("expected invalid refresh token, got %v", err)
	}
}

func TestGetAuthURL_Parameters(t *testing.T) {
	cfg := YouTubeConfig{
		ClientID:     "yt-client-id",
		ClientSecret: "yt-secret",
		CallbackURL:  "https://example.com/yt-callback",
	}
	oauthCfg := NewOAuthConfig(cfg)
	state := "yt-random-state"

	authURL := GetAuthURL(oauthCfg, state)
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	if u.Scheme != "https" || u.Host != "accounts.google.com" || u.Path != "/o/oauth2/v2/auth" {
		t.Fatalf("unexpected auth endpoint: %s", authURL)
	}

	q := u.Query()
	if q.Get("client_id") != "yt-client-id" {
		t.Errorf("client_id = %s, want yt-client-id", q.Get("client_id"))
	}
	if q.Get("state") != state {
		t.Errorf("state = %s, want %s", q.Get("state"), state)
	}
	if q.Get("access_type") != "offline" {
		t.Errorf("access_type = %s, want offline", q.Get("access_type"))
	}
	if q.Get("prompt") != "consent" && q.Get("approval_prompt") != "force" {
		t.Errorf("expected consent prompt parameter in %s", authURL)
	}

	scope := q.Get("scope")
	for _, expectedScope := range oauthCfg.Scopes {
		if !strings.Contains(scope, expectedScope) {
			t.Errorf("missing scope %s in auth URL", expectedScope)
		}
	}
}

func TestExchangeCodeForToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = r.ParseForm()
		if r.Form.Get("code") != "google-auth-code" {
			t.Errorf("code = %s, want google-auth-code", r.Form.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "yt-access",
			"refresh_token": "yt-refresh",
			"expires_in": 3600,
			"token_type": "Bearer"
		}`))
	}))
	defer server.Close()

	oauthCfg := NewOAuthConfig(YouTubeConfig{ClientID: "cid", CallbackURL: "http://localhost/cb"})
	oauthCfg.Endpoint.TokenURL = server.URL

	token, err := ExchangeCodeForToken(context.Background(), oauthCfg, "google-auth-code")
	if err != nil {
		t.Fatalf("ExchangeCodeForToken failed: %v", err)
	}
	if token.AccessToken != "yt-access" {
		t.Errorf("AccessToken = %s, want yt-access", token.AccessToken)
	}
	if token.RefreshToken != "yt-refresh" {
		t.Errorf("RefreshToken = %s, want yt-refresh", token.RefreshToken)
	}
}

func TestGetChannelInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer yt-tok-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": [
				{
					"id": "UC_test_channel_id",
					"snippet": {"title": "Test Title", "description": "Test Desc"}
				}
			]
		}`))
	}))
	defer server.Close()

	origURL := channelInfoURL
	channelInfoURL = server.URL
	defer func() { channelInfoURL = origURL }()

	token := &oauth2.Token{AccessToken: "yt-tok-123"}
	info, err := GetChannelInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("GetChannelInfo failed: %v", err)
	}
	if info.ID != "UC_test_channel_id" {
		t.Errorf("info.ID = %s, want UC_test_channel_id", info.ID)
	}
	if info.Snippet.Title != "Test Title" {
		t.Errorf("info.Snippet.Title = %s, want Test Title", info.Snippet.Title)
	}
}

func TestGetChannelInfo_Errors(t *testing.T) {
	// 403 API Error
	server403 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Quota Exceeded"}}`))
	}))
	defer server403.Close()

	origURL := channelInfoURL
	defer func() { channelInfoURL = origURL }()

	channelInfoURL = server403.URL
	token := &oauth2.Token{AccessToken: "tok"}
	_, err := GetChannelInfo(context.Background(), token)
	if err == nil {
		t.Fatal("expected error on 403 status")
	}

	// Empty items
	serverEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer serverEmpty.Close()

	channelInfoURL = serverEmpty.URL
	_, err = GetChannelInfo(context.Background(), token)
	if err == nil || !strings.Contains(err.Error(), "no YouTube channel found") {
		t.Fatalf("expected no channel found error, got %v", err)
	}
}
