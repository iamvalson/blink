package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

type Connector struct {
	oauthConfig *oauth2.Config
	accessToken string
	baseURL     string
	httpClient  *http.Client
}

var _ connectors.OAuthConnector = (*Connector)(nil)

// New creates a new Youtube connector
func New(cfg YouTubeConfig) *Connector {
	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
		httpClient:  http.DefaultClient,
		baseURL:     "https://www.googleapis.com/youtube/v3",
	}
}

// PlatformID returns the stable slug used as route segment and storage key
func (c *Connector) PlatformID() string {
	return connectors.PlatformYoutube
}

// AuthCodeURL returns the URL the browser should be  redirected to
func (c *Connector) AuthCodeURL(state, verifier string) string {
	return GetAuthURL(c.oauthConfig, state)
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string) {
	c.accessToken = token
}

// Authenticate exchanges the OAuth code for tokens and identifies the user's YouTube channel
func (c *Connector) Authenticate(
	ctx context.Context,
	params connectors.AuthParams,
) (connectors.AuthResult, error) {
	token, err := ExchangeCodeForToken(
		ctx, c.oauthConfig, params.Code,
	)
	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf(
			"oauth exchange failed: %w",
			err,
		)
	}

	channel, err := GetChannelInfo(ctx, token)
	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf(
			"failed to get YouTube channel: %w",
			err,
		)
	}

	c.accessToken = token.AccessToken

	return connectors.AuthResult{
		PlatformUserID: channel.ID,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		Expiry:         token.Expiry,
	}, nil
}

// UploadMedia uploads media to the platform and returns a platform-specific
// media ID. For platforms with asynchronous processing, the returned ID
// can be used to poll processing status.
func (c *Connector) UploadMedia(
	ctx context.Context,
	token string,
	attemptID string,
	media io.Reader,
	mediaType string,
) (string, error) {
	if token == "" {
		return "", fmt.Errorf("no access token set")
	}

	if mediaType != "video/mp4" {
		return "", fmt.Errorf(
			"unsupported YouTube media type: %s",
			mediaType,
		)
	}

	baseURL := strings.Replace(c.baseURL, "/youtube/v3", "", 1)
	initURL := fmt.Sprintf("%s/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status", baseURL)

	// Tag the video with the attempt ID for crash reconciliation
	attemptTag := fmt.Sprintf("blink_attempt_%s", attemptID)
	dummyMeta := map[string]interface{}{
		"snippet": map[string]interface{}{
			"title": "Uploading...",
			"tags":  []string{attemptTag},
		},
	}
	metaBody, _ := json.Marshal(dummyMeta)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, bytes.NewReader(metaBody))
	if err != nil {
		return "", fmt.Errorf("failed to create upload init request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Upload-Content-Type", mediaType)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to init resumable upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", connectors.HTTPError(resp.StatusCode, fmt.Sprintf("upload init failed: %s", string(body)))
	}

	uploadURL := resp.Header.Get("Location")
	if uploadURL == "" {
		return "", fmt.Errorf("no Location header in upload init response")
	}

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, media)
	if err != nil {
		return "", fmt.Errorf("failed to create chunk upload request: %w", err)
	}

	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadReq.Header.Set("Content-Type", mediaType)

	uploadResp, err := c.httpClient.Do(uploadReq)
	if err != nil {
		return "", fmt.Errorf("failed to upload chunk: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK && uploadResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(uploadResp.Body)
		return "", connectors.HTTPError(uploadResp.StatusCode, fmt.Sprintf("chunk upload failed: %s", string(body)))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	if result.ID == "" {
		return "", fmt.Errorf("no video ID returned from upload")
	}

	return result.ID, nil
}

// Publish publishes a YouTube video
func (c *Connector) Publish(
	ctx context.Context,
	token string,
	attemptID string,
	caption string,
	mediaIDs ...string,
) (string, string, error) {
	if token == "" {
		return "", "", fmt.Errorf("no access token set")
	}

	if len(mediaIDs) == 0 {
		return "", "", fmt.Errorf(
			"YouTube publishing requires a video",
		)
	}

	videoID := mediaIDs[0]

	parts := strings.SplitN(caption, "\n", 2)
	title := parts[0]
	if len(title) > 100 {
		title = title[:97] + "..."
	}

	description := ""
	if len(parts) > 1 {
		description = strings.TrimSpace(parts[1])
	}

	attemptTag := fmt.Sprintf("blink_attempt_%s", attemptID)

	payload := map[string]interface{}{
		"id": videoID,
		"snippet": map[string]interface{}{
			"categoryId":  "22", // Default to People & Blogs
			"title":       title,
			"description": description,
			"tags":        []string{attemptTag}, // Maintain the tag
		},
		"status": map[string]interface{}{
			"privacyStatus": "public",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	updateURL := fmt.Sprintf("%s/videos?part=snippet,status", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, updateURL, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("failed to create update request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to update video metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if rateLimitErr := youtubeRateLimitError(resp.StatusCode, resp.Header, respBody); rateLimitErr != nil {
			return "", "", rateLimitErr
		}
		return "", "", connectors.HTTPError(resp.StatusCode, fmt.Sprintf("update metadata failed: %s", string(respBody)))
	}

	publicURL := fmt.Sprintf(
		"https://www.youtube.com/watch?v=%s",
		videoID,
	)

	return publicURL, videoID, nil
}

// GetStatus polls the platform for post/media status.
// Used for platforms with asynchronous publishing or processing.
func youtubeRateLimitError(statusCode int, headers http.Header, body []byte) *connectors.RateLimitError {
	if statusCode == http.StatusTooManyRequests {
		retryAfter := connectors.ParseRetryAfterHeader(headers.Get("Retry-After"))
		resetAt := connectors.ParseRateLimitResetHeader(headers.Get("X-RateLimit-Reset"))
		if retryAfter == 0 && resetAt != nil {
			retryAfter = time.Until(*resetAt)
			if retryAfter < 0 {
				retryAfter = 0
			}
		}
		err := connectors.NewRateLimitError(connectors.PlatformYoutube, statusCode, retryAfter, resetAt, "YouTube API rate limit exceeded")
		if rateLimitErr, ok := err.(*connectors.RateLimitError); ok {
			return rateLimitErr
		}
		return nil
	}
	if statusCode != http.StatusForbidden {
		return nil
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if payload.Error.Message == "" && len(payload.Error.Errors) == 0 {
		return nil
	}
	for _, errInfo := range payload.Error.Errors {
		reason := strings.ToLower(errInfo.Reason)
		if reason == "quotaexceeded" || reason == "ratelimitexceeded" || reason == "userratelimitexceeded" || reason == "dailylimitexceeded" || strings.Contains(reason, "quota") || strings.Contains(reason, "rate") {
			retryAfter := connectors.ParseRetryAfterHeader(headers.Get("Retry-After"))
			resetAt := connectors.ParseRateLimitResetHeader(headers.Get("X-RateLimit-Reset"))
			if retryAfter == 0 && resetAt != nil {
				retryAfter = time.Until(*resetAt)
				if retryAfter < 0 {
					retryAfter = 0
				}
			}
			err := connectors.NewRateLimitError(connectors.PlatformYoutube, statusCode, retryAfter, resetAt, payload.Error.Message)
			if rateLimitErr, ok := err.(*connectors.RateLimitError); ok {
				return rateLimitErr
			}
		}
	}
	message := strings.ToLower(payload.Error.Message)
	if strings.Contains(message, "quota") || strings.Contains(message, "rate limit") || strings.Contains(message, "daily limit") {
		retryAfter := connectors.ParseRetryAfterHeader(headers.Get("Retry-After"))
		resetAt := connectors.ParseRateLimitResetHeader(headers.Get("X-RateLimit-Reset"))
		if retryAfter == 0 && resetAt != nil {
			retryAfter = time.Until(*resetAt)
			if retryAfter < 0 {
				retryAfter = 0
			}
		}
		err := connectors.NewRateLimitError(connectors.PlatformYoutube, statusCode, retryAfter, resetAt, payload.Error.Message)
		if rateLimitErr, ok := err.(*connectors.RateLimitError); ok {
			return rateLimitErr
		}
	}
	return nil
}

func (c *Connector) GetStatus(
	ctx context.Context,
	platformPostID string,
) (string, string, error) {
	return c.getStatus(ctx, c.accessToken, platformPostID)
}

func (c *Connector) getStatus(
	ctx context.Context,
	token string,
	platformPostID string,
) (string, string, error) {

	if platformPostID == "" {
		return "", "", fmt.Errorf(
			"missing YouTube video ID",
		)
	}

	statusURL := fmt.Sprintf("%s/videos?part=processingDetails,status&id=%s", c.baseURL, platformPostID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create status request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("fetch status failed: %d %s", resp.StatusCode, string(body))
	}

	var statusResult struct {
		Items []struct {
			Status struct {
				UploadStatus string `json:"uploadStatus"`
			} `json:"status"`
			ProcessingDetails struct {
				ProcessingStatus string `json:"processingStatus"`
			} `json:"processingDetails"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statusResult); err != nil {
		return "", "", fmt.Errorf("failed to decode status response: %w", err)
	}

	if len(statusResult.Items) == 0 {
		return "", "", fmt.Errorf("video not found")
	}

	item := statusResult.Items[0]
	internalStatus := "processing"

	if item.Status.UploadStatus == "processed" && item.ProcessingDetails.ProcessingStatus == "succeeded" {
		internalStatus = "published"
	} else if item.Status.UploadStatus == "failed" || item.Status.UploadStatus == "rejected" || item.ProcessingDetails.ProcessingStatus == "failed" {
		internalStatus = "failed"
	}

	publicURL := fmt.Sprintf(
		"https://www.youtube.com/watch?v=%s",
		platformPostID,
	)

	return internalStatus, publicURL, nil
}

// GetOAuthConfig returns the OAuth config.
func (c *Connector) GetOAuthConfig() *oauth2.Config {
	return c.oauthConfig
}

// ReconcilePublish attempts to locate a previously published or uploaded video for an ambiguous attempt.
// For YouTube, it searches the user's channel for the specific attempt ID tag.
func (c *Connector) ReconcilePublish(
	ctx context.Context,
	token string,
	platformUserID string,
	attemptID string,
	caption string,
) (result connectors.ReconciliationResult, err error) {
	if token == "" {
		return connectors.ReconciliationResult{}, fmt.Errorf("no access token set")
	}

	attemptTag := fmt.Sprintf("blink_attempt_%s", attemptID)

	// Query the YouTube search API for the tag among the user's own videos
	searchURL := fmt.Sprintf("%s/search?part=snippet&forMine=true&q=%s&type=video&maxResults=5", c.baseURL, url.QueryEscape(attemptTag))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return connectors.ReconciliationResult{}, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return connectors.ReconciliationResult{}, fmt.Errorf("failed to execute search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return connectors.ReconciliationResult{}, fmt.Errorf("search failed: %d %s", resp.StatusCode, string(body))
	}

	var searchResult struct {
		Items []struct {
			Id struct {
				VideoId string `json:"videoId"`
			} `json:"id"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return connectors.ReconciliationResult{}, fmt.Errorf("failed to decode search response: %w", err)
	}

	if len(searchResult.Items) > 0 {
		videoID := searchResult.Items[0].Id.VideoId
		status, publicURL, statusErr := c.getStatus(ctx, token, videoID)
		if statusErr != nil {
			return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
		}
		if status != "published" {
			return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown, PlatformPostID: videoID, PublicURL: publicURL}, nil
		}
		return connectors.ReconciliationResult{Outcome: connectors.ReconciliationFound, PublicURL: publicURL, PlatformPostID: videoID}, nil
	}

	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}
