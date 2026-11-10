package api

import (
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/config"
	"github.com/iamvalson/blink/internal/storage"
)

const instagramDeletionStatusPath = "/webhooks/instagram/deletion/"

func registerInstagramRoutes(r chi.Router, cfg config.InstagramConfig, accounts *storage.SocialAccountRepository) {
	if cfg.Enabled() {
		// The application's configuration validates the callback URL at startup.
		callback, err := url.Parse(cfg.CallbackURL)
		if err == nil && callback.Host != "" {
			privacy := handler.InstagramPrivacy{Secret: cfg.AppSecret, StatusURLPrefix: callback.Scheme + "://" + callback.Host + instagramDeletionStatusPath, Accounts: accounts}
			r.Post("/webhooks/instagram/deauthorize", privacy.Deauthorize)
			r.Post("/webhooks/instagram/data-deletion", privacy.Delete)
			r.Get(instagramDeletionStatusPath+"{code}", privacy.Status)
		}
	}
	if cfg.WebhookVerifyToken != "" {
		webhook := handler.InstagramWebhook{VerifyToken: cfg.WebhookVerifyToken, AppSecret: cfg.WebhookAppSecret}
		r.Get("/webhooks/instagram", webhook.Verify)
		r.Post("/webhooks/instagram", webhook.Receive)
	}
}
