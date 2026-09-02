package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

// ErrorHandler catches panics
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func ()  {
			if err := recover(); err != nil {
				log.Error().Interface("panic", err).Msg("Request panic")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				if err := json.NewEncoder(w).Encode(map[string]string{
					"error": "Internal server error",
				}); err != nil {
					log.Error().Err(err).Msg("Failed to encode panic response")
				}
			}
		} ()
		next.ServeHTTP(w, r)
	})
}