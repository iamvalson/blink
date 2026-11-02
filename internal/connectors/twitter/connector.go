package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

type Connector struct {
	oauthConfig    *oauth2.Config
	accessToken    string
	baseURL        string
	mediaUploadURL string
	httpClient     *http.Client
}

var _ connectors.OAuthConnector = (*Connector)(nil)

// New creates a new Twitter/X connector.
func New(cfg TwitterConfig, client ...*http.Client) *Connector {
	var hc *http.Client
	if len(client) > 0 && client[0] != nil {
		hc = client[0]
	}
	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
		httpClient:  hc,
	}
}

// PlatformID returns the stable slug used as route segment and storage key.
func (c *Connector) PlatformID() string { return connectors.PlatformTwitter }

// AuthCodeURL returns the URL the browser should be redirected to.
func (c *Connector) AuthCodeURL(state, verifier string) (string, error) {
	return GetAuthURL(c.oauthConfig, state, verifier), nil
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string) {
	c.accessToken = token
}

// Authenticate exchanges auth code for tokens and returns user ID
func (c *Connector) Authenticate(ctx context.Context, params connectors.AuthParams) (result connectors.AuthResult, err error) {
	authCtx := ctx
	if c.httpClient != nil {
		authCtx = context.WithValue(ctx, oauth2.HTTPClient, c.httpClient)
	}

	// Exchange code for token
	token, err := ExchangeCodeForToken(authCtx, c.oauthConfig, params.Code, params.CodeVerifier)
	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf("oauth exchange failed: %w", err)
	}

	// Get user info
	userInfo, err := GetUserInfo(ctx, token, c.httpClient)
	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf("failed to get user info: %w", err)
	}

	// Store token for later use (caller will encrypt and save to DB)
	c.accessToken = token.AccessToken

	return connectors.AuthResult{
		PlatformUserID: userInfo.ID,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		Expiry:         token.Expiry,
	}, nil
}

func (c *Connector) getMediaEndpoint() string {
	if c.mediaUploadURL != "" {
		return c.mediaUploadURL
	}
	if c.baseURL != "" {
		if strings.HasSuffix(c.baseURL, "/2/tweets") {
			return strings.TrimSuffix(c.baseURL, "/2/tweets") + "/2/media/upload"
		}
		if strings.HasSuffix(c.baseURL, "/2/media/upload") {
			return c.baseURL
		}
		return strings.TrimRight(c.baseURL, "/") + "/2/media/upload"
	}
	return "https://api.x.com/2/media/upload"
}

func (c *Connector) classifyRateLimit(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	policy := connectors.PlatformRateLimitPolicy{Platform: connectors.PlatformTwitter}
	rateLimitErr := policy.Classify(resp.StatusCode, resp.Header)
	if rateLimitErr != nil {
		if text := strings.TrimSpace(string(body)); text != "" {
			rateLimitErr.Message = fmt.Sprintf("%s: %s", rateLimitErr.Message, text)
		}
		return rateLimitErr
	}
	return connectors.NewRateLimitError(connectors.PlatformTwitter, resp.StatusCode, 0, nil, fmt.Sprintf("x api rate limit exceeded: %s", strings.TrimSpace(string(body))))
}

// UploadMedia uploads media to Twitter and returns media ID.
// Flow: Read stored file -> Upload file to X -> Wait for processing if required -> Return X media ID
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
	if media == nil {
		return "", connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"INVALID_MEDIA",
			fmt.Errorf("media reader is nil"),
		)
	}

	if !strings.HasPrefix(mediaType, "image/") && !strings.HasPrefix(mediaType, "video/") {
		return "", connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"UNSUPPORTED_MEDIA_TYPE",
			fmt.Errorf("unsupported media type for Twitter/X: %s", mediaType),
		)
	}

	var r io.ReadSeeker
	var totalBytes int64

	if rs, ok := media.(io.ReadSeeker); ok {
		end, err := rs.Seek(0, io.SeekEnd)
		if err == nil {
			_, err = rs.Seek(0, io.SeekStart)
			if err == nil {
				r = rs
				totalBytes = end
			}
		}
	}

	if r == nil {
		data, err := io.ReadAll(media)
		if err != nil {
			return "", fmt.Errorf("read media content: %w", err)
		}
		totalBytes = int64(len(data))
		r = bytes.NewReader(data)
	}

	if totalBytes <= 0 {
		return "", connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"EMPTY_MEDIA",
			fmt.Errorf("media content is empty"),
		)
	}

	mediaCategory := getMediaCategory(mediaType)

	// For videos and large files (>5MB), use chunked upload flow (INIT, APPEND, FINALIZE, STATUS).
	// For standard images <= 5MB, use simple single-shot upload.
	if strings.HasPrefix(mediaType, "video/") || totalBytes > 5*1024*1024 {
		return c.uploadChunked(ctx, token, r, totalBytes, mediaType, mediaCategory)
	}

	return c.uploadSimple(ctx, token, r, mediaCategory)
}

func (c *Connector) uploadSimple(
	ctx context.Context,
	token string,
	r io.Reader,
	mediaCategory string,
) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("media", "media")
	if err != nil {
		return "", fmt.Errorf("create multipart form file: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", fmt.Errorf("copy media data to multipart: %w", err)
	}

	if err := writer.WriteField("media_category", mediaCategory); err != nil {
		return "", fmt.Errorf("write media_category field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	endpoint := c.getMediaEndpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("execute upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", c.classifyRateLimit(resp)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return "", connectors.HTTPError(resp.StatusCode, fmt.Sprintf("x media upload error: %d %s", resp.StatusCode, string(respBody)))
	}

	var uploadResp MediaUploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}

	mediaID := uploadResp.GetMediaID()
	if mediaID == "" {
		return "", connectors.NewClassifiedError(connectors.ErrorPermanent, "INVALID_MEDIA_ID", fmt.Errorf("x media upload returned empty media id: %s", string(respBody)))
	}

	if procInfo := uploadResp.GetProcessingInfo(); procInfo != nil {
		return c.waitForProcessing(ctx, token, mediaID, procInfo)
	}

	return mediaID, nil
}

func (c *Connector) uploadChunked(
	ctx context.Context,
	token string,
	r io.ReadSeeker,
	totalBytes int64,
	mediaType string,
	mediaCategory string,
) (string, error) {
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := c.getMediaEndpoint()

	// 1. INIT
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse media endpoint: %w", err)
	}
	q := u.Query()
	q.Set("command", "INIT")
	q.Set("total_bytes", strconv.FormatInt(totalBytes, 10))
	q.Set("media_type", mediaType)
	q.Set("media_category", mediaCategory)
	u.RawQuery = q.Encode()

	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create init request: %w", err)
	}
	initReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	initResp, err := client.Do(initReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("execute init request: %w", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode == http.StatusTooManyRequests {
		return "", c.classifyRateLimit(initResp)
	}

	initBody, _ := io.ReadAll(initResp.Body)
	if initResp.StatusCode != http.StatusOK && initResp.StatusCode != http.StatusCreated && initResp.StatusCode != http.StatusAccepted {
		return "", connectors.HTTPError(initResp.StatusCode, fmt.Sprintf("x media init error: %d %s", initResp.StatusCode, string(initBody)))
	}

	var initRespObj MediaUploadResponse
	if err := json.Unmarshal(initBody, &initRespObj); err != nil {
		return "", fmt.Errorf("decode init response: %w", err)
	}

	mediaID := initRespObj.GetMediaID()
	if mediaID == "" {
		return "", connectors.NewClassifiedError(connectors.ErrorPermanent, "INVALID_MEDIA_ID", fmt.Errorf("x media init returned empty media id: %s", string(initBody)))
	}

	// 2. APPEND chunks
	const chunkSize = 4 * 1024 * 1024
	buf := make([]byte, chunkSize)
	segmentIndex := 0

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		n, readErr := io.ReadFull(r, buf)
		if n > 0 {
			chunkData := buf[:n]

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)

			if err := writer.WriteField("command", "APPEND"); err != nil {
				return "", fmt.Errorf("write append command field: %w", err)
			}
			if err := writer.WriteField("media_id", mediaID); err != nil {
				return "", fmt.Errorf("write append media_id field: %w", err)
			}
			if err := writer.WriteField("segment_index", strconv.Itoa(segmentIndex)); err != nil {
				return "", fmt.Errorf("write append segment_index field: %w", err)
			}

			part, err := writer.CreateFormFile("media", "chunk")
			if err != nil {
				return "", fmt.Errorf("create append chunk form file: %w", err)
			}
			if _, err := part.Write(chunkData); err != nil {
				return "", fmt.Errorf("write append chunk data: %w", err)
			}

			if err := writer.Close(); err != nil {
				return "", fmt.Errorf("close append multipart writer: %w", err)
			}

			appendReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
			if err != nil {
				return "", fmt.Errorf("create append request: %w", err)
			}
			appendReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			appendReq.Header.Set("Content-Type", writer.FormDataContentType())

			appendResp, err := client.Do(appendReq)
			if err != nil {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				return "", fmt.Errorf("execute append request segment %d: %w", segmentIndex, err)
			}

			if appendResp.StatusCode == http.StatusTooManyRequests {
				appendResp.Body.Close()
				return "", c.classifyRateLimit(appendResp)
			}

			if appendResp.StatusCode < 200 || appendResp.StatusCode >= 300 {
				respBody, _ := io.ReadAll(appendResp.Body)
				appendResp.Body.Close()
				return "", connectors.HTTPError(appendResp.StatusCode, fmt.Sprintf("x media append error segment %d: %d %s", segmentIndex, appendResp.StatusCode, string(respBody)))
			}
			appendResp.Body.Close()

			segmentIndex++
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				break
			}
			return "", fmt.Errorf("read chunk: %w", readErr)
		}
	}

	// 3. FINALIZE
	finalizeURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse media endpoint: %w", err)
	}
	fq := finalizeURL.Query()
	fq.Set("command", "FINALIZE")
	fq.Set("media_id", mediaID)
	finalizeURL.RawQuery = fq.Encode()

	finalizeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, finalizeURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create finalize request: %w", err)
	}
	finalizeReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	finalizeResp, err := client.Do(finalizeReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("execute finalize request: %w", err)
	}
	defer finalizeResp.Body.Close()

	if finalizeResp.StatusCode == http.StatusTooManyRequests {
		return "", c.classifyRateLimit(finalizeResp)
	}

	finalizeBody, _ := io.ReadAll(finalizeResp.Body)
	if finalizeResp.StatusCode != http.StatusOK && finalizeResp.StatusCode != http.StatusCreated && finalizeResp.StatusCode != http.StatusAccepted {
		return "", connectors.HTTPError(finalizeResp.StatusCode, fmt.Sprintf("x media finalize error: %d %s", finalizeResp.StatusCode, string(finalizeBody)))
	}

	var finalResp MediaUploadResponse
	if err := json.Unmarshal(finalizeBody, &finalResp); err != nil {
		return "", fmt.Errorf("decode finalize response: %w", err)
	}

	if finalID := finalResp.GetMediaID(); finalID != "" {
		mediaID = finalID
	}

	// 4. Polling if required
	if procInfo := finalResp.GetProcessingInfo(); procInfo != nil {
		return c.waitForProcessing(ctx, token, mediaID, procInfo)
	}

	return mediaID, nil
}

func (c *Connector) waitForProcessing(
	ctx context.Context,
	token string,
	mediaID string,
	procInfo *ProcessingInfo,
) (string, error) {
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := c.getMediaEndpoint()

	for {
		switch procInfo.State {
		case "succeeded":
			return mediaID, nil
		case "failed":
			errMsg := "media processing failed"
			if procInfo.Error != nil && procInfo.Error.Message != "" {
				errMsg = fmt.Sprintf("media processing failed: %s (code %d)", procInfo.Error.Message, procInfo.Error.Code)
			}
			return "", connectors.NewClassifiedError(
				connectors.ErrorPermanent,
				"MEDIA_PROCESSING_FAILED",
				errors.New(errMsg),
			)
		case "pending", "in_progress":
			waitSecs := procInfo.CheckAfterSecs
			if waitSecs <= 0 {
				waitSecs = 1
			}

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(waitSecs) * time.Second):
			}

			statusURL, err := url.Parse(endpoint)
			if err != nil {
				return "", fmt.Errorf("parse media endpoint: %w", err)
			}
			sq := statusURL.Query()
			sq.Set("command", "STATUS")
			sq.Set("media_id", mediaID)
			statusURL.RawQuery = sq.Encode()

			statusReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL.String(), nil)
			if err != nil {
				return "", fmt.Errorf("create status request: %w", err)
			}
			statusReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

			statusResp, err := client.Do(statusReq)
			if err != nil {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				return "", fmt.Errorf("execute status request: %w", err)
			}

			if statusResp.StatusCode == http.StatusTooManyRequests {
				statusResp.Body.Close()
				return "", c.classifyRateLimit(statusResp)
			}

			statusBody, _ := io.ReadAll(statusResp.Body)
			statusResp.Body.Close()

			if statusResp.StatusCode != http.StatusOK {
				return "", connectors.HTTPError(statusResp.StatusCode, fmt.Sprintf("x media status error: %d %s", statusResp.StatusCode, string(statusBody)))
			}

			var statusRespObj MediaUploadResponse
			if err := json.Unmarshal(statusBody, &statusRespObj); err != nil {
				return "", fmt.Errorf("decode status response: %w", err)
			}

			newProcInfo := statusRespObj.GetProcessingInfo()
			if newProcInfo == nil {
				return mediaID, nil
			}
			procInfo = newProcInfo

		default:
			return "", connectors.NewClassifiedError(
				connectors.ErrorPermanent,
				"UNKNOWN_PROCESSING_STATE",
				fmt.Errorf("unknown media processing state: %s", procInfo.State),
			)
		}
	}
}

func getMediaCategory(mediaType string) string {
	switch {
	case mediaType == "image/gif":
		return "tweet_gif"
	case strings.HasPrefix(mediaType, "image/"):
		return "tweet_image"
	case strings.HasPrefix(mediaType, "video/"):
		return "tweet_video"
	default:
		return "tweet_image"
	}
}

// Publish posts a tweet
func (c *Connector) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (publicURL string, platformPostID string, err error) {
	if token == "" {
		return "", "", fmt.Errorf("no access token set")
	}

	// Twitter/X constraint: A post may include up to 4 photos, 1 animated GIF, or 1 video.
	if len(mediaIDs) > 4 {
		return "", "", connectors.NewClassifiedError(
			connectors.ErrorPermanent,
			"INVALID_MEDIA_COUNT",
			fmt.Errorf("Twitter/X allows at most 4 media items per post, got %d", len(mediaIDs)),
		)
	}

	// Build tweet payload
	payload := map[string]any{
		"text": caption,
	}

	// Add media if provided
	if len(mediaIDs) > 0 {
		payload["media"] = map[string]any{
			"media_ids": mediaIDs,
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}

	endpoint := c.baseURL
	if endpoint == "" {
		endpoint = "https://api.x.com/2/tweets"
	}

	// Create request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		endpoint,
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	// Make request
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to publish tweet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", "", c.classifyRateLimit(resp)
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", connectors.HTTPError(resp.StatusCode, fmt.Sprintf("x api error: %d %s", resp.StatusCode, string(body)))
	}

	// Parse response
	var tweetResp TweetResponse
	if err := json.NewDecoder(resp.Body).Decode(&tweetResp); err != nil {
		return "", "", fmt.Errorf("failed to parse tweet response: %w", err)
	}

	// Format public URL
	publicURL = fmt.Sprintf("https://x.com/i/web/status/%s", tweetResp.Data.ID)

	return publicURL, tweetResp.Data.ID, nil
}

// GetStatus checks tweet status
func (c *Connector) GetStatus(ctx context.Context, platformPostID string) (status string, publicURL string, err error) {
	publicURL = fmt.Sprintf("https://x.com/i/web/status/%s", platformPostID)
	return "published", publicURL, nil
}

// ReconcilePublish reports an ambiguous result for X. The current X API integration
// has no reliable lookup keyed by the stable publication ID, so republishing is unsafe.
func (c *Connector) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}

// GetOAuthConfig returns the OAuth config (needed by AuthHandler)
func (c *Connector) GetOAuthConfig() *oauth2.Config {
	return c.oauthConfig
}
