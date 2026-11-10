package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/connectors"
)

const Platform = "instagram"

type Connector struct {
	id, secret, callback, version string
	http                          *http.Client
	graph, exchange               string
}

// New constructs the Instagram connector for the validated application configuration.
func New(cfg config.InstagramConfig) *Connector {
	if !cfg.Enabled() {
		return nil
	}
	return &Connector{
		id: cfg.AppID, secret: cfg.AppSecret, callback: cfg.CallbackURL, version: cfg.APIVersion,
		graph: "https://graph.instagram.com", exchange: "https://api.instagram.com/oauth/access_token",
		http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

var _ connectors.OAuthConnector = (*Connector)(nil)

func (*Connector) PlatformID() string { return Platform }
func (c *Connector) call(ctx context.Context, method, endpoint, token string, values url.Values, dest any) error {
	var body io.Reader
	if method == "GET" && len(values) > 0 {
		endpoint += "?" + values.Encode()
	}
	if method == "POST" {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("invalid Instagram request")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return connectors.NewClassifiedError(connectors.ErrorRetryable, "INSTAGRAM_NETWORK", errors.New("Instagram request did not complete"))
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return connectors.NewClassifiedError(connectors.ErrorRetryable, "INSTAGRAM_RESPONSE", errors.New("failed to read Instagram response"))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var fault struct {
			Error struct {
				Code      int  `json:"code"`
				Transient bool `json:"is_transient"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &fault)
		code := fault.Error.Code
		if code == 190 || response.StatusCode == http.StatusUnauthorized {
			return connectors.ErrAuthFailed
		}
		if response.StatusCode == 429 || code == 4 || code == 17 || code == 32 || code == 613 || code == 80002 {
			seconds, _ := strconv.Atoi(response.Header.Get("Retry-After"))
			return connectors.NewRateLimitError(Platform, response.StatusCode, time.Duration(seconds)*time.Second, nil, "Instagram rate limit")
		}
		if fault.Error.Transient {
			return connectors.NewClassifiedError(connectors.ErrorRetryable, "INSTAGRAM_TRANSIENT", errors.New("temporary Instagram API failure"))
		}
		return connectors.HTTPError(response.StatusCode, fmt.Sprintf("Instagram rejected the request (code %d, HTTP %d)", code, response.StatusCode))
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return errors.New("invalid Instagram response")
	}
	return nil
}

// Publishing is driven through the durable container workflow.
func (*Connector) UploadMedia(context.Context, string, string, io.Reader, string) (string, error) {
	return "", connectors.ErrUnsupportedMedia
}
func (*Connector) Publish(context.Context, string, string, string, ...string) (string, string, error) {
	return "", "", connectors.ErrPublishFailed
}
func (*Connector) GetStatus(context.Context, string) (string, string, error) {
	return "", "", connectors.ErrPublishFailed
}
func (*Connector) ReconcilePublish(context.Context, string, string, string, string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, nil
}
