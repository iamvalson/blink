package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/email"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
)

var ErrRecoveryUnavailable = errors.New("password recovery temporarily unavailable")
var ErrInvalidRecoveryInput = errors.New("invalid recovery input")

const ResetLifetime = 30 * time.Minute

type RecoveryStore interface {
	Issue(context.Context, string, []byte, string, time.Time) error
	Valid(context.Context, []byte) (bool, error)
	Consume(context.Context, []byte, string) error
	Claim(context.Context) (*storage.ResetDelivery, error)
	Delivered(context.Context, string, string, int) error
	DeliveryFailed(context.Context, string, int, bool) error
	Cleanup(context.Context) error
}
type ResetEmailSender interface {
	SendPasswordReset(context.Context, string, string, string) (string, error)
}
type PasswordRecoveryService struct {
	store         RecoveryStore
	sender        ResetEmailSender
	encryptionKey string
	resetBase     string
}
type resetEmail struct {
	To  string `json:"to"`
	URL string `json:"url"`
}

func NewPasswordRecoveryService(store RecoveryStore, sender ResetEmailSender, key, frontendURL string, production bool) (*PasswordRecoveryService, error) {
	u, err := url.Parse(frontendURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && (production || u.Scheme != "http")) {
		return nil, errors.New("FRONTEND_URL must be a valid origin/base URL; production requires HTTPS")
	}
	if store == nil || sender == nil {
		return nil, ErrRecoveryUnavailable
	}
	if _, err := auth.EncryptToken("configuration-check", key); err != nil {
		return nil, errors.New("valid ENCRYPTION_KEY is required for password recovery")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/reset-password"
	u.RawPath = ""
	return &PasswordRecoveryService{store: store, sender: sender, encryptionKey: key, resetBase: u.String()}, nil
}

func (s *PasswordRecoveryService) Request(ctx context.Context, input string) error {
	if s == nil {
		return ErrRecoveryUnavailable
	}
	address := strings.ToLower(strings.TrimSpace(input))
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || len(address) > 255 {
		return ErrInvalidRecoveryInput
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return ErrRecoveryUnavailable
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	hash := sha256.Sum256([]byte(token))
	payload, err := json.Marshal(resetEmail{To: address, URL: s.resetBase + "#token=" + token})
	if err != nil {
		return ErrRecoveryUnavailable
	}
	encrypted, err := auth.EncryptToken(string(payload), s.encryptionKey)
	if err != nil {
		return ErrRecoveryUnavailable
	}
	if err := s.store.Issue(ctx, address, hash[:], encrypted, time.Now().Add(ResetLifetime)); err != nil {
		return ErrRecoveryUnavailable
	}
	return nil
}

func (s *PasswordRecoveryService) Reset(ctx context.Context, token, password string) error {
	if s == nil {
		return ErrRecoveryUnavailable
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if len(token) != 43 || err != nil || len(decoded) != 32 {
		return storage.ErrInvalidResetToken
	}
	if err := auth.ValidateNewPassword(password); err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(token))
	valid, err := s.store.Valid(ctx, hash[:])
	if err != nil {
		return ErrRecoveryUnavailable
	}
	if !valid {
		return storage.ErrInvalidResetToken
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return ErrRecoveryUnavailable
	}
	err = s.store.Consume(ctx, hash[:], passwordHash)
	if errors.Is(err, storage.ErrInvalidResetToken) {
		return storage.ErrInvalidResetToken
	}
	if err != nil {
		return ErrRecoveryUnavailable
	}
	return nil
}

// DeliverNext resumes leased work after crashes and uses a stable Resend
// idempotency key if delivery succeeded but saving the provider ID failed.
func (s *PasswordRecoveryService) DeliverNext(ctx context.Context) (bool, error) {
	job, err := s.store.Claim(ctx)
	if err != nil || job == nil {
		return false, err
	}
	raw, err := auth.DecryptToken(job.EncryptedEmail, s.encryptionKey)
	var message resetEmail
	if err != nil || json.Unmarshal([]byte(raw), &message) != nil || message.To == "" || message.URL == "" {
		return true, s.store.DeliveryFailed(ctx, job.ID, job.Attempt, true)
	}
	id, sendErr := s.sender.SendPasswordReset(ctx, message.To, message.URL, job.ID)
	if sendErr != nil {
		event := log.Warn().Bool("permanent", errors.Is(sendErr, email.ErrPermanent))
		var providerError *email.DeliveryError
		if errors.As(sendErr, &providerError) {
			event = event.Int("provider_status", providerError.StatusCode).
				Str("provider_code", providerError.Code).Str("reason", providerError.Reason)
		}
		event.Msg("Password reset email delivery failed")
		return true, s.store.DeliveryFailed(ctx, job.ID, job.Attempt, errors.Is(sendErr, email.ErrPermanent))
	}
	return true, s.store.Delivered(ctx, job.ID, id, job.Attempt)
}

func (s *PasswordRecoveryService) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := s.store.Cleanup(cleanupCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Error().Msg("Password recovery cleanup failed")
			}
			for i := 0; i < 5 && ctx.Err() == nil; i++ {
				operationCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				worked, err := s.DeliverNext(operationCtx)
				cancel()
				if err != nil && ctx.Err() == nil {
					log.Error().Msg("Password recovery delivery persistence failed")
				}
				if err != nil || !worked {
					break
				}
			}
		}
	}
}
