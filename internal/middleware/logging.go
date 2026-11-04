package middleware

import (
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

// RequestLogger records paths, never query strings, headers, or bodies.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// RequestURI would expose OAuth authorization codes and state.
		logger := log.With().Str("method", r.Method).Str("path", r.URL.EscapedPath()).
			Str("request_id", chiMiddleware.GetReqID(r.Context())).Logger()
		response := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		logger.Info().Msg("Request started")
		next.ServeHTTP(response, r)
		logger.Info().Int("status", response.Status()).Int("bytes", response.BytesWritten()).
			Dur("duration", time.Since(start)).Msg("Request completed")
	})
}
