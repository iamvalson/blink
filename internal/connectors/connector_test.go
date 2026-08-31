package connectors

import (
	"bytes"
	"context"
	"testing"
)

func TestMockConnectorAuthenticate(t *testing.T) {
	mock := NewMockConnector()

	userID, err := mock.Authenticate(context.Background(), "auth_code_123")
	if err != nil{
		t.Fatalf("Authenticate failed: %v", err)
	}

	if userID != "mock_user_123" {
		t.Fatalf("Expected mock_user_123 got %s", userID)
	}
}


func TestMockConnectorPublish(t *testing.T) {
	mock := NewMockConnector()

	url, postID, err := mock.Publish(context.Background(), "Hello World", "media_123")

	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if url != "https://example.com/post/123" {
		t.Fatalf("Expected valid URL, got %s", url)
	}

	if postID != "mock_post_123" {
		t.Fatalf("Expected mock_post_123 got %s", postID)
	}
}


func TestMockConnectorUploadMedia(t *testing.T) {
	mock := NewMockConnector()

	media := bytes.NewReader([]byte("fake image data"))

	mediaID, err := mock.UploadMedia(context.Background(), media, "image/png")

	if err != nil{
		t.Fatalf("UploadMedia failed: %v", err)
	}

	if mediaID != "mock_media_123"{
		t.Fatalf("Expected mock_media_123 got %s", mediaID)
	}
}


func TestMockConnectorCustomBehaviour(t *testing.T) {
	mock := NewMockConnector()


	// Override behaviour for this test
	mock.PublishFn = func(ctx context.Context, caption string, mediaIDs ...string) (string, string, error) {
		if caption == "error" {
			return "", "", ErrPublishFailed
		}
		return "https://example.com/custom", "custom_123", nil
	}

	// Should work
	_, postID, err := mock.Publish(context.Background(), "hello", "media_1")
	if err != nil {
		t.Fatalf("Publish should succeed")
	}
	if postID != "custom_123" {
		t.Fatalf("Expected custom_123, got %s", postID)
	}

	// Should fail
	_, _, err = mock.Publish(context.Background(), "error", "media_1")
	if err == nil {
		t.Fatalf("Publish should fail with 'error' caption")
	}
}
