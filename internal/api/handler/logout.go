package handler

import (
	"net/http"
)

func NewLogoutHandler(cookieSecure bool, cookieSameSite http.SameSite) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: cookieSameSite,
			MaxAge:   -1,
		})

		w.WriteHeader(http.StatusNoContent)
	}
}
