package tiktok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

type Connector struct {
	oauthConfig *oauth2.Config
	accessToken string
	httpClient  *http.Client
}

var _ connectors.OAuthConnector = (*Connector)(nil)
var _ connectors.OAuthTokenRefresher = (*Connector)(nil)

// New creates a new TikTok connector
func New(cfg TikTokConfig, client ...*http.Client) *Connector {
	hc := &http.Client{
		Timeout: 30 * time.Second,
	}

	if len(client) > 0 && client[0] != nil {
		hc = client[0]
	}

	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
		httpClient:  hc,
	}
}

// PlatformID returns the stable slug used as route segment and storage key
func (c *Connector) PlatformID() string {
	return connectors.PlatformTikTok
}

// AuthCodeURL returns the URL the browser should be redirected to
func (c *Connector) AuthCodeURL(state, verifier string) (string, error) {
	return GetAuthURL(c.oauthConfig, state)
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string) {
	c.accessToken = token
}

// Authenticate exchanges the OAuth code for tokens and returns the TikTok credentials
func (c *Connector) Authenticate(ctx context.Context, params connectors.AuthParams) (connectors.AuthResult, error) {
	token, err := ExchangeCodeForToken(ctx, c.oauthConfig, params.Code, c.httpClient)

	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf(
			"oauth exchange failed: %w",
			err,
		)
	}

	openID, ok := token.Extra("open_id").(string)
	if !ok || openID == "" {
		return connectors.AuthResult{}, fmt.Errorf(
			"TikTok token response missing open_id",
		)
	}

	c.accessToken = token.AccessToken

	return connectors.AuthResult{
		PlatformUserID: openID,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		Expiry:         token.Expiry,
	}, nil
}

const (
	tiktokCreatorInfoEndpoint = "https://open.tiktokapis.com/v2/post/publish/creator_info/query/"
	maxErrorResponseBytes     = 64 * 1024
	maxSuccessResponseBytes   = 1024 * 1024
)

// QueryCreatorInfo gets the target creator's latest information.
func (c *Connector) QueryCreatorInfo(ctx context.Context, token string) (*TikTokQueryCreatorDataResponse, error) {
	if strings.TrimSpace(token) == "" {
		return nil, connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"AUTH_REQUIRED",
			errors.Join(connectors.ErrAuthFailed, fmt.Errorf("no access token set")),
		)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tiktokCreatorInfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create query request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get creator info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		if readErr != nil {
			return nil, fmt.Errorf("failed to read error response: %w", readErr)
		}

		if len(bodyBytes) > 0 {
			var errResp struct {
				Error   *TikTokAPIError `json:"error"`
				Code    string          `json:"code"`
				Message string          `json:"message"`
				LogID   string          `json:"log_id"`
			}
			if jsonErr := json.Unmarshal(bodyBytes, &errResp); jsonErr == nil {
				var errCode, errMsg, logID string
				if errResp.Error != nil {
					errCode = errResp.Error.Code
					errMsg = errResp.Error.Message
					logID = errResp.Error.LogID
				}
				if errCode == "" {
					errCode = errResp.Code
				}
				if errMsg == "" {
					errMsg = errResp.Message
				}
				if logID == "" {
					logID = errResp.LogID
				}

				if errCode != "" || errMsg != "" {
					errMsg = sanitizeErrorString(errMsg, token)
					return nil, classifyTikTokAPIError(errCode, errMsg, logID, resp.StatusCode, resp.Header)
				}
			}
		}

		bodyText := sanitizeErrorString(string(bodyBytes), token)
		return nil, c.classifyHTTPStatusError(resp.StatusCode, resp.Header, bodyText)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxSuccessResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("empty response body from TikTok creator info API")
	}
	if len(bodyBytes) > maxSuccessResponseBytes {
		return nil, fmt.Errorf("tiktok response body exceeded maximum allowed size of %d bytes", maxSuccessResponseBytes)
	}

	var result TikTokCreatorInfoAPIResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf(
			"failed to decode TikTok creator info response: %w",
			err,
		)
	}

	errCode := result.Error.Code
	errMsg := result.Error.Message
	logID := result.Error.LogID
	if errCode == "" && result.Code != "" {
		errCode = result.Code
		errMsg = result.Message
		logID = result.LogID
	}

	if errCode != "" && errCode != "ok" {
		errMsg = sanitizeErrorString(errMsg, token)
		return nil, classifyTikTokAPIError(errCode, errMsg, logID, resp.StatusCode, resp.Header)
	}

	if strings.TrimSpace(result.Data.Username) == "" {
		return nil, connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"INVALID_RESPONSE",
			fmt.Errorf("invalid TikTok creator info response: missing creator username"),
		)
	}

	if result.Data.MaxVideoPostDuration < 0 {
		return nil, connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"INVALID_RESPONSE",
			fmt.Errorf("invalid TikTok creator info response: negative max video duration"),
		)
	}

	if result.Data.PrivacyLevel == nil {
		result.Data.PrivacyLevel = []string{}
	}

	return &result.Data, nil
}

func (c *Connector) classifyHTTPStatusError(statusCode int, headers http.Header, bodyText string) error {
	trimmedBody := strings.TrimSpace(bodyText)
	if statusCode == http.StatusTooManyRequests {
		policy := connectors.PlatformRateLimitPolicy{Platform: connectors.PlatformTikTok}
		rateLimitErr := policy.Classify(statusCode, headers)
		if rateLimitErr != nil {
			if trimmedBody != "" {
				rateLimitErr.Message = fmt.Sprintf("%s: %s", rateLimitErr.Message, trimmedBody)
			}
			return rateLimitErr
		}
		msg := "get creator info failed: rate limit exceeded"
		if trimmedBody != "" {
			msg = fmt.Sprintf("get creator info failed: %s", trimmedBody)
		}
		return connectors.NewRateLimitError(connectors.PlatformTikTok, statusCode, 0, nil, msg)
	}

	msg := fmt.Sprintf("get creator info failed with status %d", statusCode)
	if trimmedBody != "" {
		msg = fmt.Sprintf("get creator info failed: %s", trimmedBody)
	}
	return connectors.HTTPError(statusCode, msg)
}

func classifyTikTokAPIError(code, message, logID string, statusCode int, headers http.Header) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = code
	}
	if logID != "" {
		msg = fmt.Sprintf("%s (log_id: %s)", msg, logID)
	}

	// 1. Rate limiting
	if statusCode == http.StatusTooManyRequests || code == "rate_limit_exceeded" || strings.HasPrefix(code, "spam_risk_too_many") {
		policy := connectors.PlatformRateLimitPolicy{Platform: connectors.PlatformTikTok}
		rateLimitErr := policy.Classify(statusCode, headers)
		if rateLimitErr != nil {
			if msg != "" {
				rateLimitErr.Message = fmt.Sprintf("TikTok rate limit exceeded: %s", msg)
			}
			return rateLimitErr
		}
		return connectors.NewRateLimitError(
			connectors.PlatformTikTok,
			statusCode,
			0,
			nil,
			fmt.Sprintf("TikTok rate limit exceeded: %s", msg),
		)
	}

	// 2. Authentication failures & token expiry
	if statusCode == http.StatusUnauthorized ||
		code == "access_token_invalid" ||
		code == "invalid_token" ||
		code == "token_expired" ||
		code == "user_not_found" ||
		code == "user_status_abnormal" ||
		code == "client_not_authorized" {
		errCode := code
		if errCode == "" {
			errCode = "AUTH_FAILED"
		}
		return &connectors.ClassifiedError{
			Class:      connectors.ErrorPermanent,
			Code:       errCode,
			StatusCode: statusCode,
			Err:        errors.Join(connectors.ErrInvalidToken, connectors.ErrAuthFailed, fmt.Errorf("TikTok creator info: %s", msg)),
		}
	}

	// 3. Authorization / permissions / scopes
	if statusCode == http.StatusForbidden ||
		code == "scope_not_authorized" ||
		code == "scope_permission_missed" ||
		code == "permission_denied" ||
		code == "forbidden" ||
		code == "unaudited_client_can_only_post_to_self_only" ||
		strings.Contains(code, "banned_from_posting") {
		errCode := code
		if errCode == "" {
			errCode = "FORBIDDEN"
		}
		return &connectors.ClassifiedError{
			Class:      connectors.ErrorPermanent,
			Code:       errCode,
			StatusCode: statusCode,
			Err:        errors.Join(connectors.ErrAuthFailed, fmt.Errorf("TikTok creator info: %s", msg)),
		}
	}

	// 4. Temporary server errors
	if statusCode >= 500 ||
		code == "internal_error" ||
		code == "server_error" ||
		code == "service_unavailable" ||
		code == "gateway_timeout" {
		errCode := code
		if errCode == "" {
			errCode = "HTTP_ERROR"
		}
		return &connectors.ClassifiedError{
			Class:      connectors.ErrorRetryable,
			Code:       errCode,
			StatusCode: statusCode,
			Err:        fmt.Errorf("TikTok creator info: %s", msg),
		}
	}

	// 5. Validation errors
	if statusCode == http.StatusBadRequest ||
		code == "invalid_param" ||
		code == "invalid_params" ||
		code == "missing_param" {
		errCode := code
		if errCode == "" {
			errCode = "INVALID_PARAMS"
		}
		return &connectors.ClassifiedError{
			Class:      connectors.ErrorPermanent,
			Code:       errCode,
			StatusCode: statusCode,
			Err:        errors.Join(connectors.ErrValidationFailed, fmt.Errorf("TikTok creator info: %s", msg)),
		}
	}

	// 6. Fallback
	class := connectors.ErrorPermanent
	if statusCode >= 500 {
		class = connectors.ErrorRetryable
	}
	errCode := code
	if errCode == "" {
		errCode = "HTTP_ERROR"
	}
	return &connectors.ClassifiedError{
		Class:      class,
		Code:       errCode,
		StatusCode: statusCode,
		Err:        fmt.Errorf("TikTok creator info: %s", msg),
	}
}

func sanitizeErrorString(msg, token string) string {
	if token != "" {
		msg = strings.ReplaceAll(msg, "Bearer "+token, "[REDACTED]")
		msg = strings.ReplaceAll(msg, token, "[REDACTED]")
	}
	return msg
}

// UploadMedia uploads binary media (not yet implemented for TikTok)
func (c *Connector) UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error) {
	return "", errors.New("not implemented")
}

// Publish posts content to TikTok (not yet implemented)
func (c *Connector) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error) {
	return "", "", errors.New("not implemented")
}

// GetStatus polls TikTok for post status (not yet implemented)
func (c *Connector) GetStatus(ctx context.Context, platformPostID string) (string, string, error) {
	return "", "", errors.New("not implemented")
}

// ReconcilePublish resolves an ambiguous publication (not yet implemented)
func (c *Connector) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, errors.New("not implemented")
}
