package api

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
)

func NewRouter(
	twitterConnector *twitter.Connector,
	accounts *storage.SocialAccountRepository,
	encryptionKey string,
	signupService *service.SignupService,
	loginService *service.LoginService,
	jwtService *auth.JWTService,
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
	signupHandler := handler.NewSignupHandler(signupService)
	loginHandler := handler.NewLoginHandler(loginService)

	r.Post("/auth/signup", signupHandler.Signup)
	r.Post("/auth/login", loginHandler.Login)
	r.Post("/auth/logout", handler.Logout)

	// Protected Twitter connection routes
	authHandler := handler.NewAuthHandler(twitterConnector, accounts, encryptionKey)

	r.With(middleware.RequireAuth(jwtService)).Get("/auth/twitter", authHandler.TwitterAuth)

	r.With(middleware.RequireAuth(jwtService)).Get("/auth/twitter/callback", authHandler.TwitterCallback)

	return r
}
