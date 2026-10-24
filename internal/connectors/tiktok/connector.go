package tiktok

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/iamvalson/blink/internal/connectors"
	"golang.org/x/oauth2"
)

type Connector struct {
	oauthConfig *oauth2.Config
	accessToken string
	httpClient  *http.Client
}

var _ connectors.OAuthConnector = (*Connector)(nil)

// New creates a new TikTok connector
func New(cfg TikTokConfig, client ...*http.Client) *Connector {
	hc := &http.Client{
		Timeout: 30 * time.Second,
	}

	if len(client) > 0 && client[0] != nil {
		hc = client[0]
	}

	return &Connector{
		oauthConfig: NewOAuthConfig(cfg),
		httpClient:  hc,
	}
}

// PlatformID returns the stable slug used as route segment and storage key
func (c *Connector) PlatformID() string {
	return connectors.PlatformTikTok
}

// AuthCodeURL returns the URL the browser should be redirected to
func (c *Connector) AuthCodeURL(state, verifier string) (string, error) {
	return GetAuthURL(c.oauthConfig, state)
}

// SetAccessToken sets the user's OAuth token
func (c *Connector) SetAccessToken(token string) {
	c.accessToken = token
}

// Authenticate exchanges the OAuth code for tokens and returns the TikTok credentials
func (c *Connector) Authenticate(ctx context.Context, params connectors.AuthParams) (connectors.AuthResult, error) {
	token, err := ExchangeCodeForToken(ctx, c.oauthConfig, params.Code, c.httpClient)

	if err != nil {
		return connectors.AuthResult{}, fmt.Errorf(
			"oauth exchange failed: %w",
			err,
		)
	}

	openID, ok := token.Extra("open_id").(string)
	if !ok || openID == "" {
		return connectors.AuthResult{}, fmt.Errorf(
			"TikTok token response missing open_id",
		)
	}

	c.accessToken = token.AccessToken

	return connectors.AuthResult{
		PlatformUserID: openID,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		Expiry:         token.Expiry,
	}, nil
}

// UploadMedia uploads binary media (not yet implemented for TikTok)
func (c *Connector) UploadMedia(ctx context.Context, token string, attemptID string, media io.Reader, mediaType string) (string, error) {
	return "", errors.New("not implemented")
}

// Publish posts content to TikTok (not yet implemented)
func (c *Connector) Publish(ctx context.Context, token string, attemptID string, caption string, mediaIDs ...string) (string, string, error) {
	return "", "", errors.New("not implemented")
}

// GetStatus polls TikTok for post status (not yet implemented)
func (c *Connector) GetStatus(ctx context.Context, platformPostID string) (string, string, error) {
	return "", "", errors.New("not implemented")
}

// ReconcilePublish resolves an ambiguous publication (not yet implemented)
func (c *Connector) ReconcilePublish(ctx context.Context, token string, platformUserID string, attemptID string, caption string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: connectors.ReconciliationUnknown}, errors.New("not implemented")
}
