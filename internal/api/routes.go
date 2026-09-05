package api

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
)



func NewRouter(
	twitterConnector *twitter.Connector,
	accounts *storage.SocialAccountRepository,
	encryptionKey string,
) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	
	r.Use(middleware.ErrorHandler)
	r.Use(middleware.RequestLogger)


	// Public routes
	r.Get("/health", handler.HealthHandler)


	// Auth routes
	authHandler := handler.NewAuthHandler(twitterConnector, accounts, encryptionKey)
	r.With(middleware.RequireAuth).Get("/auth/twitter", authHandler.TwitterAuth)
	r.With(middleware.RequireAuth).Get("/auth/twitter/callback", authHandler.TwitterCallback)


	return r
}