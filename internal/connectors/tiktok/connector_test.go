package tiktok

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func TestNew(t *testing.T) {
	cfg := TikTokConfig{
		ClientKey:    "test-key",
		ClientSecret: "test-secret",
		RedirectURI:  "https://example.com/callback",
		Scopes:       []string{"user.info.basic"},
	}
	conn := New(cfg)
	if conn == nil {
		t.Fatal("failed to create TikTok connector")
	}
}

func TestNew_WithCustomClient(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	conn := New(TikTokConfig{}, customClient)
	if conn.httpClient != customClient {
		t.Errorf("expected custom HTTP client to be set")
	}
}

func TestConnector_PlatformID(t *testing.T) {
	conn := New(TikTokConfig{})
	if id := conn.PlatformID(); id != connectors.PlatformTikTok {
		t.Errorf("expected platform ID %s, got %s", connectors.PlatformTikTok, id)
	}
}

func TestConnector_AuthCodeURL(t *testing.T) {
	cfg := TikTokConfig{
		ClientKey:    "test-client-key",
		ClientSecret: "test-client-secret",
		RedirectURI:  "https://example.com/callback",
		Scopes:       []string{"user.info.basic", "video.list"},
	}
	conn := New(cfg)

	url, err := conn.AuthCodeURL("random-state", "random-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(url, "random-state") {
		t.Errorf("expected AuthCodeURL to contain state, got: %s", url)
	}
	if !strings.Contains(url, "test-client-key") {
		t.Errorf("expected AuthCodeURL to contain client_key, got: %s", url)
	}
}

func TestConnector_AuthCodeURL_Error(t *testing.T) {
	cfg := TikTokConfig{
		ClientKey: "test-key",
	}
	conn := New(cfg)
	conn.oauthConfig.Endpoint.AuthURL = "://invalid"

	_, err := conn.AuthCodeURL("state", "verifier")
	if err == nil {
		t.Fatal("expected error for invalid endpoint URL")
	}
}

func TestConnector_SetAccessToken(t *testing.T) {
	conn := New(TikTokConfig{})
	conn.SetAccessToken("my-access-token")
	if conn.accessToken != "my-access-token" {
		t.Errorf("expected access token 'my-access-token', got %s", conn.accessToken)
	}
}

func TestConnector_Authenticate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type": "Bearer",
			"expires_in": 3600,
			"open_id": "tiktok-user-123",
			"scope": "user.info.basic"
		}`))
	}))
	defer server.Close()

	conn := New(TikTokConfig{
		ClientKey:    "key",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost/callback",
	})
	conn.oauthConfig.Endpoint.TokenURL = server.URL

	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "auth-code"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.PlatformUserID != "tiktok-user-123" {
		t.Errorf("expected PlatformUserID tiktok-user-123, got %s", res.PlatformUserID)
	}
	if res.AccessToken != "test-access-token" {
		t.Errorf("expected AccessToken test-access-token, got %s", res.AccessToken)
	}
	if res.RefreshToken != "test-refresh-token" {
		t.Errorf("expected RefreshToken test-refresh-token, got %s", res.RefreshToken)
	}
	if res.Expiry.Before(time.Now()) {
		t.Error("expected future token expiry")
	}
	if conn.accessToken != "test-access-token" {
		t.Errorf("expected connector accessToken to be updated, got %s", conn.accessToken)
	}
}

func TestConnector_Authenticate_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()

	conn := New(TikTokConfig{
		ClientKey:    "key",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost/callback",
	})
	conn.oauthConfig.Endpoint.TokenURL = server.URL

	_, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "bad-code"})
	if err == nil {
		t.Fatal("expected error on failed exchange")
	}
}

func TestConnector_Authenticate_MissingOpenID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "token-without-openid",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	conn := New(TikTokConfig{})
	conn.oauthConfig.Endpoint.TokenURL = server.URL

	_, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "valid-code"})
	if err == nil || !strings.Contains(err.Error(), "missing open_id") {
		t.Fatalf("expected missing open_id error, got: %v", err)
	}
}

func TestConnector_Authenticate_InjectedClient(t *testing.T) {
	var clientCalled bool
	customClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			clientCalled = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"access_token": "custom-client-token",
					"expires_in": 3600,
					"open_id": "custom-client-user"
				}`)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, customClient)
	res, err := conn.Authenticate(context.Background(), connectors.AuthParams{Code: "code"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clientCalled {
		t.Fatal("expected injected HTTP client to be called during Authenticate")
	}
	if res.PlatformUserID != "custom-client-user" {
		t.Errorf("expected custom-client-user, got %s", res.PlatformUserID)
	}
}

func TestConnector_UnimplementedMethods(t *testing.T) {
	conn := New(TikTokConfig{})
	ctx := context.Background()

	_, err := conn.UploadMedia(ctx, "token", "attempt", bytes.NewReader([]byte("data")), "video/mp4")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for UploadMedia, got %v", err)
	}

	_, _, err = conn.Publish(ctx, "token", "attempt", "caption")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for Publish, got %v", err)
	}

	_, _, err = conn.GetStatus(ctx, "post-id")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for GetStatus, got %v", err)
	}

	res, err := conn.ReconcilePublish(ctx, "token", "user", "attempt", "caption")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected not implemented error for ReconcilePublish, got %v", err)
	}
	if res.Outcome != connectors.ReconciliationUnknown {
		t.Errorf("expected ReconciliationUnknown, got %s", res.Outcome)
	}
}

func TestQueryCreatorInfo_Success(t *testing.T) {
	mockJSON := `{
		"data": {
			"creator_avatar_url": "https://example.com/avatar.jpg",
			"creator_username": "tiktokcreator",
			"creator_nickname": "TikTok Creator",
			"privacy_level_options": [
				"PUBLIC_TO_EVERYONE",
				"MUTUAL_FOLLOW_FRIENDS",
				"SELF_ONLY"
			],
			"comment_disabled": false,
			"duet_disabled": false,
			"stitch_disabled": false,
			"max_video_post_duration_sec": 600
		},
		"error": {
			"code": "ok",
			"message": "",
			"log_id": "test-log-123"
		}
	}`

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
				Body:       io.NopCloser(strings.NewReader(mockJSON)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, client)
	data, err := conn.QueryCreatorInfo(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.AvatarURL != "https://example.com/avatar.jpg" {
		t.Errorf("expected avatar URL https://example.com/avatar.jpg, got %s", data.AvatarURL)
	}
	if data.Username != "tiktokcreator" {
		t.Errorf("expected username tiktokcreator, got %s", data.Username)
	}
	if data.Nickname != "TikTok Creator" {
		t.Errorf("expected nickname TikTok Creator, got %s", data.Nickname)
	}
	if len(data.PrivacyLevel) != 3 || data.PrivacyLevel[0] != "PUBLIC_TO_EVERYONE" {
		t.Errorf("expected 3 privacy levels, got %v", data.PrivacyLevel)
	}
	if data.CommentDisabled || data.DuetDisabled || data.StitchDisabled {
		t.Errorf("expected interaction flags to be false")
	}
	if data.MaxVideoPostDuration != 600 {
		t.Errorf("expected max video duration 600, got %d", data.MaxVideoPostDuration)
	}
}

func TestQueryCreatorInfo_DisabledInteractions(t *testing.T) {
	mockJSON := `{
		"data": {
			"creator_avatar_url": "https://example.com/avatar.jpg",
			"creator_username": "restricted_user",
			"creator_nickname": "Restricted User",
			"privacy_level_options": ["SELF_ONLY"],
			"comment_disabled": true,
			"duet_disabled": true,
			"stitch_disabled": true,
			"max_video_post_duration_sec": 300
		},
		"error": {
			"code": "ok"
		}
	}`

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(mockJSON)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, client)
	data, err := conn.QueryCreatorInfo(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !data.CommentDisabled {
		t.Errorf("expected comment_disabled to be true")
	}
	if !data.DuetDisabled {
		t.Errorf("expected duet_disabled to be true")
	}
	if !data.StitchDisabled {
		t.Errorf("expected stitch_disabled to be true")
	}
}

func TestQueryCreatorInfo_EmptyPrivacyOptions(t *testing.T) {
	mockJSON := `{
		"data": {
			"creator_avatar_url": "https://example.com/avatar.jpg",
			"creator_username": "banned_user",
			"creator_nickname": "Banned User",
			"privacy_level_options": [],
			"comment_disabled": true,
			"duet_disabled": true,
			"stitch_disabled": true,
			"max_video_post_duration_sec": 0
		},
		"error": {
			"code": "ok"
		}
	}`

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(mockJSON)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, client)
	data, err := conn.QueryCreatorInfo(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("unexpected error on empty privacy options: %v", err)
	}

	if data.PrivacyLevel == nil {
		t.Errorf("expected non-nil PrivacyLevel slice")
	}
	if len(data.PrivacyLevel) != 0 {
		t.Errorf("expected 0 privacy levels, got %d", len(data.PrivacyLevel))
	}
}

func TestQueryCreatorInfo_EmptyToken(t *testing.T) {
	conn := New(TikTokConfig{})
	testCases := []struct {
		name  string
		token string
	}{
		{name: "empty string", token: ""},
		{name: "whitespace only", token: "   \t\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := conn.QueryCreatorInfo(context.Background(), tc.token)
			if err == nil {
				t.Fatalf("expected error for empty token, got data: %v", data)
			}

			if !errors.Is(err, connectors.ErrAuthFailed) {
				t.Errorf("expected err to wrap ErrAuthFailed, got %v", err)
			}

			if class := connectors.ClassifyError(err); class != connectors.ErrorPermanent {
				t.Errorf("expected ErrorPermanent, got %v", class)
			}

			var classified *connectors.ClassifiedError
			if !errors.As(err, &classified) {
				t.Fatalf("expected ClassifiedError, got %T", err)
			}
			if classified.Code != "AUTH_REQUIRED" {
				t.Errorf("expected Code AUTH_REQUIRED, got %s", classified.Code)
			}
		})
	}
}

func TestQueryCreatorInfo_Request(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			capturedReq = r
			if r.Body != nil {
				capturedBody, _ = io.ReadAll(r.Body)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"data": {
						"creator_username": "tiktokcreator",
						"privacy_level_options": ["PUBLIC_TO_EVERYONE"]
					},
					"error": {"code": "ok"}
				}`)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, client)
	_, err := conn.QueryCreatorInfo(context.Background(), "my-test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.Method != http.MethodPost {
		t.Errorf("expected POST method, got %s", capturedReq.Method)
	}

	expectedURL := "https://open.tiktokapis.com/v2/post/publish/creator_info/query/"
	if capturedReq.URL.String() != expectedURL {
		t.Errorf("expected URL %s, got %s", expectedURL, capturedReq.URL.String())
	}

	authHeader := capturedReq.Header.Get("Authorization")
	if authHeader != "Bearer my-test-token" {
		t.Errorf("expected Authorization Bearer my-test-token, got %s", authHeader)
	}

	contentType := capturedReq.Header.Get("Content-Type")
	if contentType != "application/json; charset=UTF-8" {
		t.Errorf("expected Content-Type application/json; charset=UTF-8, got %s", contentType)
	}

	if len(capturedBody) != 0 {
		t.Errorf("expected empty body, got %d bytes: %s", len(capturedBody), string(capturedBody))
	}
}

func TestQueryCreatorInfo_HTTPErrors(t *testing.T) {
	tests := []struct {
		name              string
		statusCode        int
		headers           http.Header
		responseBody      string
		expectedClass     connectors.ErrorClass
		expectedSentinel  error
		expectedCode      string
		expectRateLimit   bool
		expectedRetrySecs time.Duration
	}{
		{
			name:             "HTTP 401 with JSON structured error",
			statusCode:       http.StatusUnauthorized,
			responseBody:     `{"error": {"code": "access_token_invalid", "message": "Token has expired", "log_id": "log-401"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrAuthFailed,
			expectedCode:     "access_token_invalid",
		},
		{
			name:             "HTTP 401 with plain text",
			statusCode:       http.StatusUnauthorized,
			responseBody:     "Unauthorized",
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrAuthFailed,
			expectedCode:     "AUTH_FAILED",
		},
		{
			name:             "HTTP 403 with JSON structured error",
			statusCode:       http.StatusForbidden,
			responseBody:     `{"error": {"code": "scope_not_authorized", "message": "video.publish scope required", "log_id": "log-403"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrAuthFailed,
			expectedCode:     "scope_not_authorized",
		},
		{
			name:             "HTTP 403 with plain text",
			statusCode:       http.StatusForbidden,
			responseBody:     "Forbidden",
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrAuthFailed,
			expectedCode:     "AUTH_FAILED",
		},
		{
			name:              "HTTP 429 with Retry-After header and JSON",
			statusCode:        http.StatusTooManyRequests,
			headers:           http.Header{"Retry-After": []string{"45"}},
			responseBody:      `{"error": {"code": "rate_limit_exceeded", "message": "Rate limit exceeded", "log_id": "log-429"}}`,
			expectedClass:     connectors.ErrorRetryable,
			expectedSentinel:  connectors.ErrRateLimited,
			expectRateLimit:   true,
			expectedRetrySecs: 45 * time.Second,
		},
		{
			name:             "HTTP 429 with plain text",
			statusCode:       http.StatusTooManyRequests,
			responseBody:     "Too Many Requests",
			expectedClass:    connectors.ErrorRetryable,
			expectedSentinel: connectors.ErrRateLimited,
			expectRateLimit:  true,
		},
		{
			name:          "HTTP 500 with JSON structured error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"error": {"code": "internal_error", "message": "Internal TikTok error", "log_id": "log-500"}}`,
			expectedClass: connectors.ErrorRetryable,
			expectedCode:  "internal_error",
		},
		{
			name:          "HTTP 500 with plain text",
			statusCode:    http.StatusInternalServerError,
			responseBody:  "Internal Server Error",
			expectedClass: connectors.ErrorRetryable,
			expectedCode:  "HTTP_ERROR",
		},
		{
			name:          "HTTP 503 Service Unavailable",
			statusCode:    http.StatusServiceUnavailable,
			responseBody:  "Service Unavailable",
			expectedClass: connectors.ErrorRetryable,
			expectedCode:  "HTTP_ERROR",
		},
		{
			name:             "HTTP 400 with JSON parameter error",
			statusCode:       http.StatusBadRequest,
			responseBody:     `{"error": {"code": "invalid_param", "message": "Invalid parameter provided"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrValidationFailed,
			expectedCode:     "invalid_param",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					headers := tc.headers
					if headers == nil {
						headers = make(http.Header)
					}
					return &http.Response{
						StatusCode: tc.statusCode,
						Header:     headers,
						Body:       io.NopCloser(strings.NewReader(tc.responseBody)),
					}, nil
				}),
			}

			conn := New(TikTokConfig{}, client)
			_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
			if err == nil {
				t.Fatalf("expected error for HTTP %d, got nil", tc.statusCode)
			}

			if class := connectors.ClassifyError(err); class != tc.expectedClass {
				t.Errorf("expected error class %s, got %s", tc.expectedClass, class)
			}

			if tc.expectedSentinel != nil {
				if !errors.Is(err, tc.expectedSentinel) {
					t.Errorf("expected error to wrap sentinel %v, got %v", tc.expectedSentinel, err)
				}
			}

			if tc.expectRateLimit {
				var rateLimitErr *connectors.RateLimitError
				if !errors.As(err, &rateLimitErr) {
					t.Errorf("expected RateLimitError, got %T: %v", err, err)
				} else {
					if !rateLimitErr.Retryable {
						t.Errorf("expected rate limit error to be retryable")
					}
					if tc.expectedRetrySecs > 0 && rateLimitErr.RetryAfter != tc.expectedRetrySecs {
						t.Errorf("expected RetryAfter %v, got %v", tc.expectedRetrySecs, rateLimitErr.RetryAfter)
					}
				}
			}

			if tc.expectedCode != "" {
				var classified *connectors.ClassifiedError
				if errors.As(err, &classified) {
					if classified.Code != tc.expectedCode {
						t.Errorf("expected classified code %s, got %s", tc.expectedCode, classified.Code)
					}
					if classified.StatusCode != tc.statusCode {
						t.Errorf("expected status code %d, got %d", tc.statusCode, classified.StatusCode)
					}
				}
			}
		})
	}
}

func TestQueryCreatorInfo_TikTokAPIErrors(t *testing.T) {
	tests := []struct {
		name             string
		responseJSON     string
		expectedClass    connectors.ErrorClass
		expectedSentinel error
		expectedCode     string
	}{
		{
			name:             "rate_limit_exceeded in 200 OK",
			responseJSON:     `{"error": {"code": "rate_limit_exceeded", "message": "Rate limit exceeded"}}`,
			expectedClass:    connectors.ErrorRetryable,
			expectedSentinel: connectors.ErrRateLimited,
			expectedCode:     "rate_limit_exceeded",
		},
		{
			name:          "internal_error in 200 OK",
			responseJSON:  `{"error": {"code": "internal_error", "message": "Server error"}}`,
			expectedClass: connectors.ErrorRetryable,
			expectedCode:  "internal_error",
		},
		{
			name:             "access_token_invalid in 200 OK",
			responseJSON:     `{"error": {"code": "access_token_invalid", "message": "Invalid token"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrInvalidToken,
			expectedCode:     "access_token_invalid",
		},
		{
			name:             "scope_not_authorized in 200 OK",
			responseJSON:     `{"error": {"code": "scope_not_authorized", "message": "Not authorized"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrAuthFailed,
			expectedCode:     "scope_not_authorized",
		},
		{
			name:             "invalid_params in 200 OK",
			responseJSON:     `{"error": {"code": "invalid_params", "message": "Parameter error"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrValidationFailed,
			expectedCode:     "invalid_params",
		},
		{
			name:             "spam_risk_too_many_posts in 200 OK",
			responseJSON:     `{"error": {"code": "spam_risk_too_many_posts", "message": "Too many posts"}}`,
			expectedClass:    connectors.ErrorRetryable,
			expectedSentinel: connectors.ErrRateLimited,
			expectedCode:     "spam_risk_too_many_posts",
		},
		{
			name:             "spam_risk_user_banned_from_posting in 200 OK",
			responseJSON:     `{"error": {"code": "spam_risk_user_banned_from_posting", "message": "Account banned from posting"}}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrAuthFailed,
			expectedCode:     "spam_risk_user_banned_from_posting",
		},
		{
			name:             "top-level code and message format without error object",
			responseJSON:     `{"code": "access_token_invalid", "message": "Token expired"}`,
			expectedClass:    connectors.ErrorPermanent,
			expectedSentinel: connectors.ErrInvalidToken,
			expectedCode:     "access_token_invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(tc.responseJSON)),
					}, nil
				}),
			}

			conn := New(TikTokConfig{}, client)
			_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
			if err == nil {
				t.Fatalf("expected error for API error response, got nil")
			}

			if class := connectors.ClassifyError(err); class != tc.expectedClass {
				t.Errorf("expected class %s, got %s", tc.expectedClass, class)
			}

			if tc.expectedSentinel != nil {
				if !errors.Is(err, tc.expectedSentinel) {
					t.Errorf("expected err to wrap sentinel %v, got %v", tc.expectedSentinel, err)
				}
			}

			var classified *connectors.ClassifiedError
			if errors.As(err, &classified) {
				if classified.Code != tc.expectedCode {
					t.Errorf("expected Code %s, got %s", tc.expectedCode, classified.Code)
				}
			}
		})
	}
}

func TestQueryCreatorInfo_InvalidJSON(t *testing.T) {
	testCases := []struct {
		name string
		json string
	}{
		{name: "malformed JSON syntax", json: `{"data": { broken json`},
		{name: "unexpected field type for boolean", json: `{"data": {"creator_username": "user", "comment_disabled": "invalid_bool"}}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(tc.json)),
					}, nil
				}),
			}

			conn := New(TikTokConfig{}, client)
			_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
			if err == nil {
				t.Fatal("expected error on invalid JSON, got nil")
			}

			if !strings.Contains(err.Error(), "failed to decode TikTok creator info response") {
				t.Errorf("expected decode error message, got: %v", err)
			}
		})
	}
}

func TestQueryCreatorInfo_EmptyResponse(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, client)
	_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
	if err == nil {
		t.Fatal("expected error on empty response body, got nil")
	}

	if !strings.Contains(err.Error(), "empty response body") {
		t.Errorf("expected empty response body error, got: %v", err)
	}
}

func TestQueryCreatorInfo_MissingRequiredFields(t *testing.T) {
	testCases := []struct {
		name         string
		responseJSON string
		errSubstring string
	}{
		{
			name:         "empty data object",
			responseJSON: `{"data": {}, "error": {"code": "ok"}}`,
			errSubstring: "missing creator username",
		},
		{
			name:         "data with empty username string",
			responseJSON: `{"data": {"creator_username": "   ", "creator_nickname": "Test"}, "error": {"code": "ok"}}`,
			errSubstring: "missing creator username",
		},
		{
			name:         "negative max video duration",
			responseJSON: `{"data": {"creator_username": "valid_user", "max_video_post_duration_sec": -100}, "error": {"code": "ok"}}`,
			errSubstring: "negative max video duration",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(tc.responseJSON)),
					}, nil
				}),
			}

			conn := New(TikTokConfig{}, client)
			_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}

			if !strings.Contains(err.Error(), tc.errSubstring) {
				t.Errorf("expected error containing %q, got %v", tc.errSubstring, err)
			}

			if class := connectors.ClassifyError(err); class != connectors.ErrorPermanent {
				t.Errorf("expected ErrorPermanent, got %v", class)
			}
		})
	}
}

func TestQueryCreatorInfo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before or during request

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, r.Context().Err()
		}),
	}

	conn := New(TikTokConfig{}, client)
	_, err := conn.QueryCreatorInfo(ctx, "test-token")
	if err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected errors.Is(err, context.Canceled) to be true, got: %v", err)
	}
}

func TestQueryCreatorInfo_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, r.Context().Err()
		}),
	}

	conn := New(TikTokConfig{}, client)
	_, err := conn.QueryCreatorInfo(ctx, "test-token")
	if err == nil {
		t.Fatal("expected error on timed-out context, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected errors.Is(err, context.DeadlineExceeded) to be true, got: %v", err)
	}
}

func TestQueryCreatorInfo_NetworkFailure(t *testing.T) {
	netErr := errors.New("simulated network connection reset by peer")
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, netErr
		}),
	}

	conn := New(TikTokConfig{}, client)
	_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}

	if !errors.Is(err, netErr) {
		t.Errorf("expected errors.Is(err, netErr) to be true, got: %v", err)
	}
}

func TestQueryCreatorInfo_OversizedResponse(t *testing.T) {
	t.Run("oversized error response is handled in bounded memory", func(t *testing.T) {
		// Provide 2MB of error body
		oversizedErrorBody := strings.Repeat("A", 2*1024*1024)

		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader(oversizedErrorBody)),
				}, nil
			}),
		}

		conn := New(TikTokConfig{}, client)
		_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
		if err == nil {
			t.Fatal("expected error for HTTP 500, got nil")
		}

		if class := connectors.ClassifyError(err); class != connectors.ErrorRetryable {
			t.Errorf("expected ErrorRetryable, got %v", class)
		}
	})

	t.Run("oversized 200 OK response returns error and bounds memory", func(t *testing.T) {
		// Provide > 1MB of 200 OK body
		oversizedBody := strings.Repeat(" ", 1024*1024+50)

		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(oversizedBody)),
				}, nil
			}),
		}

		conn := New(TikTokConfig{}, client)
		_, err := conn.QueryCreatorInfo(context.Background(), "test-token")
		if err == nil {
			t.Fatal("expected error for oversized response, got nil")
		}

		if !strings.Contains(err.Error(), "exceeded maximum allowed size") {
			t.Errorf("expected size exceeded error, got: %v", err)
		}
	})
}

func TestQueryCreatorInfo_Security_TokenRedaction(t *testing.T) {
	secretToken := "act.secret-token-value-xyz987"
	rawErrorWithToken := fmt.Sprintf(`{"error": {"code": "access_token_invalid", "message": "Failed token Bearer %s verification", "log_id": "log-sec"}}`, secretToken)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(rawErrorWithToken)),
			}, nil
		}),
	}

	conn := New(TikTokConfig{}, client)
	_, err := conn.QueryCreatorInfo(context.Background(), secretToken)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errString := err.Error()
	if strings.Contains(errString, secretToken) {
		t.Fatalf("security violation: secret token leaked in error message: %s", errString)
	}

	if !strings.Contains(errString, "[REDACTED]") {
		t.Errorf("expected token to be replaced with [REDACTED], got: %s", errString)
	}
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (t *trackingReadCloser) Close() error {
	t.closed = true
	return nil
}

func TestQueryCreatorInfo_ResponseBodyClosed(t *testing.T) {
	scenarios := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "success 200", statusCode: http.StatusOK, body: `{"data":{"creator_username":"user"},"error":{"code":"ok"}}`},
		{name: "error 401", statusCode: http.StatusUnauthorized, body: `{"error":{"code":"access_token_invalid"}}`},
		{name: "malformed JSON", statusCode: http.StatusOK, body: `{"data": invalid json`},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			tracker := &trackingReadCloser{Reader: strings.NewReader(sc.body)}
			client := &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: sc.statusCode,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       tracker,
					}, nil
				}),
			}

			conn := New(TikTokConfig{}, client)
			_, _ = conn.QueryCreatorInfo(context.Background(), "test-token")

			if !tracker.closed {
				t.Errorf("expected response body to be closed for %s", sc.name)
			}
		})
	}
}
