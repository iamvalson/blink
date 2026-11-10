package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
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

// RevokeTokens disconnects the app from TikTok using the current access token.
func (c *Connector) RevokeTokens(ctx context.Context, accessToken, _ string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{
		"client_key":    {c.oauthConfig.ClientID},
		"client_secret": {c.oauthConfig.ClientSecret},
		"token":         {accessToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.tiktokapis.com/v2/oauth/revoke/", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build TikTok revocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("contact TikTok to revoke access: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Error string `json:"error"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result)
	if resp.StatusCode == http.StatusOK && (decodeErr == io.EOF || (decodeErr == nil && result.Error == "")) {
		return nil
	}
	if decodeErr == nil && result.Error == "invalid_token" &&
		(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized) {
		return nil
	}
	return fmt.Errorf("TikTok token revocation failed (HTTP %d)", resp.StatusCode)
}

// RefreshToken uses TikTok's OAuth token endpoint.

func (c *Connector) RefreshToken(ctx context.Context, refreshToken string) (connectors.TokenResult, error) {
	v := url.Values{
		"client_key":    {c.oauthConfig.ClientID},
		"client_secret": {c.oauthConfig.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	body := strings.NewReader(v.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthConfig.Endpoint.TokenURL, body)

	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("create TikTok token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	hc := c.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}

	res, err := hc.Do(req)

	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("send TikTok token request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorResponse struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}

		if err := json.NewDecoder(res.Body).Decode(&errorResponse); err != nil {
			return connectors.TokenResult{}, fmt.Errorf(
				"%w: TikTok returned HTTP %d with an unreadable error response",
				connectors.ErrTokenRefreshFailed,
				res.StatusCode,
			)
		}

		if errorResponse.Error == "invalid_grant" {
			return connectors.TokenResult{}, connectors.ErrInvalidRefreshToken
		}

		return connectors.TokenResult{}, fmt.Errorf(
			"%w: TikTok token endpoint returned HTTP %d (error: %s)",
			connectors.ErrTokenRefreshFailed,
			res.StatusCode,
			errorResponse.Error,
		)
	}

	// TikTok OAuth Token Response
	var tokenResponse TikTokTokenResponse

	err = json.NewDecoder(res.Body).Decode(&tokenResponse)

	if err != nil {
		return connectors.TokenResult{}, fmt.Errorf("decode TikTok token response: %w", err)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return connectors.TokenResult{}, fmt.Errorf("TikTok token response has an empty access token")
	}
	// Bound seconds before converting to nanoseconds to avoid duration overflow.
	if tokenResponse.ExpiresIn <= 0 || tokenResponse.ExpiresIn > math.MaxInt64/int64(time.Second) {
		return connectors.TokenResult{}, fmt.Errorf("TikTok token response has an invalid expires_in")
	}

	expiry := time.Now().Add(
		time.Duration(tokenResponse.ExpiresIn) * time.Second,
	)

	result := connectors.TokenResult{
		AccessToken: tokenResponse.AccessToken,
		ExpiresAt:   &expiry,
	}

	if tokenResponse.RefreshToken != "" {
		result.RefreshToken = &tokenResponse.RefreshToken
	}

	return result, nil
}

const tiktokUserInfoURL = "https://open.tiktokapis.com/v2/user/info/"

// connectedAccountProfile uses only fields covered by the scopes already granted.
// Profile lookup is optional so a transient error cannot block a connection.
func connectedAccountProfile(ctx context.Context, client *http.Client, token *oauth2.Token, openID string) (name, avatarURL string) {
	scope, _ := token.Extra("scope").(string)
	granted := make(map[string]bool)
	for _, value := range strings.Split(scope, ",") {
		granted[strings.TrimSpace(value)] = true
	}
	if !granted["user.info.basic"] {
		return "", ""
	}

	fields := "open_id,display_name,avatar_url"
	if granted["user.info.profile"] {
		fields += ",username"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tiktokUserInfoURL+"?"+url.Values{"fields": {fields}}.Encode(), nil)
	if err != nil {
		return "", ""
	}
	token.SetAuthHeader(req)
	response, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", ""
	}

	var profile struct {
		Data struct {
			User struct {
				OpenID      string `json:"open_id"`
				DisplayName string `json:"display_name"`
				Username    string `json:"username"`
				AvatarURL   string `json:"avatar_url"`
			} `json:"user"`
		} `json:"data"`
		Error TikTokAPIError `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, maxSuccessResponseBytes)).Decode(&profile) != nil ||
		profile.Error.Code != "ok" || profile.Data.User.OpenID != openID {
		return "", ""
	}
	if granted["user.info.profile"] && strings.TrimSpace(profile.Data.User.Username) != "" {
		return profile.Data.User.Username, profile.Data.User.AvatarURL
	}
	return profile.Data.User.DisplayName, profile.Data.User.AvatarURL
}
