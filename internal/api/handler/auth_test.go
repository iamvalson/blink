package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamvalson/blink/internal/connectors/twitter"
)

func TestTwitterAuthSetsSecureOAuthCookiesBehindTLSProxy(t *testing.T) {
	connector := twitter.New(twitter.TwitterConfig{
		ClientID:    "test-client",
		CallbackURL: "https://example.com/auth/twitter/callback",
	})
	handler := NewAuthHandler(connector, nil, "")
	req := httptest.NewRequest(http.MethodGet, "https://example.com/auth/twitter", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	handler.TwitterAuth(recorder, req)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected state cookie, got %d", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.Name != "oauth_state" {
			t.Fatalf("expected oauth_state cookie, got %q", cookie.Name)
		}
		if cookie.Value == "" {
			t.Fatalf("cookie %q has an empty value", cookie.Name)
		}
		if !cookie.Secure {
			t.Errorf("cookie %q should be Secure behind an HTTPS proxy", cookie.Name)
		}
	}
}
