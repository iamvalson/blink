package twitter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func TestNew(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		CallbackURL:  "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)
	if conn == nil {
		t.Fatal("failed to create Twitter connection")
	}
}

func TestSetAccessToken(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		CallbackURL:  "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)
	token := "test_token_123"
	conn.SetAccessToken(token)

	if conn.accessToken != token {
		t.Fatalf("Expected token %s, got %s", token, conn.accessToken)
	}
}

func TestPublishWithoutToken(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		CallbackURL:  "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)

	_, _, err := conn.Publish(context.Background(), "", "attempt-123", "test tweet")
	if err == nil {
		t.Fatal("Publish should fail without access token")
	}
}

func TestGetStatus(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		CallbackURL:  "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)
	status, url, err := conn.GetStatus(context.Background(), "12345")

	if err != nil {
		t.Fatalf("GetStatus failed %v", err)
	}

	if status != "published" {
		t.Fatalf("Expected status 'published', got %s", status)
	}

	if url != "https://x.com/i/web/status/12345" {
		t.Fatalf("Expected a valid URL, got %s", url)
	}
}

func TestReconcilePublishReturnsUnknown(t *testing.T) {
	conn := New(TwitterConfig{})

	result, err := conn.ReconcilePublish(context.Background(), "token", "user", "attempt-123", "tweet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != connectors.ReconciliationUnknown {
		t.Fatalf("expected UNKNOWN reconciliation outcome, got %q", result.Outcome)
	}
}

func TestPublishSuccessWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_valid_token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"192837465","text":"Hello World"}}`))
	}))
	defer server.Close()

	conn := &Connector{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	url, postID, err := conn.Publish(context.Background(), "test_valid_token", "attempt-123", "Hello World")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if postID != "192837465" {
		t.Errorf("expected postID 192837465, got %s", postID)
	}

	expectedURL := "https://x.com/i/web/status/192837465"
	if url != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, url)
	}
}

func TestPublishRateLimitedWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	conn := &Connector{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	_, _, err := conn.Publish(context.Background(), "test_valid_token", "attempt-123", "Hello World")
	if !errors.Is(err, connectors.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	var rateLimitErr *connectors.RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatal("expected typed RateLimitError")
	}
	if rateLimitErr.RetryAfter != 120*time.Second {
		t.Fatalf("retry after = %s, want 120s", rateLimitErr.RetryAfter)
	}
}

func TestPublishRateLimitedThenSuccessWithMockServer(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"192837465","text":"Hello World"}}`))
	}))
	defer server.Close()

	conn := &Connector{baseURL: server.URL, httpClient: server.Client()}
	_, _, err := conn.Publish(context.Background(), "test_valid_token", "attempt-123", "Hello World")
	if !errors.Is(err, connectors.ErrRateLimited) {
		t.Fatalf("first request should return rate limit error, got %v", err)
	}
	_, _, err = conn.Publish(context.Background(), "test_valid_token", "attempt-456", "Hello World")
	if err != nil {
		t.Fatalf("second request should succeed after rate limit: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestPublishAPIErrorWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"detail":"credits depleted"}`))
	}))
	defer server.Close()

	conn := &Connector{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	_, _, err := conn.Publish(context.Background(), "test_valid_token", "attempt-123", "Hello World")
	if err == nil || !strings.Contains(err.Error(), "x api error: 402") {
		t.Fatalf("expected 402 error, got %v", err)
	}
}
