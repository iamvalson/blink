package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestCreateAccessTokenUsesRememberMeLifetime(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate JWT keys: %v", err)
	}

	service := NewJWTService(privateKey, publicKey)
	for _, test := range []struct {
		name       string
		rememberMe bool
	}{
		{name: "session", rememberMe: false},
		{name: "remembered", rememberMe: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokenString, err := service.CreateAccessToken("user-123", test.rememberMe)
			if err != nil {
				t.Fatalf("CreateAccessToken failed: %v", err)
			}

			var claims Claims
			_, _, err = jwt.NewParser().ParseUnverified(tokenString, &claims)
			if err != nil {
				t.Fatalf("failed to parse token: %v", err)
			}

			expected := AccessTokenLifetime(test.rememberMe)
			actual := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
			if actual != expected {
				t.Fatalf("expected token lifetime %s, got %s", expected, actual)
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // 64 hex chars = 32 bytes

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "simple_token",
			token: "my_secret_token_123",
		},
		{
			name:  "long token",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
		},
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "special chars",
			token: "token!@#$%^&*()_+-=[]{}|;:,.<>?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := EncryptToken(tt.token, key)
			if err != nil {
				t.Fatalf("EncryptToken failed %v", err)
			}

			// Encrypted should not be the same as the original
			if encrypted == tt.token {
				t.Fatalf("Encrypted token should differ from original")
			}

			// Decrypt
			decrypted, err := DecryptToken(encrypted, key)
			if err != nil {
				t.Fatalf("DecryptToken failed %v", err)
			}

			// Should match original
			if decrypted != tt.token {
				t.Fatalf("Decrypted token %q does not match original %q", decrypted, tt.token)
			}
		})
	}
}

func TestDecryptInvalidKey(t *testing.T) {
	key1 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key2 := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	token := "my_secret_token"

	// Encrypt with key 1
	encrypted, err := EncryptToken(token, key1)
	if err != nil {
		t.Fatalf("EncryptToken failed %v", err)
	}

	// Decrypt with key 2
	_, err = DecryptToken(encrypted, key2)
	if err == nil {
		t.Fatalf("DecryptToken should fail with wrong key")
	}
}

func TestEncryptInvalidKeyLength(t *testing.T) {
	shortKey := "tooshort"
	token := "test_token"

	_, err := EncryptToken(token, shortKey)
	if err == nil {
		t.Fatalf("EncryptToken should fail with invalid key length")
	}
}

func TestDecryptInvalidKeyLength(t *testing.T) {
	shortKey := "tooshort"
	encrypted := "someciphertext"

	_, err := DecryptToken(encrypted, shortKey)
	if err == nil {
		t.Fatalf("DecryptToken should fail with invalid key length")
	}
}
