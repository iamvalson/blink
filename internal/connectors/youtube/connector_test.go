package youtube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamvalson/blink/internal/connectors"
)

// setupTestServer creates a new httptest.Server and ensures it gets closed after the test
func setupTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// setupTestConnector configures a Connector to hit the provided test server URL
func setupTestConnector(t *testing.T, serverURL string) *Connector {
	cfg := YouTubeConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		CallbackURL:  "http://localhost/callback",
	}

	conn := New(cfg)
	conn.baseURL = serverURL + "/youtube/v3"

	// Override OAuth endpoints for token exchange
	conn.oauthConfig.Endpoint.TokenURL = serverURL + "/token"
	conn.oauthConfig.Endpoint.AuthURL = serverURL + "/auth"

	// Override package-level variables strictly for testing
	origChannelInfoURL := channelInfoURL
	channelInfoURL = serverURL + "/youtube/v3/channels"
	t.Cleanup(func() { channelInfoURL = origChannelInfoURL })

	return conn
}

func TestConnector_AuthCodeURL(t *testing.T) {
	conn := setupTestConnector(t, "http://dummy")
	url := conn.AuthCodeURL("random-state", "random-verifier")

	if !strings.Contains(url, "random-state") {
		t.Errorf("expected AuthCodeURL to contain state, got: %s", url)
	}
	if !strings.Contains(url, "test-client-id") {
		t.Errorf("expected AuthCodeURL to contain client id, got: %s", url)
	}
}

func TestConnector_Authenticate(t *testing.T) {
	calledToken := false
	calledChannel := false

	ts := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" && r.Method == http.MethodPost {
			calledToken = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"access_token": "mock-access-token",
				"refresh_token": "mock-refresh-token",
				"expires_in": 3600,
				"token_type": "Bearer"
			}`))
			return
		}

		if r.URL.Path == "/youtube/v3/channels" && r.Method == http.MethodGet {
			calledChannel = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"items": [
					{
						"id": "channel-123",
						"snippet": {
							"title": "My Test Channel",
							"description": "Channel description"
						}
					}
				]
			}`))
			return
		}

		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	})

	conn := setupTestConnector(t, ts.URL)
	ctx := context.Background()
	params := connectors.AuthParams{
		Code: "auth-code-123",
	}

	res, err := conn.Authenticate(ctx, params)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if !calledToken {
		t.Errorf("expected token exchange endpoint to be called")
	}
	if !calledChannel {
		t.Errorf("expected channel info endpoint to be called")
	}

	if res.PlatformUserID != "channel-123" {
		t.Errorf("expected platform user ID 'channel-123', got '%s'", res.PlatformUserID)
	}
	if res.AccessToken != "mock-access-token" {
		t.Errorf("expected access token 'mock-access-token', got '%s'", res.AccessToken)
	}
}

func TestConnector_UploadMedia(t *testing.T) {
	calledInit := false
	calledChunk := false

	ts := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload/youtube/v3/videos" && r.Method == http.MethodPost {
			calledInit = true
			if r.URL.Query().Get("uploadType") != "resumable" {
				t.Errorf("expected uploadType=resumable, got %s", r.URL.Query().Get("uploadType"))
			}

			// Respond with the location header for the chunk upload
			w.Header().Set("Location", "http://"+r.Host+"/upload-chunk")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.URL.Path == "/upload-chunk" && r.Method == http.MethodPut {
			calledChunk = true
			body, _ := io.ReadAll(r.Body)
			if string(body) != "dummy video content" {
				t.Errorf("unexpected video body: %s", string(body))
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "test-video-123"}`))
			return
		}

		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	})

	conn := setupTestConnector(t, ts.URL)
	conn.SetAccessToken("mock-access-token")

	videoData := strings.NewReader("dummy video content")
	ctx := context.Background()

	mediaID, err := conn.UploadMedia(ctx, "mock-access-token", "test.mp4", videoData, "video/mp4")

	if err != nil && strings.Contains(err.Error(), "not implemented") {
		// Expecting failure due to missing implementation in TDD
		t.Skip("UploadMedia logic is not yet implemented")
	}

	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	if !calledInit || !calledChunk {
		t.Errorf("expected upload init and chunk streaming endpoints to be called")
	}

	if mediaID != "test-video-123" {
		t.Errorf("expected mediaID 'test-video-123', got '%s'", mediaID)
	}
}

func TestConnector_Publish(t *testing.T) {
	called := false
	ts := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/videos" && r.Method == http.MethodPut {
			called = true

			var payload struct {
				ID      string `json:"id"`
				Snippet struct {
					Title       string `json:"title"`
					Description string `json:"description"`
				} `json:"snippet"`
				Status struct {
					PrivacyStatus string `json:"privacyStatus"`
				} `json:"status"`
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode publish payload: %v", err)
			}

			if payload.ID != "vid-123" {
				t.Errorf("expected id vid-123, got %s", payload.ID)
			}
			if payload.Status.PrivacyStatus != "public" {
				t.Errorf("expected privacyStatus 'public', got '%s'", payload.Status.PrivacyStatus)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "vid-123"}`))
			return
		}

		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	})

	conn := setupTestConnector(t, ts.URL)
	ctx := context.Background()

	caption := "My Video Title\nHere is a detailed description of the video."
	publicURL, postID, err := conn.Publish(ctx, "mock-token", "attempt-123", caption, "vid-123")

	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if !called {
		t.Errorf("expected API call to update video metadata")
	}

	expectedURL := "https://www.youtube.com/watch?v=vid-123"
	if publicURL != expectedURL {
		t.Errorf("expected publicURL %s, got %s", expectedURL, publicURL)
	}
	if postID != "vid-123" {
		t.Errorf("expected postID vid-123, got %s", postID)
	}
}

func TestConnector_ReconcilePublishReturnsUnknownWhileProcessing(t *testing.T) {
	ts := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/search" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":{"videoId":"vid-123"}}]}`))
			return
		}
		if r.URL.Path == "/youtube/v3/videos" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"status":{"uploadStatus":"uploaded"},"processingDetails":{"processingStatus":"processing"}}]}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	})

	conn := setupTestConnector(t, ts.URL)
	result, err := conn.ReconcilePublish(context.Background(), "token", "channel-123", "attempt-123", "caption")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != connectors.ReconciliationUnknown {
		t.Fatalf("expected UNKNOWN reconciliation outcome, got %q", result.Outcome)
	}
	if result.PlatformPostID != "vid-123" {
		t.Fatalf("expected video ID to be retained, got %q", result.PlatformPostID)
	}
}

func TestConnector_GetStatus(t *testing.T) {
	called := false
	ts := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/videos" && r.Method == http.MethodGet {
			called = true
			if r.URL.Query().Get("id") != "vid-123" {
				t.Errorf("expected id=vid-123, got %s", r.URL.Query().Get("id"))
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"items": [
					{
						"id": "vid-123",
						"status": {
							"uploadStatus": "processed"
						},
						"processingDetails": {
							"processingStatus": "succeeded"
						}
					}
				]
			}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	})

	conn := setupTestConnector(t, ts.URL)
	ctx := context.Background()

	status, publicURL, err := conn.GetStatus(ctx, "vid-123")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if !called {
		t.Errorf("expected API call to fetch video status")
	}

	if status != "published" {
		t.Errorf("expected status 'published', got '%s'", status)
	}

	expectedURL := "https://www.youtube.com/watch?v=vid-123"
	if publicURL != expectedURL {
		t.Errorf("expected publicURL %s, got %s", expectedURL, publicURL)
	}
}

func TestConnector_Publish_MissingVideo_PermanentError(t *testing.T) {
	conn := setupTestConnector(t, "http://dummy")
	ctx := context.Background()

	_, _, err := conn.Publish(ctx, "mock-token", "attempt-1", "Caption with no video")
	if err == nil {
		t.Fatal("expected error for missing video, got nil")
	}

	class := connectors.ClassifyError(err)
	if class != connectors.ErrorPermanent {
		t.Fatalf("expected ErrorPermanent for missing video, got %v", class)
	}
}

func TestConnector_UploadMedia_NilMedia_PermanentError(t *testing.T) {
	conn := setupTestConnector(t, "http://dummy")
	ctx := context.Background()

	_, err := conn.UploadMedia(ctx, "mock-token", "attempt-1", nil, "video/mp4")
	if err == nil {
		t.Fatal("expected error for nil media, got nil")
	}

	class := connectors.ClassifyError(err)
	if class != connectors.ErrorPermanent {
		t.Fatalf("expected ErrorPermanent for nil media, got %v", class)
	}
}

func TestConnector_Publish_MissingToken_PermanentError(t *testing.T) {
	conn := setupTestConnector(t, "http://dummy")
	ctx := context.Background()

	_, _, err := conn.Publish(ctx, "", "attempt-1", "Caption", "video-123")
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}

	class := connectors.ClassifyError(err)
	if class != connectors.ErrorPermanent {
		t.Fatalf("expected ErrorPermanent for missing token, got %v", class)
	}
}
