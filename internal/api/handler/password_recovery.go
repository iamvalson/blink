package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/storage"
)

const RecoveryAcknowledgement = "If an account exists for that email, a password reset link will be sent. Check your inbox and spam folder."

type PasswordRecoveryHandler struct {
	recovery       *service.PasswordRecoveryService
	cookieSecure   bool
	cookieSameSite http.SameSite
}

func NewPasswordRecoveryHandler(recovery *service.PasswordRecoveryService, secure bool, sameSite http.SameSite) *PasswordRecoveryHandler {
	return &PasswordRecoveryHandler{recovery, secure, sameSite}
}
func (h *PasswordRecoveryHandler) Forgot(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	err := h.recovery.Request(r.Context(), input.Email)
	if errors.Is(err, service.ErrInvalidRecoveryInput) {
		recoveryError(w, http.StatusBadRequest, "INVALID_EMAIL", "enter a valid email address")
		return
	}
	if err != nil {
		recoveryError(w, http.StatusServiceUnavailable, "RECOVERY_UNAVAILABLE", "password recovery temporarily unavailable; try again later")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": RecoveryAcknowledgement})
}
func (h *PasswordRecoveryHandler) Reset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	err := h.recovery.Reset(r.Context(), input.Token, input.Password)
	switch {
	case errors.Is(err, storage.ErrInvalidResetToken):
		recoveryError(w, http.StatusBadRequest, "RESET_TOKEN_INVALID", "this reset link is invalid or expired; request a new link")
		return
	case errors.Is(err, auth.ErrInvalidPassword):
		recoveryError(w, http.StatusBadRequest, "INVALID_PASSWORD", "password must contain 8 to 128 characters, uppercase, lowercase, a number and a symbol")
		return
	case err != nil:
		recoveryError(w, http.StatusServiceUnavailable, "RECOVERY_UNAVAILABLE", "password recovery temporarily unavailable; try again later")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "auth_token", Value: "", Path: "/", HttpOnly: true,
		Secure: h.cookieSecure, SameSite: h.cookieSameSite, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func recoveryError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "error": message})
}
