package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
)

func (c *Connector) AuthCodeURL(state, _ string) (string, error) {
	q := url.Values{"client_id": {c.id}, "redirect_uri": {c.callback}, "response_type": {"code"}, "state": {state}, "scope": {"instagram_business_basic,instagram_business_content_publish"}, "enable_fb_login": {"0"}}
	return "https://www.instagram.com/oauth/authorize?" + q.Encode(), nil
}

type tokenResponse struct {
	Token       string           `json:"access_token"`
	Lifetime    int              `json:"expires_in"`
	Permissions json.RawMessage  `json:"permissions"`
	UserID      remoteIdentifier `json:"user_id"`
}

func (c *Connector) Authenticate(ctx context.Context, params connectors.AuthParams) (connectors.AuthResult, error) {
	var short tokenResponse
	err := c.call(ctx, "POST", c.exchange, "", url.Values{"client_id": {c.id}, "client_secret": {c.secret}, "redirect_uri": {c.callback}, "grant_type": {"authorization_code"}, "code": {params.Code}}, &short)
	if err != nil {
		return connectors.AuthResult{}, err
	}
	var scopes []string
	if json.Unmarshal(short.Permissions, &scopes) != nil {
		var csv string
		if json.Unmarshal(short.Permissions, &csv) == nil {
			scopes = strings.Split(csv, ",")
		}
	}
	for _, required := range []string{"instagram_business_basic", "instagram_business_content_publish"} {
		found := false
		for _, scope := range scopes {
			if strings.TrimSpace(scope) == required {
				found = true
			}
		}
		if !found {
			return connectors.AuthResult{}, errors.New("Instagram did not grant the required publishing permissions")
		}
	}
	if short.Token == "" {
		return connectors.AuthResult{}, errors.New("missing short-lived Instagram token")
	}
	result, err := c.longToken(ctx, "/access_token", url.Values{"grant_type": {"ig_exchange_token"}, "client_secret": {c.secret}, "access_token": {short.Token}})
	if err != nil {
		return connectors.AuthResult{}, err
	}
	var profile struct {
		UserID    remoteIdentifier `json:"user_id"`
		ID        remoteIdentifier `json:"id"`
		Username  string           `json:"username"`
		AvatarURL string           `json:"profile_picture_url"`
	}
	endpoint := c.graph + "/" + c.version + "/me"
	fields := url.Values{"fields": {"user_id,id,username,profile_picture_url"}}
	if err := c.call(ctx, http.MethodGet, endpoint, result.AccessToken, fields, &profile); err != nil {
		var rejected *connectors.ClassifiedError
		if !errors.As(err, &rejected) || rejected.StatusCode != http.StatusBadRequest {
			return connectors.AuthResult{}, err
		}
		// The optional picture field must never block account connection.
		fields.Set("fields", "user_id,id,username")
		if err := c.call(ctx, http.MethodGet, endpoint, result.AccessToken, fields, &profile); err != nil {
			return connectors.AuthResult{}, err
		}
	}
	if profile.UserID == "" {
		profile.UserID = profile.ID
	}
	if !remoteID.MatchString(string(profile.UserID)) {
		return connectors.AuthResult{}, errors.New("Instagram returned no valid professional account ID")
	}
	return connectors.AuthResult{PlatformUserID: string(profile.UserID), ProviderSubject: string(short.UserID), Username: profile.Username, AvatarURL: profile.AvatarURL, AccessToken: result.AccessToken, Expiry: *result.ExpiresAt}, nil
}

type remoteIdentifier string

func (id *remoteIdentifier) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}
	value := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
	}
	if !remoteID.MatchString(value) {
		return errors.New("invalid Instagram account ID")
	}
	*id = remoteIdentifier(value)
	return nil
}

var remoteID = regexp.MustCompile(`^[0-9]+$`)

func (c *Connector) RefreshAccessToken(ctx context.Context, current string) (connectors.TokenResult, error) {
	result, err := c.longToken(ctx, "/refresh_access_token", url.Values{"grant_type": {"ig_refresh_token"}, "access_token": {current}})
	if errors.Is(err, connectors.ErrAuthFailed) {
		return result, connectors.ErrInvalidRefreshToken
	}
	return result, err
}
func (c *Connector) longToken(ctx context.Context, path string, params url.Values) (connectors.TokenResult, error) {
	var response tokenResponse
	if err := c.call(ctx, "GET", c.graph+path, "", params, &response); err != nil {
		return connectors.TokenResult{}, err
	}
	if response.Token == "" || response.Lifetime <= 0 {
		return connectors.TokenResult{}, errors.New("invalid long-lived Instagram token response")
	}
	expiry := time.Now().Add(time.Duration(response.Lifetime) * time.Second)
	return connectors.TokenResult{AccessToken: response.Token, ExpiresAt: &expiry}, nil
}
