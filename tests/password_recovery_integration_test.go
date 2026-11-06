package tests

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/api"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/email"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type resetSender struct {
	urls, ids []string
	err       error
}

func (s *resetSender) SendPasswordReset(_ context.Context, _ string, link, id string) (string, error) {
	s.urls = append(s.urls, link)
	s.ids = append(s.ids, id)
	return "provider-id", s.err
}

type resetRateStore struct{}

func (resetRateStore) Take(context.Context, string, int, time.Duration) (time.Duration, error) {
	return 0, nil
}

func recoveryFixture(t *testing.T) (*pgxpool.Pool, *service.PasswordRecoveryService, *resetSender, http.Handler) {
	t.Helper()
	db := testDatabase(t)
	users := storage.NewUserRepository(db)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWTService(private, public).WithSessionVersionStore(users)
	sender := &resetSender{}
	recovery, err := service.NewPasswordRecoveryService(storage.NewPasswordRecoveryRepository(db), sender,
		"0123456789abcdef0123456789abcdef", "https://app.example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	protection, err := middleware.NewAuthProtection(resetRateStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	router := api.NewRouter(nil, nil, nil, "", nil, "https://app.example.com",
		service.NewSignupService(users, jwt), service.NewLoginService(users, jwt), service.NewMeService(users),
		jwt, nil, true, true, http.SameSiteLaxMode, nil, protection, recovery)
	return db, recovery, sender, router
}

func recoveryRequest(router http.Handler, path string, payload any, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body strings.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = *strings.NewReader(string(raw))
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
func registerRecoveryUser(t *testing.T, router http.Handler) *http.Cookie {
	t.Helper()
	rec := recoveryRequest(router, "/auth/signup", map[string]any{"email": "user@example.com", "display_name": "Tester", "password": "Original-password1!"}, nil)
	if rec.Code != 201 {
		t.Fatalf("signup: %d %s", rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}
func resetToken(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	values, err := url.ParseQuery(u.Fragment)
	if err != nil || u.RawQuery != "" || u.Path != "/reset-password" {
		t.Fatal("unsafe reset URL")
	}
	return values.Get("token")
}
func sessionRequest(router http.Handler, cookie *http.Cookie) int {
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestPasswordRecoveryEndToEndAndConcurrentConsumption(t *testing.T) {
	db, recovery, sender, router := recoveryFixture(t)
	oldCookie := registerRecoveryUser(t, router)
	unknown := recoveryRequest(router, "/auth/forgot-password", map[string]string{"email": "unknown@example.com"}, nil)
	known := recoveryRequest(router, "/auth/forgot-password", map[string]string{"email": " USER@EXAMPLE.COM "}, nil)
	if unknown.Code != 202 || known.Code != 202 || unknown.Body.String() != known.Body.String() {
		t.Fatal("email enumeration through response")
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM password_resets").Scan(&count); err != nil || count != 1 {
		t.Fatal("unknown user created reset records")
	}
	if len(sender.urls) != 0 {
		t.Fatal("request sent email synchronously")
	}
	if worked, err := recovery.DeliverNext(context.Background()); err != nil || !worked {
		t.Fatal(err)
	}
	token := resetToken(t, sender.urls[0])
	if len(token) != 43 {
		t.Fatal("reset secret has insufficient entropy")
	}
	var encrypted *string
	if err := db.QueryRow(context.Background(), "SELECT encrypted_email FROM password_resets").Scan(&encrypted); err != nil || encrypted != nil {
		t.Fatal("delivered secret retained")
	}
	weak := recoveryRequest(router, "/auth/reset-password", map[string]string{"token": token, "password": "weak"}, nil)
	if weak.Code != 400 {
		t.Fatal("weak password accepted")
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- recoveryRequest(router, "/auth/reset-password",
				map[string]string{"token": token, "password": "Replacement-password2!"}, nil).Code
		}()
	}
	wg.Wait()
	close(results)
	successes, invalid := 0, 0
	for status := range results {
		if status == 204 {
			successes++
		}
		if status == 400 {
			invalid++
		}
	}
	if successes != 1 || invalid != 1 {
		t.Fatalf("reset replay: success=%d invalid=%d", successes, invalid)
	}
	if sessionRequest(router, oldCookie) != 401 {
		t.Fatal("old session survived password reset")
	}
	oldLogin := recoveryRequest(router, "/auth/login", map[string]string{"email": "user@example.com", "password": "Original-password1!"}, nil)
	if oldLogin.Code != 401 {
		t.Fatal("old password survived reset")
	}
	newLogin := recoveryRequest(router, "/auth/login", map[string]string{"email": "user@example.com", "password": "Replacement-password2!"}, nil)
	if newLogin.Code != 200 || sessionRequest(router, newLogin.Result().Cookies()[0]) != 200 {
		t.Fatal("new password/session does not work")
	}
	var hash string
	var version int64
	if err := db.QueryRow(context.Background(), "SELECT password_hash,session_version FROM users WHERE email='user@example.com'").Scan(&hash, &version); err != nil || version != 1 || !auth.CheckPassword("Replacement-password2!", hash) {
		t.Fatal("password update and session revocation not atomic")
	}
}

func TestPasswordRecoveryCooldownExpiryAndReplacement(t *testing.T) {
	db, recovery, sender, router := recoveryFixture(t)
	registerRecoveryUser(t, router)
	ctx := context.Background()
	if err := recovery.Request(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	var firstID string
	if err := db.QueryRow(ctx, "SELECT id FROM password_resets").Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Request(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	var sameID string
	if err := db.QueryRow(ctx, "SELECT id FROM password_resets").Scan(&sameID); err != nil || firstID != sameID {
		t.Fatal("mailbox cooldown bypassed")
	}
	if _, err := recovery.DeliverNext(ctx); err != nil {
		t.Fatal(err)
	}
	oldToken := resetToken(t, sender.urls[0])
	execSQL(t, db, "UPDATE password_resets SET requested_at=NOW()-INTERVAL '61 seconds'")
	if err := recovery.Request(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Reset(ctx, oldToken, "Replacement-password2!"); !errors.Is(err, storage.ErrInvalidResetToken) {
		t.Fatal("superseded token accepted")
	}
	if _, err := recovery.DeliverNext(ctx); err != nil {
		t.Fatal(err)
	}
	token := resetToken(t, sender.urls[1])
	execSQL(t, db, "UPDATE password_resets SET expires_at=NOW()-INTERVAL '1 second'")
	if err := recovery.Reset(ctx, token, "Replacement-password2!"); !errors.Is(err, storage.ErrInvalidResetToken) {
		t.Fatal("expired token accepted")
	}
}

func TestPasswordRecoveryDeliveryRecoveryAndPermanentFailure(t *testing.T) {
	db, recovery, sender, router := recoveryFixture(t)
	registerRecoveryUser(t, router)
	ctx := context.Background()
	if err := recovery.Request(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	// A process died after claiming; another instance resumes the same request.
	repo := storage.NewPasswordRecoveryRepository(db)
	job, err := repo.Claim(ctx)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	execSQL(t, db, "UPDATE password_resets SET lease_until=NOW()-INTERVAL '1 second'")

	reclaimed, err := repo.Claim(ctx)
	if err != nil || reclaimed == nil || reclaimed.Attempt != 2 {
		t.Fatal("lease was not reclaimed")
	}
	if err := repo.Delivered(ctx, job.ID, "stale-provider", job.Attempt); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeliveryFailed(ctx, job.ID, job.Attempt, true); err != nil {
		t.Fatal(err)
	}
	var activeStatus string
	if err := db.QueryRow(ctx, "SELECT delivery_status FROM password_resets").Scan(&activeStatus); err != nil || activeStatus != "SENDING" {
		t.Fatal("stale worker changed a newer delivery claim")
	}
	execSQL(t, db, "UPDATE password_resets SET lease_until=NOW()-INTERVAL '1 second'")
	sender.err = errors.New("temporary provider failure")

	if _, err := recovery.DeliverNext(ctx); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, "UPDATE password_resets SET next_attempt_at=NOW()-INTERVAL '1 second'")
	sender.err = nil
	if _, err := recovery.DeliverNext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sender.ids) != 2 || sender.ids[0] != job.ID || sender.ids[1] != job.ID || sender.urls[0] != sender.urls[1] {
		t.Fatal("retry changed provider idempotency identity")
	}
	execSQL(t, db, "UPDATE password_resets SET requested_at=NOW()-INTERVAL '61 seconds'")
	if err := recovery.Request(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	sender.err = email.ErrPermanent
	if _, err := recovery.DeliverNext(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	var encrypted *string
	if err := db.QueryRow(ctx, "SELECT delivery_status,encrypted_email FROM password_resets").Scan(&status, &encrypted); err != nil || status != "FAILED" || encrypted != nil {
		t.Fatal("permanent failure retained secret or kept retrying")
	}
}
