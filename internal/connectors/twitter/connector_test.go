package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestNew_WithCustomClient(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	conn := New(TwitterConfig{}, customClient)
	if conn.httpClient != customClient {
		t.Errorf("expected custom HTTP client to be set")
	}
}

func TestConnector_Authenticate_Success(t *testing.T) {
	var calledToken, calledUser bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" && r.Method == http.MethodPost {
			calledToken = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"access_token": "mock-twitter-token",
				"refresh_token": "mock-twitter-refresh",
				"token_type": "bearer",
				"expires_in": 3600
			}`))
			return
		}
		if r.URL.Path == "/2/users/me" && r.Method == http.MethodGet {
			calledUser = true
			if r.Header.Get("Authorization") != "Bearer mock-twitter-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"tw-user-777","name":"Auth User","username":"authuser"}}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	}))
	defer server.Close()

	origURL := userInfoURL
	userInfoURL = server.URL + "/2/users/me"
	defer func() { userInfoURL = origURL }()

	conn := New(TwitterConfig{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		CallbackURL:  "http://localhost:8080/callback",
	})
	conn.oauthConfig.Endpoint.TokenURL = server.URL + "/oauth2/token"

	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{
		Code:         "code-val",
		CodeVerifier: "verifier-val",
	})
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if !calledToken {
		t.Error("expected token endpoint to be called")
	}
	if !calledUser {
		t.Error("expected user info endpoint to be called")
	}
	if res.PlatformUserID != "tw-user-777" {
		t.Errorf("PlatformUserID = %s, want tw-user-777", res.PlatformUserID)
	}
	if res.AccessToken != "mock-twitter-token" {
		t.Errorf("AccessToken = %s, want mock-twitter-token", res.AccessToken)
	}
	if res.RefreshToken != "mock-twitter-refresh" {
		t.Errorf("RefreshToken = %s, want mock-twitter-refresh", res.RefreshToken)
	}
	if conn.accessToken != "mock-twitter-token" {
		t.Errorf("conn.accessToken = %s, want mock-twitter-token", conn.accessToken)
	}
}

func TestConnector_Authenticate_InjectedClient(t *testing.T) {
	var tokenCalls, userCalls int
	customClient := &http.Client{
		Transport: roundTripFuncTwitter(func(r *http.Request) (*http.Response, error) {
			if strings.Contains(r.URL.Path, "token") {
				tokenCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{
						"access_token": "injected-token",
						"refresh_token": "injected-refresh",
						"token_type": "bearer",
						"expires_in": 3600
					}`)),
				}, nil
			}
			if strings.Contains(r.URL.Path, "users/me") {
				userCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"injected-user-id","username":"inj"}}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		}),
	}

	conn := New(TwitterConfig{ClientID: "cid"}, customClient)
	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{
		Code:         "code-1",
		CodeVerifier: "verifier-1",
	})
	if err != nil {
		t.Fatalf("Authenticate failed with injected client: %v", err)
	}
	if tokenCalls != 1 {
		t.Errorf("expected 1 token call via injected client, got %d", tokenCalls)
	}
	if userCalls != 1 {
		t.Errorf("expected 1 user info call via injected client, got %d", userCalls)
	}
	if res.PlatformUserID != "injected-user-id" {
		t.Errorf("PlatformUserID = %s, want injected-user-id", res.PlatformUserID)
	}
}

func TestConnector_AuthCodeURL(t *testing.T) {
	cfg := TwitterConfig{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		CallbackURL:  "http://localhost:8080/auth/twitter/callback",
	}

	conn := New(cfg)
	url, err := conn.AuthCodeURL("random-state", "random-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(url, "random-state") {
		t.Errorf("expected AuthCodeURL to contain state, got: %s", url)
	}
	if !strings.Contains(url, "test_id") {
		t.Errorf("expected AuthCodeURL to contain client id, got: %s", url)
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

func TestUploadMediaValidation(t *testing.T) {
	conn := New(TwitterConfig{})

	// 1. Missing token
	_, err := conn.UploadMedia(context.Background(), "", "attempt", strings.NewReader("data"), "image/png")
	if err == nil {
		t.Fatal("expected error with empty token")
	}

	// 2. Nil media
	id, err := conn.UploadMedia(context.Background(), "token", "attempt", nil, "image/png")
	if err == nil || id != "" || connectors.ClassifyError(err) != connectors.ErrorPermanent {
		t.Fatalf("nil media upload returned id=%q err=%v", id, err)
	}

	// 3. Unsupported media type
	_, err = conn.UploadMedia(context.Background(), "token", "attempt", strings.NewReader("data"), "application/pdf")
	if err == nil || connectors.ClassifyError(err) != connectors.ErrorPermanent {
		t.Fatalf("expected permanent error for unsupported media type, got %v", err)
	}

	// 4. Empty media
	_, err = conn.UploadMedia(context.Background(), "token", "attempt", strings.NewReader(""), "image/png")
	if err == nil || connectors.ClassifyError(err) != connectors.ErrorPermanent {
		t.Fatalf("expected permanent error for empty media, got %v", err)
	}
}

func TestUploadMedia_SimpleImageSuccess(t *testing.T) {
	var mediaCategoryReceived string
	var mediaReceived []byte
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		err := r.ParseMultipartForm(10 * 1024 * 1024)
		if err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mediaCategoryReceived = r.FormValue("media_category")
		file, _, err := r.FormFile("media")
		if err == nil {
			defer file.Close()
			mediaReceived, _ = io.ReadAll(file)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"1880028106020515840","media_key":"3_1880028106020515840"}}`))
	}))
	defer server.Close()

	conn := &Connector{
		mediaUploadURL: server.URL,
		httpClient:     server.Client(),
	}

	imageContent := []byte("fake image binary content")
	mediaID, err := conn.UploadMedia(context.Background(), "test_token_123", "attempt-1", bytes.NewReader(imageContent), "image/jpeg")
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	if mediaID != "1880028106020515840" {
		t.Errorf("expected mediaID 1880028106020515840, got %s", mediaID)
	}
	if authHeader != "Bearer test_token_123" {
		t.Errorf("expected auth header Bearer test_token_123, got %s", authHeader)
	}
	if mediaCategoryReceived != "tweet_image" {
		t.Errorf("expected media_category tweet_image, got %s", mediaCategoryReceived)
	}
	if !bytes.Equal(mediaReceived, imageContent) {
		t.Errorf("received media content did not match uploaded content")
	}
}

func TestUploadMedia_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"rate limit exceeded"}`))
	}))
	defer server.Close()

	conn := &Connector{
		mediaUploadURL: server.URL,
		httpClient:     server.Client(),
	}

	_, err := conn.UploadMedia(context.Background(), "test_token", "attempt-1", strings.NewReader("sample"), "image/png")
	if !errors.Is(err, connectors.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	var rateLimitErr *connectors.RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatal("expected typed RateLimitError")
	}
	if rateLimitErr.RetryAfter != 45*time.Second {
		t.Fatalf("expected retry after 45s, got %s", rateLimitErr.RetryAfter)
	}
}

func TestUploadMedia_ChunkedVideoSuccessWithStatusPolling(t *testing.T) {
	var initCalled, appendCalled, finalizeCalled, statusCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		command := r.URL.Query().Get("command")
		if command == "" && r.Method == http.MethodPost {
			// Might be APPEND with multipart
			_ = r.ParseMultipartForm(10 * 1024 * 1024)
			command = r.FormValue("command")
		}

		w.Header().Set("Content-Type", "application/json")

		switch command {
		case "INIT":
			initCalled++
			if r.URL.Query().Get("media_category") != "tweet_video" {
				t.Errorf("expected tweet_video, got %s", r.URL.Query().Get("media_category"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"id":"video_12345"}}`))

		case "APPEND":
			appendCalled++
			if r.FormValue("media_id") != "video_12345" {
				t.Errorf("expected media_id video_12345, got %s", r.FormValue("media_id"))
			}
			w.WriteHeader(http.StatusNoContent)

		case "FINALIZE":
			finalizeCalled++
			if r.URL.Query().Get("media_id") != "video_12345" {
				t.Errorf("expected media_id video_12345, got %s", r.URL.Query().Get("media_id"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"id":"video_12345","processing_info":{"state":"pending","check_after_secs":1}}}`))

		case "STATUS":
			statusCalls++
			if r.URL.Query().Get("media_id") != "video_12345" {
				t.Errorf("expected media_id video_12345, got %s", r.URL.Query().Get("media_id"))
			}
			w.WriteHeader(http.StatusOK)
			if statusCalls == 1 {
				_, _ = w.Write([]byte(`{"data":{"id":"video_12345","processing_info":{"state":"in_progress","check_after_secs":1}}}`))
			} else {
				_, _ = w.Write([]byte(`{"data":{"id":"video_12345","processing_info":{"state":"succeeded"}}}`))
			}

		default:
			t.Errorf("unexpected command: %s (query: %s, method: %s)", command, r.URL.RawQuery, r.Method)
			http.Error(w, "unexpected command", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	conn := &Connector{
		mediaUploadURL: server.URL,
		httpClient:     server.Client(),
	}

	videoData := []byte("fake mp4 video binary data")
	mediaID, err := conn.UploadMedia(context.Background(), "test_token", "attempt-1", bytes.NewReader(videoData), "video/mp4")
	if err != nil {
		t.Fatalf("UploadMedia chunked failed: %v", err)
	}

	if mediaID != "video_12345" {
		t.Errorf("expected media ID video_12345, got %s", mediaID)
	}
	if initCalled != 1 {
		t.Errorf("expected 1 INIT call, got %d", initCalled)
	}
	if appendCalled < 1 {
		t.Errorf("expected at least 1 APPEND call, got %d", appendCalled)
	}
	if finalizeCalled != 1 {
		t.Errorf("expected 1 FINALIZE call, got %d", finalizeCalled)
	}
	if statusCalls != 2 {
		t.Errorf("expected 2 STATUS calls, got %d", statusCalls)
	}
}

func TestUploadMedia_ProcessingFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		command := r.URL.Query().Get("command")
		if command == "" && r.Method == http.MethodPost {
			_ = r.ParseMultipartForm(10 * 1024 * 1024)
			command = r.FormValue("command")
		}

		w.Header().Set("Content-Type", "application/json")
		switch command {
		case "INIT":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"id":"video_fail"}}`))
		case "APPEND":
			w.WriteHeader(http.StatusNoContent)
		case "FINALIZE":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"id":"video_fail","processing_info":{"state":"failed","error":{"code":1,"name":"InvalidVideo","message":"Unsupported video codec"}}}}`))
		default:
			http.Error(w, "unknown", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	conn := &Connector{
		mediaUploadURL: server.URL,
		httpClient:     server.Client(),
	}

	_, err := conn.UploadMedia(context.Background(), "test_token", "attempt-1", strings.NewReader("video"), "video/mp4")
	if err == nil {
		t.Fatal("expected error on processing failure")
	}
	if connectors.ClassifyError(err) != connectors.ErrorPermanent {
		t.Fatalf("expected ErrorPermanent, got %v", connectors.ClassifyError(err))
	}
	if !strings.Contains(err.Error(), "Unsupported video codec") {
		t.Fatalf("expected error message to contain reason, got %v", err)
	}
}

func TestUploadMedia_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		command := r.URL.Query().Get("command")
		if command == "" && r.Method == http.MethodPost {
			_ = r.ParseMultipartForm(10 * 1024 * 1024)
			command = r.FormValue("command")
		}
		w.Header().Set("Content-Type", "application/json")
		switch command {
		case "INIT":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"id":"video_cancel"}}`))
		case "APPEND":
			w.WriteHeader(http.StatusNoContent)
		case "FINALIZE":
			w.WriteHeader(http.StatusOK)
			// Return 10 seconds check_after_secs so test can cancel
			_, _ = w.Write([]byte(`{"data":{"id":"video_cancel","processing_info":{"state":"pending","check_after_secs":10}}}`))
		default:
			http.Error(w, "unknown", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	conn := &Connector{
		mediaUploadURL: server.URL,
		httpClient:     server.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := conn.UploadMedia(ctx, "test_token", "attempt-1", strings.NewReader("video data"), "video/mp4")
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation/deadline exceeded error, got %v", err)
	}
}

func TestPublish_WithMediaIDsPayload(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"99887766","text":"Post with photo"}}`))
	}))
	defer server.Close()

	conn := &Connector{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	url, postID, err := conn.Publish(
		context.Background(),
		"valid_token",
		"attempt-1",
		"Post with photo",
		"1880028106020515840",
		"1880028106020515841",
	)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if postID != "99887766" {
		t.Errorf("expected postID 99887766, got %s", postID)
	}
	if url != "https://x.com/i/web/status/99887766" {
		t.Errorf("expected URL https://x.com/i/web/status/99887766, got %s", url)
	}

	if requestBody["text"] != "Post with photo" {
		t.Errorf("expected text 'Post with photo', got %v", requestBody["text"])
	}

	mediaObj, ok := requestBody["media"].(map[string]any)
	if !ok {
		t.Fatalf("expected media object in payload, got %v", requestBody["media"])
	}

	mediaIDs, ok := mediaObj["media_ids"].([]any)
	if !ok || len(mediaIDs) != 2 {
		t.Fatalf("expected 2 media_ids in payload, got %v", mediaObj["media_ids"])
	}
	if mediaIDs[0] != "1880028106020515840" || mediaIDs[1] != "1880028106020515841" {
		t.Errorf("unexpected media_ids: %v", mediaIDs)
	}
}

func TestPublish_WithoutMediaIDsPayload(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"11223344","text":"Text only post"}}`))
	}))
	defer server.Close()

	conn := &Connector{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	_, _, err := conn.Publish(context.Background(), "valid_token", "attempt-1", "Text only post")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if requestBody["text"] != "Text only post" {
		t.Errorf("expected text 'Text only post', got %v", requestBody["text"])
	}
	if _, exists := requestBody["media"]; exists {
		t.Errorf("expected no 'media' field in payload, got %v", requestBody["media"])
	}
}

func TestPublish_RejectsMoreThan4MediaIDs(t *testing.T) {
	conn := New(TwitterConfig{})
	_, _, err := conn.Publish(
		context.Background(),
		"valid_token",
		"attempt-1",
		"Caption",
		"id1", "id2", "id3", "id4", "id5",
	)
	if err == nil {
		t.Fatal("expected error when more than 4 media IDs are provided")
	}
	if connectors.ClassifyError(err) != connectors.ErrorPermanent {
		t.Fatalf("expected ErrorPermanent, got %v", connectors.ClassifyError(err))
	}
	if !strings.Contains(err.Error(), "at most 4 media items") {
		t.Fatalf("expected error message to mention at most 4 media items, got: %v", err)
	}
}
