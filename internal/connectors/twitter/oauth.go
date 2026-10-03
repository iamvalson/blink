package twitter

import (
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

const userInfoURL = "https://api.x.com/2/users/me"

// NewOAuthConfig creates an OAuth2 config for Twitter
func NewOAuthConfig(cfg TwitterConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.CallbackURL,
		Scopes:       []string{"tweet.read", "tweet.write", "users.read", "offline.access"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://x.com/i/oauth2/authorize",
			TokenURL: "https://api.x.com/2/oauth2/token",
		},
	}
}

// GetAuthURL returns the URL user should visit to authorize
func GetAuthURL(oauthConfig *oauth2.Config, state, verifier string) string {
	return oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	)
}

// ExchangeCodeForToken swaps auth code for access token
func ExchangeCodeForToken(ctx context.Context, oauthConfig *oauth2.Config, code, verifier string) (*oauth2.Token, error) {
	token, err := oauthConfig.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("Failed to exchange code: %w", err)
	}
	return token, nil
}

// RefreshToken uses X OAuth 2.0 offline access. The worker persists any
// rotated refresh token; this method never handles ciphertext or storage.
func (c *Connector) RefreshToken(ctx context.Context, refreshToken string) (connectors.TokenResult, error) {
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthConfig.Endpoint.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("%w: create X token request", connectors.ErrTokenRefreshFailed)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.oauthConfig.ClientID, c.oauthConfig.ClientSecret)
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("%w: X token endpoint unavailable", connectors.ErrTokenRefreshFailed)
	}
	defer resp.Body.Close()
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return connectors.TokenResult{}, fmt.Errorf("%w: malformed X token response", connectors.ErrTokenRefreshFailed)
	}
	if resp.StatusCode >= 400 {
		if payload.Error == "invalid_grant" {
			return connectors.TokenResult{}, connectors.ErrInvalidRefreshToken
		}
		return connectors.TokenResult{}, fmt.Errorf("%w: X token endpoint returned %d", connectors.ErrTokenRefreshFailed, resp.StatusCode)
	}
	if payload.AccessToken == "" {
		return connectors.TokenResult{}, fmt.Errorf("%w: X returned no access token", connectors.ErrTokenRefreshFailed)
	}

	result := connectors.TokenResult{AccessToken: payload.AccessToken}
	if payload.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
		result.ExpiresAt = &expiresAt
	}
	if payload.RefreshToken != "" {
		result.RefreshToken = &payload.RefreshToken
	}
	return result, nil
}

// GetUserInfo fetches authenticated user's info
func GetUserInfo(ctx context.Context, token *oauth2.Token) (*TwitterUserInfo, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info: %d %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data TwitterUserInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &result.Data, nil
}
