package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/iamvalson/blink/internal/api/service"
)

type LoginHandler struct {
	login *service.LoginService
}

func NewLoginHandler(login *service.LoginService) *LoginHandler {
	return &LoginHandler{
		login: login,
	}
}

type loginRequest struct{
	Email		string	`json:"email"`
	Password	string	`json:"password"`
}

type loginResponse struct {
	UserID	string	`json:"user_id"`
}


func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil{
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.login.Login(
		r.Context(),
		service.LoginInput{
			Email: req.Email,
			Password: req.Password,
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
		Name: "auth_token",
		Value: result.AccessToken,
		Path: "/",
		HttpOnly: true,
		Secure: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge: 15 * 60,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(loginResponse{
		UserID: result.UserID,
	})
}