package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamvalson/blink/internal/config"
)

func TestRouterRegistersInstagramWebhook(t *testing.T) {
	cfg := config.InstagramConfig{WebhookVerifyToken: "test-token", WebhookAppSecret: "test-secret"}
	router := NewRouterWithOptions(RouterOptions{Instagram: cfg}, nil, nil, nil, "", nil, "", nil, nil, nil, nil, nil, false, true, http.SameSiteNoneMode, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/webhooks/instagram?hub.mode=subscribe&hub.verify_token=test-token&hub.challenge=01234", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "01234" {
		t.Fatalf("webhook verification returned %d, %q", response.Code, response.Body.String())
	}
}
