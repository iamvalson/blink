package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
