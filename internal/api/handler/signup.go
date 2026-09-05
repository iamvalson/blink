package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
)


type SignupHandler struct {
	signup *service.SignupService
}


func NewSignupHandler(signup *service.SignupService) *SignupHandler{
	return &SignupHandler{
		signup: signup,
	}
}


type signupRequest struct {
	Email		string	`json:"email"`
	DisplayName	string	`json:"display_name"`
	Password	string	`json:"password"`
}

type signupResponse struct {
	UserID	string	`json:"user_id"`
}


func (h *SignupHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil{
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.signup.Signup(
		r.Context(),
		service.SignupInput{
			Email: req.Email,
			DisplayName: req.DisplayName,
			Password: req.Password,
		},
	)
	if err != nil{
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

	// Set JWT as an HTTPOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name: "auth_token",
		Value: result.AccessToken,
		Path: "/",
		HttpOnly: true,
		Secure: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge: 15 * 60,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(signupResponse{
		UserID: result.UserID,
	})
}