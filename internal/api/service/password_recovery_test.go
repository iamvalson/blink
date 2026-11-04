package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/auth"
)

type requestRecoveryStore struct {
	RecoveryStore
	hash    []byte
	payload string
	expires time.Time
}

func (s *requestRecoveryStore) Issue(_ context.Context, email string, hash []byte, payload string, expires time.Time) error {
	if email != "user@example.com" {
		return errors.New("unexpected email")
	}
	s.hash = append([]byte(nil), hash...)
	s.payload = payload
	s.expires = expires
	return nil
}

type unusedResetSender struct{}

func (unusedResetSender) SendPasswordReset(context.Context, string, string, string) (string, error) {
	return "", errors.New("must not send synchronously")
}

func TestRecoveryConfigurationAndEncryptedRequest(t *testing.T) {
	store := &requestRecoveryStore{}
	key := "0123456789abcdef0123456789abcdef"
	for _, frontend := range []string{"http://app.example.com", "https://user:password@app.example.com", "https://app.example.com?redirect=evil", "https://app.example.com#secret", "//app.example.com"} {
		if _, err := NewPasswordRecoveryService(store, unusedResetSender{}, key, frontend, true); err == nil {
			t.Fatalf("accepted unsafe frontend URL %q", frontend)
		}
	}
	if _, err := NewPasswordRecoveryService(store, unusedResetSender{}, "bad-key", "https://app.example.com", true); err == nil {
		t.Fatal("invalid encryption key accepted")
	}
	service, err := NewPasswordRecoveryService(store, unusedResetSender{}, key, "https://app.example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Request(context.Background(), " USER@EXAMPLE.COM "); err != nil {
		t.Fatal(err)
	}
	raw, err := auth.DecryptToken(store.payload, key)
	if err != nil {
		t.Fatal(err)
	}
	var message resetEmail
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(message.URL)
	if err != nil {
		t.Fatal(err)
	}
	values, err := url.ParseQuery(u.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	token := values.Get("token")
	hash := sha256.Sum256([]byte(token))
	if string(hash[:]) != string(store.hash) || len(token) != 43 || strings.Contains(store.payload, token) || strings.Contains(store.payload, message.To) {
		t.Fatal("plaintext reset data stored or incorrect verifier")
	}
	if remaining := time.Until(store.expires); remaining <= 29*time.Minute || remaining > ResetLifetime {
		t.Fatal("incorrect token lifetime")
	}
	for _, input := range []string{"", "bad-email", "Name <user@example.com>", "user@example.com\r\nBcc: other@example.com"} {
		if err := service.Request(context.Background(), input); !errors.Is(err, ErrInvalidRecoveryInput) {
			t.Fatal("invalid email accepted")
		}
	}
	var disabled *PasswordRecoveryService
	if !errors.Is(disabled.Request(context.Background(), "user@example.com"), ErrRecoveryUnavailable) {
		t.Fatal("disabled recovery claimed success")
	}
}
