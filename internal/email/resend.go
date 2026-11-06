package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("RESEND_API_KEY and EMAIL_FROM are required")
var ErrPermanent = errors.New("email rejected by provider")

type Resend struct {
	key, from, endpoint string
	client              *http.Client
}

func NewResend(key, from string) (*Resend, error) {
	if key == "" && from == "" {
		return nil, ErrNotConfigured
	}
	address, err := mail.ParseAddress(from)
	if strings.TrimSpace(key) == "" || err != nil || address.Address == "" || strings.ContainsAny(from, "\r\n") {
		return nil, ErrNotConfigured
	}
	return &Resend{key: key, from: from, endpoint: "https://api.resend.com/emails",
		client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}}, nil
}

func (s *Resend) SendPasswordReset(ctx context.Context, to, resetURL, requestID string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"from": s.from, "to": []string{to}, "subject": "Reset your Blink password",
		"text": "A password reset was requested for your Blink account.\n\n" +
			resetURL + "\n\nThis link expires in 30 minutes and can be used once. " +
			"If you did not request this, you can ignore this email.",
		"html": "<h1>Reset your Blink password</h1><p>A password reset was requested for your Blink account.</p>" +
			"<p><a href=\"" + html.EscapeString(resetURL) + "\">Reset password</a></p>" +
			"<p>This link expires in 30 minutes and can be used once.</p>" +
			"<p>If you did not request this, you can ignore this email.</p>",
	})
	if err != nil {
		return "", errors.New("encode reset email failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("create reset email request failed")
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "password-reset/"+requestID)
	response, err := s.client.Do(req)
	if err != nil {
		return "", errors.New("reset email transport failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return "", resendDeliveryError(response)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&result); err != nil || result.ID == "" {
		return "", errors.New("invalid reset email provider response")
	}
	return result.ID, nil
}

// DeliveryError exposes only allowlisted diagnostics, never the provider message.
type DeliveryError struct {
	StatusCode int
	Code       string
	Reason     string
	Permanent  bool
}

func (e *DeliveryError) Error() string {
	return fmt.Sprintf("Resend HTTP %d (%s): %s", e.StatusCode, e.Code, e.Reason)
}

func (e *DeliveryError) Unwrap() error {
	if e.Permanent {
		return ErrPermanent
	}
	return nil
}

func resendDeliveryError(response *http.Response) *DeliveryError {
	var body struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	// Read for classification only. Never return or log these original strings.
	_ = json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&body)
	code := "unknown"
	switch body.Name {
	case "validation_error", "missing_api_key", "restricted_api_key", "suspended_api_key",
		"invalid_permission", "invalid_idempotency_key", "invalid_idempotent_request",
		"concurrent_idempotent_requests", "invalid_parameter", "missing_required_field",
		"daily_quota_exceeded", "monthly_quota_exceeded", "rate_limit_exceeded",
		"application_error", "service_unavailable":
		code = body.Name
	}
	reason := "check_resend_dashboard"
	message := strings.ToLower(body.Message)
	switch {
	case strings.Contains(message, "only send testing emails"):
		reason = "testing_recipient_restricted"
	case strings.Contains(message, "domain") && strings.Contains(message, "not verified"):
		reason = "sender_domain_unverified"
	case strings.Contains(message, "api key is invalid"):
		reason = "invalid_api_key"
	case code == "missing_api_key":
		reason = "missing_api_key"
	case code == "restricted_api_key" || code == "invalid_permission":
		reason = "api_key_permission_or_domain_restriction"
	case code == "suspended_api_key":
		reason = "api_key_suspended"
	}
	permanent := response.StatusCode >= 400 && response.StatusCode < 500 &&
		response.StatusCode != 429 && response.StatusCode != 409
	if code == "invalid_idempotent_request" {
		permanent = true
	}
	return &DeliveryError{StatusCode: response.StatusCode, Code: code, Reason: reason, Permanent: permanent}
}
