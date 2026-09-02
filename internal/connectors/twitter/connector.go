package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

type Connector struct {
	oauthConfig *oauth2.Config
	accessToken string
}

// New creates a new Twwitter connector
func New(cfg TwitterConfig) *Connector {
	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
	}
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string) {
	c.accessToken = token
}

// Authenticate exchanges auth code for tokens and returns user ID
func (c *Connector) Authenticate(ctx context.Context, code, verifier string) (platformUserID string, err error) {
	// Exchange code for token
	token, err := ExchangeCodeForToken(ctx, c.oauthConfig, code, verifier)
	if err != nil {
		return "", fmt.Errorf("oauth exchange failed: %w", err)
	}

	// Get user info
	userInfo, err := GetUserInfo(ctx, token)
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %w", err)
	}

	// Store token for later use (caller will encrypt and save to DB)
	c.accessToken = token.AccessToken

	return userInfo.ID, nil
}

// UploadMedia uploads media to Twitter and returns media ID
func (c *Connector) UploadMedia(ctx context.Context, media io.Reader, mediaType string) (mediaID string, err error) {
	return "media_placeholder", nil
}

// Publish posts a tweet
func (c *Connector) Publish(ctx context.Context, caption string, mediaIDs ...string) (publicURL string, platformPostID string, err error) {
	if c.accessToken == "" {
		return "", "", fmt.Errorf("no access token set")
	}

	// Build tweet payload
	payload := map[string]string{
		"text": caption,
	}

	// Add media if provided
	if len(mediaIDs) > 0 {
		fmt.Print("Todo")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}

	// Create request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.x.com/2/tweets",
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-Type", "application/json")

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to publish tweet: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", "", connectors.ErrRateLimited
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("x api error: %d %s", resp.StatusCode, string(body))
	}

	// Paarse response
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

// GetOAuthConfig returns the OAuth config (needed by AuthHandler)
func (c *Connector) GetOAuthConfig() *oauth2.Config {
	return c.oauthConfig
}
