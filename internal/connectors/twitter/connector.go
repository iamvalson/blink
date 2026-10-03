package twitter

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

type Connector struct {
	oauthConfig *oauth2.Config
	accessToken string
	baseURL     string
	httpClient  *http.Client
}

var _ connectors.OAuthConnector = (*Connector)(nil)

// New creates a new Twitter/X connector.
func New(cfg TwitterConfig) *Connector {
	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
	}
}

// PlatformID returns the stable slug used as route segment and storage key.
func (c *Connector) PlatformID() string { return connectors.PlatformTwitter }

// AuthCodeURL returns the URL the browser should be redirected to.
func (c *Connector) AuthCodeURL(state, verifier string) string {
	return GetAuthURL(c.oauthConfig, state, verifier)
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string) {
	c.accessToken = token
}

// Authenticate exchanges auth code for tokens and returns user ID
func (c *Connector) Authenticate(ctx context.Context, params connectors.AuthParams) (result connectors.AuthResult, err error) {
	// Exchange code for token
	token, err := ExchangeCodeForToken(ctx, c.oauthConfig, params.Code, params.CodeVerifier)
	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf("oauth exchange failed: %w", err)
	}

	// Get user info
	userInfo, err := GetUserInfo(ctx, token)
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

// UploadMedia uploads media to Twitter and returns media ID
func (c *Connector) UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (mediaID string, err error) {
	return "media_placeholder", nil
}

// Publish posts a tweet
func (c *Connector) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (publicURL string, platformPostID string, err error) {
	if token == "" {
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
		body, _ := io.ReadAll(resp.Body)
		policy := connectors.PlatformRateLimitPolicy{Platform: connectors.PlatformTwitter}
		rateLimitErr := policy.Classify(resp.StatusCode, resp.Header)
		if rateLimitErr != nil {
			if text := strings.TrimSpace(string(body)); text != "" {
				rateLimitErr.Message = fmt.Sprintf("%s: %s", rateLimitErr.Message, text)
			}
			return "", "", rateLimitErr
		}
		return "", "", connectors.NewRateLimitError(connectors.PlatformTwitter, resp.StatusCode, 0, nil, fmt.Sprintf("x api rate limit exceeded: %s", strings.TrimSpace(string(body))))
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", connectors.HTTPError(resp.StatusCode, fmt.Sprintf("x api error: %d %s", resp.StatusCode, string(body)))
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

// ReconcilePublish reports an ambiguous result for X. The current X API integration
// has no reliable lookup keyed by the stable publication ID, so republishing is unsafe.
func (c *Connector) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}

// GetOAuthConfig returns the OAuth config (needed by AuthHandler)
func (c *Connector) GetOAuthConfig() *oauth2.Config {
	return c.oauthConfig
}
