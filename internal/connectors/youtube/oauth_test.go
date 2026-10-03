package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamvalson/blink/internal/connectors"
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
