package tiktok

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/model"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rewriteTransport struct{ target string }

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	u := *r.URL
	r2.URL = &u
	r2.URL.Scheme = "http"
	r2.URL.Host = strings.TrimPrefix(t.target, "http://")
	return http.DefaultTransport.RoundTrip(r2)
}
func TestValidateMediaURL(t *testing.T) {
	for _, raw := range []string{"http://media.example/tiktok-media/id", "https://evil.example/tiktok-media/id", "https://media.example/tiktok-media-evil/id", "https://user@media.example/tiktok-media/id", "https://media.example/tiktok-media/%2e%2e/private", "https://127.0.0.1/tiktok-media/id", "https://media.example/tiktok-media/id#secret"} {
		if ValidateMediaURL(raw, "https://media.example/tiktok-media") == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if err := ValidateMediaURL("https://media.example/tiktok-media/id?signature=secret", "https://media.example/tiktok-media"); err != nil {
		t.Fatal(err)
	}
}
func TestInitializeAndFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("invalid request headers")
		}
		if strings.Contains(r.URL.Path, "/video/init/") {
			var req DirectPostRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.SourceInfo.Source != "PULL_FROM_URL" || req.PostInfo.PrivacyLevel != "SELF_ONLY" || !req.PostInfo.DisableComment {
				t.Errorf("bad request: %+v", req)
			}
			if _, err := io.WriteString(w, `{"data":{"publish_id":"op-1"},"error":{"code":"ok"}}`); err != nil {
				t.Errorf("write initialization response: %v", err)
			}
		} else {
			if _, err := io.WriteString(w, `{"data":{"status":"PUBLISH_COMPLETE","publicaly_available_post_id":[9223372036854775806]},"error":{"code":"ok"}}`); err != nil {
				t.Errorf("write status response: %v", err)
			}
		}
	}))
	defer server.Close()
	c := New(TikTokConfig{MediaURLPrefix: "https://media.example/tiktok-media"}, &http.Client{Transport: rewriteTransport{server.URL}})
	id, err := c.InitializeVideo(context.Background(), "token", connectors.VideoPublishInput{MediaURL: "https://media.example/tiktok-media/id", Options: model.TikTokOptions{PrivacyLevel: "SELF_ONLY", Consent: true, DurationSeconds: 5, DisableComment: true}})
	if err != nil || id != "op-1" {
		t.Fatalf("%s %v", id, err)
	}
	status, err := c.FetchPublishStatus(context.Background(), "token", id)
	if err != nil || status.Status != "PUBLISH_COMPLETE" || status.PublicPostIDs[0] != "9223372036854775806" {
		t.Fatalf("%+v %v", status, err)
	}
}
func TestInitializationErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		class      connectors.ErrorClass
	}{{"lost_response", "bad", 200, connectors.ErrorAmbiguous}, {"server", "{}", 503, connectors.ErrorAmbiguous}, {"rate", `{"error":{"code":"rate_limit_exceeded"}}`, 429, connectors.ErrorRetryable}, {"privacy", `{"error":{"code":"privacy_level_option_mismatch","message":"secret-url"}}`, 400, connectors.ErrorPermanent}, {"missing_id", `{"data":{},"error":{"code":"ok"}}`, 200, connectors.ErrorAmbiguous}} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Errorf("write error response: %v", err)
				}
			}))
			defer s.Close()
			c := New(TikTokConfig{MediaURLPrefix: "https://media.example/tiktok-media"}, &http.Client{Transport: rewriteTransport{s.URL}})
			_, err := c.InitializeVideo(context.Background(), "token", connectors.VideoPublishInput{MediaURL: "https://media.example/tiktok-media/id", Options: model.TikTokOptions{PrivacyLevel: "SELF_ONLY", Consent: true, DurationSeconds: 1}})
			if err == nil || connectors.ClassifyError(err) != tc.class || strings.Contains(err.Error(), "secret-url") {
				t.Fatalf("%v", err)
			}
		})
	}
}
func TestCreatorOptions(t *testing.T) {
	c := &TikTokQueryCreatorDataResponse{PrivacyLevel: []string{"SELF_ONLY"}, MaxVideoPostDuration: 60, DuetDisabled: true}
	o := model.TikTokOptions{PrivacyLevel: "SELF_ONLY", DurationSeconds: 30, DisableDuet: true}
	if err := ValidateCreatorOptions(o, c); err != nil {
		t.Fatal(err)
	}
	o.DisableDuet = false
	if ValidateCreatorOptions(o, c) == nil {
		t.Fatal("accepted forbidden duet")
	}
	o.DisableDuet = true
	o.DurationSeconds = 61
	if ValidateCreatorOptions(o, c) == nil {
		t.Fatal("accepted duration")
	}
	o.DurationSeconds = 30
	o.PrivacyLevel = "PUBLIC_TO_EVERYONE"
	if ValidateCreatorOptions(o, c) == nil {
		t.Fatal("accepted privacy")
	}
}

func TestUnauditedTikTokAccountRejectionIsActionable(t *testing.T) {
	err := classifyTikTokAPIError("unaudited_client_can_only_post_to_private_accounts", "", "", http.StatusForbidden, nil)
	if connectors.ClassifyError(err) != connectors.ErrorPermanent || errors.Is(err, connectors.ErrAuthFailed) {
		t.Fatalf("account visibility restriction misclassified as credentials: %v", err)
	}
	if !strings.Contains(err.Error(), "private creator account") || !strings.Contains(err.Error(), "Only me") {
		t.Fatalf("missing recovery instructions: %v", err)
	}
}
