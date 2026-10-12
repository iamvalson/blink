package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/iamvalson/blink/internal/api/service"
)

type LoginHandler struct {
	login          *service.LoginService
	secureCookie   bool
	cookieSecure   bool
	cookieSameSite http.SameSite
}

func NewLoginHandler(login *service.LoginService, secureCookie, cookieSecure bool, cookieSameSite http.SameSite) *LoginHandler {
	return &LoginHandler{
		login:          login,
		secureCookie:   secureCookie,
		cookieSecure:   cookieSecure,
		cookieSameSite: cookieSameSite,
	}
}

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me"`
}

type loginResponse struct {
	UserID string `json:"user_id"`
}

func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.login.Login(
		r.Context(),
		service.LoginInput{
			Email:      req.Email,
			Password:   req.Password,
			RememberMe: req.RememberMe,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
		default:
			http.Error(w, "failed to login", http.StatusInternalServerError)
		}
		return
	}

	// Set JWT as an HTTPOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    result.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: h.cookieSameSite,
		MaxAge:   int(result.SessionDuration.Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(loginResponse{
		UserID: result.UserID,
	})
}
