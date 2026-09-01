package twitter

import (
	"context"
	"testing"
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

	_, _, err := conn.Publish(context.Background(), "test tweet")
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

	if url != "https://twitter.com/i/web/status/12345" {
		t.Fatalf("Expected a valid URL, got %s", url)
	}
}