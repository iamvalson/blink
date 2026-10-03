package twitter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
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
