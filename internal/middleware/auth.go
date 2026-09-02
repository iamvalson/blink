package middleware

import (
	"context"
	"net/http"
)

// RequireAuth checks if user is authenticated
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Verify JWT token from cookie or Authorization header
		// TODO: Extract user ID and add to context

		userID := "user_123"

		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}