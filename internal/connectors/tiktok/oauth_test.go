package tiktok

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewOAuthConfigAndGetAuthURL(t *testing.T) {
	cfg := NewOAuthConfig(TikTokConfig{ClientKey: "test-key", ClientSecret: "test-secret", RedirectURI: "https://example.com/callback", Scopes: []string{"user.info.basic", "video.list"}})
	if cfg.ClientID != "test-key" || cfg.ClientSecret != "test-secret" || cfg.RedirectURL != "https://example.com/callback" || !reflect.DeepEqual(cfg.Scopes, []string{"user.info.basic", "video.list"}) {
		t.Fatal("OAuth configuration does not match inputs")
	}
	if cfg.Endpoint.AuthURL != "https://www.tiktok.com/v2/auth/authorize/" || cfg.Endpoint.TokenURL != "https://open.tiktokapis.com/v2/oauth/token/" {
		t.Fatal("incorrect endpoints")
	}
	raw, err := GetAuthURL(cfg, "state with +&")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "www.tiktok.com" || u.Path != "/v2/auth/authorize/" {
		t.Fatalf("incorrect authorization endpoint: %s", raw)
	}
	want := url.Values{"client_key": {"test-key"}, "redirect_uri": {cfg.RedirectURL}, "scope": {"user.info.basic,video.list"}, "response_type": {"code"}, "state": {"state with +&"}}
	if !reflect.DeepEqual(u.Query(), want) {
		t.Fatalf("query = %v, want %v", u.Query(), want)
	}
	cfg.Endpoint.AuthURL = "://invalid"
	if _, err := GetAuthURL(cfg, "state"); err == nil {
		t.Fatal("expected URL parse error")
	}
}

func TestExchangeCodeForToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %s", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		want := url.Values{"client_key": {"test-key"}, "client_secret": {"secret+&"}, "code": {"code+&"}, "grant_type": {"authorization_code"}, "redirect_uri": {"https://example.com/callback"}}
		if !reflect.DeepEqual(r.PostForm, want) {
			t.Error("incorrect token request parameters")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"test-access","refresh_token":"test-refresh","token_type":"Bearer","expires_in":3600,"open_id":"test-user","scope":"user.info.basic","refresh_expires_in":86400}`)
	}))
	defer server.Close()
	cfg := NewOAuthConfig(TikTokConfig{ClientKey: "test-key", ClientSecret: "secret+&", RedirectURI: "https://example.com/callback"})
	cfg.Endpoint.TokenURL = server.URL
	before := time.Now()
	token, err := ExchangeCodeForToken(context.Background(), cfg, "code+&", server.Client())
	after := time.Now()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "test-access" || token.RefreshToken != "test-refresh" || token.TokenType != "Bearer" {
		t.Fatal("incorrect token decoding")
	}
	if token.Expiry.Before(before.Add(time.Hour)) || token.Expiry.After(after.Add(time.Hour)) {
		t.Fatalf("incorrect expiry: %v", token.Expiry)
	}
	if token.Extra("open_id") != "test-user" || token.Extra("scope") != "user.info.basic" || token.Extra("refresh_expires_in") != int64(86400) {
		t.Fatal("missing token metadata")
	}
}

func TestExchangeCodeForTokenErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body, want string
	}{
		{"empty token", 200, `{"expires_in":3600}`, "empty access token"},
		{"blank token", 200, `{"access_token":"  ","expires_in":3600}`, "empty access token"},
		{"missing lifetime", 200, `{"access_token":"test"}`, "invalid expires_in"},
		{"zero lifetime", 200, `{"access_token":"test","expires_in":0}`, "invalid expires_in"},
		{"negative lifetime", 200, `{"access_token":"test","expires_in":-1}`, "invalid expires_in"},
		{"overflow lifetime", 200, `{"access_token":"test","expires_in":9223372036854775807}`, "invalid expires_in"},
		{"nonnumeric lifetime", 200, `{"access_token":"test","expires_in":"bad"}`, "decode TikTok token response"},
		{"malformed JSON", 200, `{`, "decode TikTok token response"},
		{"structured error", 400, `{"error":"invalid_request","error_description":"The request parameters are malformed.","log_id":"omit-this-id"}`, "400: invalid_request: The request parameters are malformed."},
		{"error code only", 401, `{"error":"invalid_client"}`, "401: invalid_client"},
		{"empty error", 500, `{}`, "returned 500"},
		{"non-JSON error", 502, `Bad Gateway`, "returned 502: decode error response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer server.Close()
			cfg := NewOAuthConfig(TikTokConfig{})
			cfg.Endpoint.TokenURL = server.URL
			token, err := ExchangeCodeForToken(context.Background(), cfg, "test-code", server.Client())
			if token != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("token = %v, error = %v; want %q", token, err, tc.want)
			}
			if strings.Contains(err.Error(), "omit-this-id") {
				t.Fatal("error includes log_id")
			}
		})
	}
}

func TestExchangeCodeForTokenRequestErrors(t *testing.T) {
	cfg := NewOAuthConfig(TikTokConfig{})
	cfg.Endpoint.TokenURL = "://invalid"
	if _, err := ExchangeCodeForToken(context.Background(), cfg, "test", nil); err == nil || !strings.Contains(err.Error(), "create TikTok token request") {
		t.Fatalf("expected request creation error, got %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("canceled request reached server") }))
	cfg.Endpoint.TokenURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExchangeCodeForToken(ctx, cfg, "test", server.Client()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	server.Close()
	if _, err := ExchangeCodeForToken(context.Background(), cfg, "test", nil); err == nil || !strings.Contains(err.Error(), "send TikTok token request") {
		t.Fatalf("expected network error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestExchangeCodeForToken_HTTPClientInjection(t *testing.T) {
	var intercepted bool
	var capturedReq *http.Request

	mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		intercepted = true
		capturedReq = r

		body := io.NopCloser(strings.NewReader(`{
			"access_token": "injected-access",
			"refresh_token": "injected-refresh",
			"token_type": "Bearer",
			"expires_in": 7200,
			"open_id": "injected-user",
			"scope": "user.info.basic",
			"refresh_expires_in": 172800
		}`))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       body,
		}, nil
	})

	customClient := &http.Client{Transport: mockTransport}
	cfg := NewOAuthConfig(TikTokConfig{
		ClientKey:    "my-key",
		ClientSecret: "my-secret",
		RedirectURI:  "https://example.com/oauth/callback",
	})

	token, err := ExchangeCodeForToken(context.Background(), cfg, "my-code", customClient)
	if err != nil {
		t.Fatalf("ExchangeCodeForToken failed: %v", err)
	}

	if !intercepted {
		t.Fatal("expected request to be handled by injected client transport")
	}

	if capturedReq.Method != http.MethodPost {
		t.Errorf("expected POST method, got %s", capturedReq.Method)
	}
	if capturedReq.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("expected urlencoded Content-Type, got %s", capturedReq.Header.Get("Content-Type"))
	}
	if capturedReq.URL.String() != cfg.Endpoint.TokenURL {
		t.Errorf("expected URL %s, got %s", cfg.Endpoint.TokenURL, capturedReq.URL.String())
	}

	if token.AccessToken != "injected-access" {
		t.Errorf("expected AccessToken injected-access, got %s", token.AccessToken)
	}
	if token.Extra("open_id") != "injected-user" {
		t.Errorf("expected open_id injected-user, got %v", token.Extra("open_id"))
	}
}

func TestExchangeCodeForToken_NilClientSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "nil-client-access",
			"expires_in": 3600,
			"open_id": "nil-user"
		}`))
	}))
	defer server.Close()

	cfg := NewOAuthConfig(TikTokConfig{})
	cfg.Endpoint.TokenURL = server.URL

	token, err := ExchangeCodeForToken(context.Background(), cfg, "code", nil)
	if err != nil {
		t.Fatalf("unexpected error with nil client: %v", err)
	}
	if token.AccessToken != "nil-client-access" {
		t.Errorf("expected nil-client-access, got %s", token.AccessToken)
	}
}
