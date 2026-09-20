package twitter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamvalson/blink/internal/connectors"
)


func TestNew(t *testing.T) {
	cfg := TwitterConfig{
		ClientID: "test_id",
		ClientSecret: "test_secret",
		CallbackURL: "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)
	if conn == nil {
		t.Fatal("failed to create Twitter connection")
	}
}


func TestSetAccessToken(t *testing.T) {
	cfg := TwitterConfig{
		ClientID: "test_id",
		ClientSecret: "test_secret",
		CallbackURL: "http://localhost:8080/auth/twitter/callback",
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
		ClientID: "test_id",
		ClientSecret: "test_secret",
		CallbackURL: "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)

	_, _, err := conn.Publish(context.Background(), "", "test tweet")
	if err == nil {
		t.Fatal("Publish should fail without access token")
	}
}


func TestGetStatus(t *testing.T) {
	cfg := TwitterConfig{
		ClientID: "test_id",
		ClientSecret: "test_secret",
		CallbackURL: "http://localhost:8080/auth/twitter/callback",
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

	url, postID, err := conn.Publish(context.Background(), "test_valid_token", "Hello World")
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
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	conn := &Connector{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	_, _, err := conn.Publish(context.Background(), "test_valid_token", "Hello World")
	if err != connectors.ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got %v", err)
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

	_, _, err := conn.Publish(context.Background(), "test_valid_token", "Hello World")
	if err == nil || !strings.Contains(err.Error(), "x api error: 402") {
		t.Fatalf("expected 402 error, got %v", err)
	}
}