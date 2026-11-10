package instagram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/iamvalson/blink/internal/connectors"
)

func TestInstagramLoginAndContainerContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/access_token":
			if r.Method != "POST" || r.FormValue("code") != "callback-code" {
				t.Error("incorrect code exchange")
			}
			fmt.Fprint(w, `{"access_token":"short","user_id":17841400000000001,"permissions":["instagram_business_basic","instagram_business_content_publish"]}`)
		case "/access_token":
			if r.URL.Query().Get("grant_type") != "ig_exchange_token" {
				t.Error("incorrect token exchange")
			}
			fmt.Fprint(w, `{"access_token":"long","expires_in":5184000}`)
		case "/refresh_access_token":
			if r.URL.Query().Get("access_token") != "long" || r.URL.Query().Get("grant_type") != "ig_refresh_token" {
				t.Error("incorrect renewal request")
			}
			fmt.Fprint(w, `{"access_token":"renewed","expires_in":5184000}`)
		case "/v26.0/me":
			if r.Header.Get("Authorization") != "Bearer long" {
				t.Error("missing user access token")
			}
			fmt.Fprint(w, `{"user_id":"123","id":"17841400000000001","username":"blink-test","profile_picture_url":"https://cdn.example/instagram.jpg"}`)
		case "/v26.0/123/media":
			if r.FormValue("media_type") != "REELS" || r.FormValue("video_url") == "" {
				t.Error("incorrect Reel request")
			}
			fmt.Fprint(w, `{"id":"456"}`)
		case "/v26.0/456":
			fmt.Fprint(w, `{"status_code":"FINISHED"}`)
		case "/v26.0/123/media_publish":
			if r.FormValue("creation_id") != "456" {
				t.Error("incorrect container submission")
			}
			fmt.Fprint(w, `{"id":"789"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := Connector{version: "v26.0", graph: server.URL, exchange: server.URL + "/oauth/access_token", http: server.Client()}
	ctx := context.Background()
	grant, err := c.Authenticate(ctx, connectors.AuthParams{Code: "callback-code"})
	if err != nil || grant.PlatformUserID != "123" || grant.ProviderSubject != "17841400000000001" || grant.Username != "blink-test" || grant.AvatarURL != "https://cdn.example/instagram.jpg" || grant.AccessToken != "long" {
		t.Fatalf("grant failed: %v", err)
	}
	renewed, err := c.RefreshAccessToken(ctx, grant.AccessToken)
	if err != nil || renewed.AccessToken != "renewed" {
		t.Fatalf("renewal failed: %v", err)
	}
	id, err := c.CreateContainer(ctx, renewed.AccessToken, grant.PlatformUserID, url.Values{"media_type": {"REELS"}, "video_url": {"https://cdn.example/test.mp4"}})
	if err != nil || id != "456" {
		t.Fatalf("container: %q %v", id, err)
	}
	status, err := c.ContainerStatus(ctx, renewed.AccessToken, id)
	if err != nil || status != "FINISHED" {
		t.Fatalf("status: %q %v", status, err)
	}
	media, err := c.PublishContainer(ctx, renewed.AccessToken, grant.PlatformUserID, id)
	if err != nil || media != "789" {
		t.Fatalf("publish: %q %v", media, err)
	}
}
