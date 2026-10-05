package api

import (
	"fmt"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/iamvalson/blink/internal/api/handler"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/middleware"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter builds the HTTP router.
//
// oauthConnectors is a slice of any platform connectors that support the
// browser-redirect OAuth flow. For each connector with PlatformID "foo" the
// router automatically registers:
//
//	GET /auth/foo          → OAuth start (redirect to platform)
//	GET /auth/foo/callback → OAuth callback (exchange code, store token)
//
// Adding a new platform requires only adding its connector to this slice.
// No other file outside internal/connectors/<platform>/ needs to change.
func NewRouter(
	oauthConnectors []connectors.OAuthConnector,
	accounts *storage.SocialAccountRepository,
	posts *storage.PostRepository,
	encryptionKey string,
	signupService *service.SignupService,
	loginService *service.LoginService,
	meService *service.MeService,
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
	r.Handle("/metrics", promhttp.Handler())

	// Auth routes
	signupHandler := handler.NewSignupHandler(signupService)
	loginHandler := handler.NewLoginHandler(loginService)
	meHandler := handler.NewMeHandler(meService)

	r.Post("/auth/signup", signupHandler.Signup)
	r.Post("/auth/login", loginHandler.Login)
	r.Post("/auth/logout", handler.Logout)
	r.With(middleware.RequireAuth(jwtService)).Get("/auth/me", meHandler.Me)

	// Register one pair of OAuth routes per connected platform.
	// Each AuthHandler is platform-agnostic; the connector carries the identity.
	for _, connector := range oauthConnectors {
		authHandler := handler.NewAuthHandler(connector, accounts, encryptionKey)
		platformID := connector.PlatformID()

		r.With(middleware.RequireAuth(jwtService)).
			Get(fmt.Sprintf("/auth/%s", platformID), authHandler.OAuthStart)

		r.With(middleware.RequireAuth(jwtService)).
			Get(fmt.Sprintf("/auth/%s/callback", platformID), authHandler.OAuthCallback)
	}

	// Post routes
	postService := service.NewPostService(posts)
	postHandler := handler.NewPostHandler(postService)

	r.With(middleware.RequireAuth(jwtService)).Post("/api/v1/posts", postHandler.CreatePost)
	r.With(middleware.RequireAuth(jwtService)).Get("/api/v1/posts/{id}", postHandler.GetPost)
	r.With(middleware.RequireAuth(jwtService)).Post("/api/posts", postHandler.CreatePost)
	r.With(middleware.RequireAuth(jwtService)).Get("/api/posts/{id}", postHandler.GetPost)

	return r
}
