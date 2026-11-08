package tests

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors/tiktok"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/connectors/youtube"
	"github.com/iamvalson/blink/internal/storage"
)

type revokeTransport func(*http.Request) (*http.Response, error)

func (f revokeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYouTubeDisconnectRevokesAndDeletesAuthorizedData(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	const key = "0123456789abcdef0123456789abcdef"
	var userID, accountID, postID, targetID, attemptID string
	if err := db.QueryRow(ctx, "INSERT INTO users (email, password_hash) VALUES ('creator@example.com', 'test') RETURNING id").Scan(&userID); err != nil {
		t.Fatal(err)
	}

	accounts := storage.NewSocialAccountRepository(db)
	saveConnection := func(refresh string) {
		t.Helper()
		accessToken, err := auth.EncryptToken("access+token", key)
		if err != nil {
			t.Fatal(err)
		}
		var refreshToken string
		if refresh != "" {
			refreshToken, err = auth.EncryptToken(refresh, key)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := accounts.Upsert(ctx, userID, "youtube", "test-channel", accessToken, refreshToken, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	saveConnection("refresh+token")
	if err := accounts.Upsert(ctx, userID, "twitter", "test-x", "other-token", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		query string
		arg   *string
		id    *string
	}{
		{"SELECT id FROM social_accounts WHERE user_id = $1 AND platform = 'youtube'", &userID, &accountID},
		{"INSERT INTO posts (user_id) VALUES ($1) RETURNING id", &userID, &postID},
		{"INSERT INTO post_targets (post_id, social_account_id) SELECT id, $1 FROM posts RETURNING id", &accountID, &targetID},
		{"INSERT INTO publication_attempts (post_target_id, platform_post_id) VALUES ($1, 'youtube-video') RETURNING id", &targetID, &attemptID},
	} {
		if err := db.QueryRow(ctx, fixture.query, *fixture.arg).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO publish_results (publication_attempt_id, post_target_id, platform_post_id, status) VALUES ($1, $2, 'youtube-video', 'SUCCEEDED')", attemptID, targetID); err != nil {
		t.Fatal(err)
	}

	wantToken, status, body, calls := "refresh+token", http.StatusServiceUnavailable, "", 0
	client := &http.Client{Transport: revokeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.String() != "https://oauth2.googleapis.com/revoke" {
			t.Fatalf("unexpected Google request: %s %s", r.Method, r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.PostForm.Get("token"); got != wantToken {
			t.Fatalf("unexpected revocation token: %q", got)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	connector := youtube.New(youtube.YouTubeConfig{}, client)
	svc := service.NewAccountsService(accounts, nil, service.NewTokenManagerService(accounts, db, key), map[string]service.AccountTokenRevoker{"youtube": connector})

	checkRecords := func(want int) {
		t.Helper()
		for _, query := range []string{
			"SELECT count(*) FROM social_accounts WHERE platform = 'youtube'",
			"SELECT count(*) FROM post_targets",
			"SELECT count(*) FROM publication_attempts",
			"SELECT count(*) FROM publish_results",
		} {
			var got int
			if err := db.QueryRow(ctx, query).Scan(&got); err != nil || got != want {
				t.Fatalf("%s: got %d, want %d, err=%v", query, got, want, err)
			}
		}
	}
	if err := svc.DisconnectAccount(ctx, userID, "youtube"); err == nil {
		t.Fatal("expected Google failure to keep the connection retryable")
	}
	checkRecords(1)

	status = http.StatusOK
	if err := svc.DisconnectAccount(ctx, userID, "youtube"); err != nil {
		t.Fatal(err)
	}
	checkRecords(0)
	var preserved int
	if err := db.QueryRow(ctx, "SELECT (SELECT count(*) FROM posts WHERE id = $1) + (SELECT count(*) FROM social_accounts WHERE user_id = $2 AND platform = 'twitter')", postID, userID).Scan(&preserved); err != nil || preserved != 2 {
		t.Fatalf("expected the original post and X account to survive: count=%d err=%v", preserved, err)
	}
	if err := svc.DisconnectAccount(ctx, userID, "youtube"); err != nil || calls != 2 {
		t.Fatalf("repeat disconnect should not call Google again: calls=%d err=%v", calls, err)
	}

	saveConnection("")
	wantToken, status, body = "access+token", http.StatusBadRequest, `{"error":"invalid_token"}`
	if err := svc.DisconnectAccount(ctx, userID, "youtube"); err != nil || calls != 3 {
		t.Fatalf("already revoked access token should allow cleanup: calls=%d err=%v", calls, err)
	}
	checkRecords(0)
}

func TestDisconnectTikTokAndX(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	const key = "0123456789abcdef0123456789abcdef"
	var userID string
	if err := db.QueryRow(ctx, "INSERT INTO users (email, password_hash) VALUES ('disconnect@example.com', 'test') RETURNING id").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	accounts := storage.NewSocialAccountRepository(db)
	access, err := auth.EncryptToken("original-access", key)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := auth.EncryptToken("original-refresh", key)
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"tiktok", "twitter"} {
		if err := accounts.Upsert(ctx, userID, platform, "provider-id", access, refresh, time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	type expectedRequest struct {
		url, token, body string
		status           int
	}
	requests := []expectedRequest{
		{"https://open.tiktokapis.com/v2/oauth/token/", "original-refresh", `{"access_token":"fresh-access","refresh_token":"rotated-refresh","expires_in":3600}`, 200},
		{"https://open.tiktokapis.com/v2/oauth/revoke/", "fresh-access", `{"error":"server_error"}`, 200},
		{"https://open.tiktokapis.com/v2/oauth/revoke/", "fresh-access", "", 200},
		{"https://api.x.com/2/oauth2/revoke", "original-refresh", "", 200},
		{"https://api.x.com/2/oauth2/revoke", "original-access", "", 503},
		{"https://api.x.com/2/oauth2/revoke", "original-refresh", "", 200},
		{"https://api.x.com/2/oauth2/revoke", "original-access", "", 200},
	}
	client := &http.Client{Transport: revokeTransport(func(r *http.Request) (*http.Response, error) {
		if len(requests) == 0 {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		want := requests[0]
		requests = requests[1:]
		if r.Method != http.MethodPost || r.URL.String() != want.url {
			t.Fatalf("got %s %s, want POST %s", r.Method, r.URL, want.url)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		tokenField := "token"
		switch r.URL.Host {
		case "open.tiktokapis.com":
			if r.PostForm.Get("client_key") != "tiktok-key" || r.PostForm.Get("client_secret") != "tiktok-secret" {
				t.Fatal("missing TikTok client credentials")
			}
			if r.URL.Path == "/v2/oauth/token/" {
				tokenField = "refresh_token"
			}
		case "api.x.com":
			id, secret, ok := r.BasicAuth()
			if !ok || id != "x-id" || secret != "x-secret" {
				t.Fatal("missing X confidential client credentials")
			}
		}
		if r.PostForm.Get(tokenField) != want.token {
			t.Fatalf("wrong %s sent to %s", tokenField, r.URL)
		}
		return &http.Response{StatusCode: want.status, Body: io.NopCloser(strings.NewReader(want.body))}, nil
	})}
	tiktokConnector := tiktok.New(tiktok.TikTokConfig{ClientKey: "tiktok-key", ClientSecret: "tiktok-secret"}, client)
	xConnector := twitter.New(twitter.TwitterConfig{ClientID: "x-id", ClientSecret: "x-secret"}, client)
	svc := service.NewAccountsService(accounts, tiktokConnector, service.NewTokenManagerService(accounts, db, key),
		map[string]service.AccountTokenRevoker{"tiktok": tiktokConnector, "twitter": xConnector})
	for _, platform := range []string{"tiktok", "twitter"} {
		if err := svc.DisconnectAccount(ctx, userID, platform); err == nil {
			t.Fatalf("expected %s revocation to fail", platform)
		}
		if _, err := accounts.GetAccountID(ctx, userID, platform); err != nil {
			t.Fatalf("%s should remain connected for a retry: %v", platform, err)
		}
		if err := svc.DisconnectAccount(ctx, userID, platform); err != nil {
			t.Fatalf("%s retry failed: %v", platform, err)
		}
		if _, err := accounts.GetAccountID(ctx, userID, platform); !errors.Is(err, storage.ErrSocialAccountNotFound) {
			t.Fatalf("%s connection should be gone: %v", platform, err)
		}
	}
	if len(requests) != 0 {
		t.Fatalf("missing %d provider requests", len(requests))
	}
}
