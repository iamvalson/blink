package middleware

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/iamvalson/blink/internal/auth"
)

func newMiddlewareJWTService(t *testing.T) (*auth.JWTService, ed25519.PrivateKey) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate JWT keys: %v", err)
	}

	return auth.NewJWTService(privateKey, publicKey), privateKey
}

func runProtectedRequest(jwtService *auth.JWTService, token string) *httptest.ResponseRecorder {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "missing user ID", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(userID))
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if token != "" {
		request.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	}

	recorder := httptest.NewRecorder()
	RequireAuth(jwtService)(next).ServeHTTP(recorder, request)
	return recorder
}

func TestRequireAuthRejectsMissingJWT(t *testing.T) {
	jwtService, _ := newMiddlewareJWTService(t)

	response := runProtectedRequest(jwtService, "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRequireAuthRejectsInvalidJWT(t *testing.T) {
	jwtService, _ := newMiddlewareJWTService(t)

	response := runProtectedRequest(jwtService, "not-a-token")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRequireAuthRejectsExpiredJWT(t *testing.T) {
	jwtService, privateKey := newMiddlewareJWTService(t)

	claims := auth.Claims{
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "blink",
			Audience:  jwt.ClaimStrings{"blink-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign expired JWT: %v", err)
	}

	response := runProtectedRequest(jwtService, token)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRequireAuthAddsRealUserIDToContext(t *testing.T) {
	jwtService, _ := newMiddlewareJWTService(t)

	token, err := jwtService.CreateAccessToken("user-123")
	if err != nil {
		t.Fatalf("failed to create access token: %v", err)
	}

	response := runProtectedRequest(jwtService, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Body.String() != "user-123" {
		t.Fatalf("expected user-123 in request context, got %q", response.Body.String())
	}
}
