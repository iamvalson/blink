package tiktok

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func TestNew(t *testing.T) {
	cfg := TikTokConfig{
		ClientKey:    "test-key",
		ClientSecret: "test-secret",
		RedirectURI:  "https://example.com/callback",
		Scopes:       []string{"user.info.basic"},
	}
	conn := New(cfg)
	if conn == nil {
		t.Fatal("failed to create TikTok connector")
	}
}

func TestNew_WithCustomClient(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	conn := New(TikTokConfig{}, customClient)
	if conn.httpClient != customClient {
		t.Errorf("expected custom HTTP client to be set")
	}
}

func TestConnector_PlatformID(t *testing.T) {
	conn := New(TikTokConfig{})
	if id := conn.PlatformID(); id != connectors.PlatformTikTok {
		t.Errorf("expected platform ID %s, got %s", connectors.PlatformTikTok, id)
	}
}

func TestConnector_AuthCodeURL(t *testing.T) {
	cfg := TikTokConfig{
		ClientKey:    "test-client-key",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/callback",
		Scopes:       []string{"user.info.basic", "video.list"},
	}
	conn := New(cfg)

	url, err := conn.AuthCodeURL("random-state", "random-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(url, "random-state") {
		t.Errorf("expected AuthCodeURL to contain state, got: %s", url)
	}
	if !strings.Contains(url, "test-client-key") {
		t.Errorf("expected AuthCodeURL to contain client_key, got: %s", url)
	}
}

func TestConnector_AuthCodeURL_Error(t *testing.T) {
	cfg := TikTokConfig{
		ClientKey: "test-key",
	}
	conn := New(cfg)
	conn.oauthConfig.Endpoint.AuthURL = "://invalid"

	_, err := conn.AuthCodeURL("state", "verifier")
	if err == nil {
		t.Fatal("expected error for invalid endpoint URL")
	}
}

func TestConnector_SetAccessToken(t *testing.T) {
	conn := New(TikTokConfig{})
	conn.SetAccessToken("my-access-token")
	if conn.accessToken != "my-access-token" {
		t.Errorf("expected access token 'my-access-token', got %s", conn.accessToken)
	}
}

func TestConnector_Authenticate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type": "Bearer",
			"expires_in": 3600,
			"open_id": "tiktok-user-123",
			"scope": "user.info.basic"
		}`))
	}))
	defer server.Close()

	conn := New(TikTokConfig{
		ClientKey:    "key",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost/callback",
	})
	conn.oauthConfig.Endpoint.TokenURL = server.URL

	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "auth-code"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.PlatformUserID != "tiktok-user-123" {
		t.Errorf("expected PlatformUserID tiktok-user-123, got %s", res.PlatformUserID)
	}
	if res.AccessToken != "test-access-token" {
		t.Errorf("expected AccessToken test-access-token, got %s", res.AccessToken)
	}
	if res.RefreshToken != "test-refresh-token" {
		t.Errorf("expected RefreshToken test-refresh-token, got %s", res.RefreshToken)
	}
	if res.Expiry.Before(time.Now()) {
		t.Error("expected future token expiry")
	}
	if conn.accessToken != "test-access-token" {
		t.Errorf("expected connector accessToken to be updated, got %s", conn.accessToken)
	}
}

func TestConnector_Authenticate_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()

	conn := New(TikTokConfig{
		ClientKey:    "key",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost/callback",
	})
	conn.oauthConfig.Endpoint.TokenURL = server.URL

	_, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "bad-code"})
	if err == nil {
		t.Fatal("expected error on failed exchange")
	}
}

func TestConnector_Authenticate_MissingOpenID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "token-without-openid",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	conn := New(TikTokConfig{})
	conn.oauthConfig.Endpoint.TokenURL = server.URL

	_, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "valid-code"})
	if err == nil || !strings.Contains(err.Error(), "missing open_id") {
		t.Fatalf("expected missing open_id error, got: %v", err)
	}
}

func TestConnector_Authenticate_InjectedClient(t *testing.T) {
	var clientCalled bool
	customClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			clientCalled = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"access_token": "custom-client-token",
					"expires_in": 3600,
					"open_id": "custom-client-user"
				}`)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, customClient)
	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "code"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clientCalled {
		t.Fatal("expected injected HTTP client to be called during Authenticate")
	}
	if res.PlatformUserID != "custom-client-user" {
		t.Errorf("expected custom-client-user, got %s", res.PlatformUserID)
	}
}

func TestConnector_UnimplementedMethods(t *testing.T) {
	conn := New(TikTokConfig{})
	ctx := context.Background()

	_, err := conn.UploadMedia(ctx, "token", "attempt", bytes.NewReader([]byte("data")), "video/mp4")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for UploadMedia, got %v", err)
	}

	_, _, err = conn.Publish(ctx, "token", "attempt", "caption")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for Publish, got %v", err)
	}

	_, _, err = conn.GetStatus(ctx, "post-id")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for GetStatus, got %v", err)
	}

	res, err := conn.ReconcilePublish(ctx, "token", "user", "attempt", "caption")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for ReconcilePublish, got %v", err)
	}
	if res.Outcome != connectors.ReconciliationUnknown {
		t.Errorf("expected ReconciliationUnknown, got %s", res.Outcome)
	}
}
