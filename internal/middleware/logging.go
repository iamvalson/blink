package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// RequestLogger logs request details
func RequestLogger(next http.Handler) http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		log.Info().Str("method", r.Method).Str("path", r.RequestURI).Str("remote_addr", r.RemoteAddr).Msg("Request started")

		next.ServeHTTP(w, r)

		duration := time.Since(start)
		log.Info().Str("method", r.Method).Str("path", r.RequestURI).Dur("duration", duration).Msg("Request completed")
	})
}