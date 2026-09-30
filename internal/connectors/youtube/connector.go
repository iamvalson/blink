package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)


type Connector struct{
	oauthConfig	*oauth2.Config
	accessToken  string
	baseURL		 string
	httpClient	 *http.Client
}

var _ connectors.OAuthConnector = (*Connector)(nil)

// New creates a new Youtube connector
func New(cfg YouTubeConfig) *Connector {
	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
		httpClient: http.DefaultClient,
		baseURL: "https://www.googleapis.com/youtube/v3",
	}
}


//PlatformID returns the stable slug used as route segment and storage key
func (c *Connector) PlatformID() string {
	return connectors.PlatformYoutube
}


// AuthCodeURL returns the URL the browser should be  redirected to
func (c *Connector) AuthCodeURL(state, verifier string) string{
	return GetAuthURL(c.oauthConfig, state)
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string){
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
	if err != nil{
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
		AccessToken: token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry: token.Expiry,
	}, nil
}


// UploadMedia uploads media to the platform and returns a platform-specific
// media ID. For platforms with asynchronous processing, the returned ID
// can be used to poll processing status.
func (c *Connector) UploadMedia(
	ctx context.Context,
	token string,
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
	
	dummyMeta := map[string]interface{}{
		"snippet": map[string]string{
			"title": "Uploading...",
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
		return "", fmt.Errorf("upload init failed: %d %s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("chunk upload failed: %d %s", uploadResp.StatusCode, string(body))
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

	payload := map[string]interface{}{
		"id": videoID,
		"snippet": map[string]interface{}{
			"categoryId":  "22", // Default to People & Blogs
			"title":       title,
			"description": description,
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
		return "", "", fmt.Errorf("update metadata failed: %d %s", resp.StatusCode, string(respBody))
	}

	publicURL := fmt.Sprintf(
		"https://www.youtube.com/watch?v=%s",
		videoID,
	)

	return publicURL, videoID, nil
}

// GetStatus polls the platform for post/media status.
// Used for platforms with asynchronous publishing or processing.
func (c *Connector) GetStatus(
	ctx context.Context,
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

	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
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

	var result struct {
		Items []struct {
			Status struct {
				UploadStatus string `json:"uploadStatus"`
			} `json:"status"`
			ProcessingDetails struct {
				ProcessingStatus string `json:"processingStatus"`
			} `json:"processingDetails"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode status response: %w", err)
	}

	if len(result.Items) == 0 {
		return "", "", fmt.Errorf("video not found")
	}

	item := result.Items[0]
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