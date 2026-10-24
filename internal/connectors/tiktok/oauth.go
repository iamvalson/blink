package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

func NewOAuthConfig(cfg TikTokConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientKey,
		ClientSecret: cfg.ClientSecret,
		Scopes:       cfg.Scopes,
		RedirectURL:  cfg.RedirectURI,
		Endpoint:     tiktokOAuthEndpoint,
	}
}

// TikTok OAuth Endpoints
var tiktokOAuthEndpoint = oauth2.Endpoint{
	AuthURL:  "https://www.tiktok.com/v2/auth/authorize/",
	TokenURL: "https://open.tiktokapis.com/v2/oauth/token/",
}

// GetAuthURL returns the URL the user should visit to authorize
func GetAuthURL(oauthConfig *oauth2.Config, state string) (string, error) {
	authURL := oauthConfig.AuthCodeURL(state)

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("parse TikTok authorization URL: %w", err)
	}

	params := parsedURL.Query()

	params.Del("client_id")
	params.Set("client_key", oauthConfig.ClientID)

	params.Set("scope", strings.Join(oauthConfig.Scopes, ","))

	parsedURL.RawQuery = params.Encode()

	return parsedURL.String(), nil
}

// ExchangeCodeForToken exchanges the authorization code for tokens
func ExchangeCodeForToken(ctx context.Context, oauthConfig *oauth2.Config, code string, client *http.Client) (*oauth2.Token, error) {
	v := url.Values{
		"client_key":    {oauthConfig.ClientID},
		"grant_type":    {"authorization_code"},
		"client_secret": {oauthConfig.ClientSecret},
		"code":          {code},
		"redirect_uri":  {oauthConfig.RedirectURL},
	}

	body := strings.NewReader(v.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthConfig.Endpoint.TokenURL, body)

	if err != nil {
		return nil, fmt.Errorf("create TikTok token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	hc := client
	if hc == nil {
		hc = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	res, err := hc.Do(req)

	if err != nil {
		return nil, fmt.Errorf("send TikTok token request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorResponse struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.NewDecoder(res.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("TikTok token endpoint returned %d: decode error response: %w", res.StatusCode, err)
		}
		if errorResponse.ErrorDescription != "" {
			return nil, fmt.Errorf("TikTok token endpoint returned %d: %s: %s", res.StatusCode, errorResponse.Error, errorResponse.ErrorDescription)
		}
		if errorResponse.Error != "" {
			return nil, fmt.Errorf("TikTok token endpoint returned %d: %s", res.StatusCode, errorResponse.Error)
		}
		return nil, fmt.Errorf("TikTok token endpoint returned %d", res.StatusCode)
	}

	// TikTok OAuth Token Response
	var tokenResponse TikTokTokenResponse

	err = json.NewDecoder(res.Body).Decode(&tokenResponse)

	if err != nil {
		return nil, fmt.Errorf("decode TikTok token response: %w", err)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return nil, fmt.Errorf("TikTok token response has an empty access token")
	}
	// Bound seconds before converting to nanoseconds to avoid duration overflow.
	if tokenResponse.ExpiresIn <= 0 || tokenResponse.ExpiresIn > math.MaxInt64/int64(time.Second) {
		return nil, fmt.Errorf("TikTok token response has an invalid expires_in")
	}

	expiry := time.Now().Add(
		time.Duration(tokenResponse.ExpiresIn) * time.Second,
	)

	token := &oauth2.Token{
		AccessToken:  tokenResponse.AccessToken,
		RefreshToken: tokenResponse.RefreshToken,
		TokenType:    tokenResponse.TokenType,
		Expiry:       expiry,
	}

	return token.WithExtra(map[string]any{
		"open_id":            tokenResponse.OpenId,
		"scope":              tokenResponse.Scope,
		"refresh_expires_in": tokenResponse.RefreshExpiresIn,
	}), nil
}
