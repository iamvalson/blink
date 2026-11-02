package twitter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestIntegration_TwitterLifecycle performs a real-account smoke test against the live
// X (Twitter) API v2 when TWITTER_TEST_TOKEN or X_TEST_TOKEN is set.
// It exercises: UploadMedia -> Publish with media ID -> GetStatus.
func TestIntegration_TwitterLifecycle(t *testing.T) {
	token := os.Getenv("TWITTER_TEST_TOKEN")
	if token == "" {
		token = os.Getenv("X_TEST_TOKEN")
	}
	if token == "" {
		t.Skip("Skipping live smoke test: TWITTER_TEST_TOKEN or X_TEST_TOKEN is not set")
	}

	cfg := TwitterConfig{
		ClientID:     os.Getenv("TWITTER_CLIENT_ID"),
		ClientSecret: os.Getenv("TWITTER_CLIENT_SECRET"),
		CallbackURL:  os.Getenv("TWITTER_CALLBACK_URL"),
	}

	conn := New(cfg)
	conn.SetAccessToken(token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1x1 transparent PNG image binary
	samplePNG := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}

	t.Log("1. Uploading media to X...")
	mediaID, err := conn.UploadMedia(ctx, token, "smoke-test", bytes.NewReader(samplePNG), "image/png")
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}
	t.Logf("Uploaded X media ID: %s", mediaID)

	t.Log("2. Publishing tweet with media to X...")
	caption := fmt.Sprintf("[Blink Test] Smoke test %d", time.Now().Unix())
	publicURL, platformPostID, err := conn.Publish(ctx, token, "smoke-test", caption, mediaID)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	t.Logf("Tweet published at: %s (ID: %s)", publicURL, platformPostID)

	t.Log("3. Checking tweet status...")
	status, statusURL, err := conn.GetStatus(ctx, platformPostID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	t.Logf("Status: %s, URL: %s", status, statusURL)
}
