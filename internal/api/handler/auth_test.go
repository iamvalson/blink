package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/middleware"
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

func TestTwitterCallbackUnauthenticatedUser(t *testing.T) {
	connector := twitter.New(twitter.TwitterConfig{
		ClientID:    "test-client",
		CallbackURL: "https://example.com/auth/twitter/callback",
	})
	handler := NewAuthHandler(connector, nil, "")

	req := httptest.NewRequest(http.MethodGet, "https://example.com/auth/twitter/callback?code=abc&state=xyz", nil)
	recorder := httptest.NewRecorder()

	handler.TwitterCallback(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", recorder.Code)
	}
}

func TestTwitterCallbackOAuthDenied(t *testing.T) {
	connector := twitter.New(twitter.TwitterConfig{
		ClientID:    "test-client",
		CallbackURL: "https://example.com/auth/twitter/callback",
	})
	handler := NewAuthHandler(connector, nil, "")

	req := httptest.NewRequest(http.MethodGet, "https://example.com/auth/twitter/callback?error=access_denied", nil)
	// Add user ID to context
	ctx := middleware.ContextWithUserID(req.Context(), "user-123")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()

	handler.TwitterCallback(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", recorder.Code)
	}
}

func TestTwitterCallbackMissingCodeOrState(t *testing.T) {
	connector := twitter.New(twitter.TwitterConfig{
		ClientID:    "test-client",
		CallbackURL: "https://example.com/auth/twitter/callback",
	})
	handler := NewAuthHandler(connector, nil, "")

	// Missing code
	req := httptest.NewRequest(http.MethodGet, "https://example.com/auth/twitter/callback?state=xyz", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-123"))
	recorder := httptest.NewRecorder()
	handler.TwitterCallback(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 when missing code, got %d", recorder.Code)
	}

	// Missing state
	req = httptest.NewRequest(http.MethodGet, "https://example.com/auth/twitter/callback?code=abc", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-123"))
	recorder = httptest.NewRecorder()
	handler.TwitterCallback(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 when missing state, got %d", recorder.Code)
	}
}

func TestTwitterCallbackStateMismatch(t *testing.T) {
	connector := twitter.New(twitter.TwitterConfig{
		ClientID:    "test-client",
		CallbackURL: "https://example.com/auth/twitter/callback",
	})
	handler := NewAuthHandler(connector, nil, "")

	req := httptest.NewRequest(http.MethodGet, "https://example.com/auth/twitter/callback?code=abc&state=query_state", nil)
	req = req.WithContext(middleware.ContextWithUserID(req.Context(), "user-123"))
	req.AddCookie(&http.Cookie{
		Name:  "oauth_state",
		Value: "different_cookie_state",
	})
	recorder := httptest.NewRecorder()

	handler.TwitterCallback(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 on state mismatch, got %d", recorder.Code)
	}
}
