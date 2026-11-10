package config

import (
	"errors"
	"net/url"
	"regexp"
)

type InstagramConfig struct {
	AppID              string
	AppSecret          string
	CallbackURL        string
	APIVersion         string
	WebhookVerifyToken string
	WebhookAppSecret   string
}

func (c InstagramConfig) Enabled() bool {
	return c.AppID != "" || c.AppSecret != "" || c.CallbackURL != "" || c.APIVersion != ""
}

var instagramAPIVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)

func (c InstagramConfig) validate() error {
	if c.WebhookVerifyToken != "" && c.WebhookAppSecret == "" {
		return errors.New("META_APP_SECRET is required when Instagram webhooks are configured")
	}
	if !c.Enabled() {
		return nil
	}
	callback, err := url.Parse(c.CallbackURL)
	if c.AppID == "" || c.AppSecret == "" || err != nil || callback.Scheme != "https" || callback.Hostname() == "" ||
		callback.User != nil || callback.RawQuery != "" || callback.ForceQuery || callback.Fragment != "" ||
		callback.Path != "/auth/instagram/callback" || !instagramAPIVersion.MatchString(c.APIVersion) {
		return errors.New("Instagram requires app ID, app secret, HTTPS /auth/instagram/callback and a pinned API version")
	}
	return nil
}
