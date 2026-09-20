package twitter

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func TestMockNew(t *testing.T) {
	conn := NewMock()
	if conn == nil {
		t.Fatal("expected non-nil MockConnector")
	}

	var _ connectors.PlatformConnector = conn
}

func TestMockPublish(t *testing.T) {
	conn := NewMock()

	// 1. Should fail when token is empty
	_, _, err := conn.Publish(context.Background(), "", "Test tweet")
	if err == nil {
		t.Fatal("expected error when token is empty, got nil")
	}

	// 2. Should succeed with valid token
	url, postID, err := conn.Publish(context.Background(), "mock_access_token", "Hello Twitter from Mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(postID, "mock_x_") {
		t.Errorf("expected platformPostID to have prefix 'mock_x_', got %s", postID)
	}

	expectedURL := "http://mock.x.local/status/" + postID
	if url != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, url)
	}
}

func TestMockUploadMedia(t *testing.T) {
	conn := NewMock()
	media := bytes.NewReader([]byte("fake image data"))

	mediaID, err := conn.UploadMedia(context.Background(), media, "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(mediaID, "mock_media_") {
		t.Errorf("expected mediaID to have prefix 'mock_media_', got %s", mediaID)
	}
}

func TestMockGetStatus(t *testing.T) {
	conn := NewMock()
	postID := "mock_x_test123"

	status, url, err := conn.GetStatus(context.Background(), postID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status != "published" {
		t.Errorf("expected status 'published', got %s", status)
	}

	expectedURL := "http://mock.x.local/status/" + postID
	if url != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, url)
	}
}

func TestMockAuthenticate(t *testing.T) {
	conn := NewMock()

	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{
		Code:         "test_code",
		CodeVerifier: "test_verifier",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(res.PlatformUserID, "mock_user_") {
		t.Errorf("expected PlatformUserID to have prefix 'mock_user_', got %s", res.PlatformUserID)
	}

	if !strings.HasPrefix(res.AccessToken, "mock_token_") {
		t.Errorf("expected AccessToken to have prefix 'mock_token_', got %s", res.AccessToken)
	}

	if res.Expiry.Before(time.Now()) {
		t.Error("expected future token expiry")
	}
}

