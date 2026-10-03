package youtube

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

var channelInfoURL = "https://www.googleapis.com/youtube/v3/channels"

// NewOAuthConfig creates an OAuth2 config for youtube
func NewOAuthConfig(cfg YouTubeConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.CallbackURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/youtube.upload",
			"https://www.googleapis.com/auth/youtube.readonly",
			"https://www.googleapis.com/auth/youtube",
		},
		Endpoint: googleOAuthEndpoint,
	}
}

// Google OAuth Endpoints
var googleOAuthEndpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
}

// GetAuthURL returns the URL the user should visit to authorize YouTube
func GetAuthURL(oauthConfig *oauth2.Config, state string) string {
	return oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
}

// ExchangeCodeForToken exchanges the authorization code for tokens
func ExchangeCodeForToken(
	ctx context.Context,
	oauthConfig *oauth2.Config,
	code string,
) (*oauth2.Token, error) {
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	return token, nil
}

// RefreshToken uses Google's OAuth token endpoint. Google commonly omits a
// replacement refresh token, which is represented by a nil result field.
func (c *Connector) RefreshToken(ctx context.Context, refreshToken string) (connectors.TokenResult, error) {
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthConfig.Endpoint.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("%w: create Google token request", connectors.ErrTokenRefreshFailed)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.oauthConfig.ClientID, c.oauthConfig.ClientSecret)
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("%w: Google token endpoint unavailable", connectors.ErrTokenRefreshFailed)
	}
	defer resp.Body.Close()
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return connectors.TokenResult{}, fmt.Errorf("%w: malformed Google token response", connectors.ErrTokenRefreshFailed)
	}
	if resp.StatusCode >= 400 {
		if payload.Error == "invalid_grant" {
			return connectors.TokenResult{}, connectors.ErrInvalidRefreshToken
		}
		return connectors.TokenResult{}, fmt.Errorf("%w: Google token endpoint returned %d", connectors.ErrTokenRefreshFailed, resp.StatusCode)
	}
	if payload.AccessToken == "" {
		return connectors.TokenResult{}, fmt.Errorf("%w: Google returned no access token", connectors.ErrTokenRefreshFailed)
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

// GetChannelInfo fetches the authenticated user's YouTube channel
func GetChannelInfo(
	ctx context.Context,
	token *oauth2.Token,
) (*YouTubeChannelInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		channelInfoURL+"?part=snippet&mine=true",
		nil,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create channel request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch channel info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf(
			"failed to get channel info: %d %s",
			resp.StatusCode,
			string(body),
		)
	}

	var result struct {
		Items []YouTubeChannelInfo `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf(
			"failed to decode channel info: %w",
			err,
		)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no YouTube channel found")
	}

	return &result.Items[0], nil
}
