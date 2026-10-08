package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"unicode"

	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
)

type SignupHandler struct {
	signup         *service.SignupService
	secureCookie   bool
	cookieSecure   bool
	cookieSameSite http.SameSite
}

func NewSignupHandler(signup *service.SignupService, secureCookie, cookieSecure bool, cookieSameSite http.SameSite) *SignupHandler {
	return &SignupHandler{
		signup:         signup,
		secureCookie:   secureCookie,
		cookieSecure:   cookieSecure,
		cookieSameSite: cookieSameSite,
	}
}

type signupRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type signupResponse struct {
	UserID string `json:"user_id"`
}

func validPassword(password string) bool {
	if len([]rune(password)) < 8 {
		return false
	}
	if len([]rune(password)) > 128 {
		return false
	}

	var hasUpper bool
	var hasLower bool
	var hasNumber bool
	var hasSymbol bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSymbol = true
		}
	}

	return hasUpper && hasLower && hasNumber && hasSymbol
}

func (h *SignupHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if !validPassword(req.Password) {
		http.Error(
			w,
			"password must be at least 8 characters and contain at least one uppercase letter, one lowercase letter, one number, and one symbol",
			http.StatusBadRequest,
		)
		return
	}

	result, err := h.signup.Signup(
		r.Context(),
		service.SignupInput{
			Email:       req.Email,
			DisplayName: req.DisplayName,
			Password:    req.Password,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailAlreadyExists):
			http.Error(w, "email already exists", http.StatusConflict)

		case errors.Is(err, auth.ErrInvalidPassword):
			http.Error(w, err.Error(), http.StatusBadRequest)

		default:
			http.Error(w, "failed to create account", http.StatusInternalServerError)
		}

		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    result.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: h.cookieSameSite,
		MaxAge:   int(auth.AccessTokenLifetime(false).Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(signupResponse{
		UserID: result.UserID,
	})
}
