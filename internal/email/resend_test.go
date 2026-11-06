package email

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendPasswordResetRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" ||
			r.Header.Get("Idempotency-Key") != "password-reset/request-id" {
			t.Error("incorrect provider request")
		}
		var body struct {
			From                string
			To                  []string
			Subject, Text, HTML string
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.From != "Blink <reset@example.com>" || len(body.To) != 1 || body.To[0] != "user@example.com" ||
			!strings.Contains(body.Text, "#token=secret") || !strings.Contains(body.HTML, "30 minutes") {
			t.Error("email payload missing recipient or reset instructions")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email-id"}`))
	}))
	defer server.Close()
	sender, err := NewResend("test-key", "Blink <reset@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	sender.endpoint = server.URL
	id, err := sender.SendPasswordReset(context.Background(), "user@example.com", "https://app.example.com/reset-password#token=secret", "request-id")
	if err != nil || id != "email-id" || calls != 1 {
		t.Fatalf("unexpected send result %q %v", id, err)
	}
}

func TestResendErrorsNeverExposeProviderBody(t *testing.T) {
	for _, status := range []int{400, 401, 429, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("private-recipient token=secret"))
		}))
		sender, _ := NewResend("test-key", "reset@example.com")
		sender.endpoint = server.URL
		_, err := sender.SendPasswordReset(context.Background(), "user@example.com", "https://app.example.com/#secret", "id")
		server.Close()
		if err == nil || strings.Contains(err.Error(), "private-recipient") || strings.Contains(err.Error(), "secret") {
			t.Fatal("unsafe provider error")
		}
		permanent := status == 400 || status == 401
		if errors.Is(err, ErrPermanent) != permanent {
			t.Fatalf("incorrect retry classification for %d", status)
		}
	}
}

func TestResendDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("followed provider redirect") }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	sender, _ := NewResend("test-key", "reset@example.com")
	sender.endpoint = server.URL
	if _, err := sender.SendPasswordReset(context.Background(), "user@example.com", "https://app.example.com/#secret", "id"); err == nil {
		t.Fatal("accepted redirect")
	}
}

func TestResendDiagnosticsAreAllowlisted(t *testing.T) {
	for _, tc := range []struct {
		name, message, code, reason string
		status                      int
	}{
		{"validation_error", "The private-domain.example domain is not verified", "validation_error", "sender_domain_unverified", 403},
		{"validation_error", "You can only send testing emails to your own email address (private@example.com)", "validation_error", "testing_recipient_restricted", 403},
		{"validation_error", "API key is invalid", "validation_error", "invalid_api_key", 401},
		{"restricted_api_key", "private key data", "restricted_api_key", "api_key_permission_or_domain_restriction", 401},
		{"private@example.com", "private-secret-token", "unknown", "check_resend_dashboard", 422},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_ = json.NewEncoder(w).Encode(map[string]string{"name": tc.name, "message": tc.message})
		}))
		sender, _ := NewResend("test-key", "reset@example.com")
		sender.endpoint = server.URL
		_, err := sender.SendPasswordReset(context.Background(), "user@example.com", "https://app.example.com/#secret", "id")
		server.Close()
		var diagnostic *DeliveryError
		if !errors.As(err, &diagnostic) || diagnostic.StatusCode != tc.status || diagnostic.Code != tc.code || diagnostic.Reason != tc.reason {
			t.Fatalf("incorrect safe diagnostic: %v", err)
		}
		for _, secret := range []string{"private@example.com", "private-domain.example", "private-secret-token", "private key data"} {
			if strings.Contains(err.Error(), secret) {
				t.Fatal("provider data leaked through diagnostic")
			}
		}
	}
}
