package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)


const (
	accessTokenTTL = 15 * time.Minute
	issuer = "blink"
	audience = "blink-api"
)


type JWTService struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}


func NewJWTService(
	privateKey ed25519.PrivateKey,
	publicKey ed25519.PublicKey,
) *JWTService {
	return &JWTService{
		privateKey: privateKey,
		publicKey: publicKey,
	}
}


type Claims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

func (s *JWTService) CreateAccessToken(userID string) (string, error) {
	now := time.Now()

	jti, err := generateJTI()
	if err != nil{
		return "", fmt.Errorf("generate token ID: %w", err)
	}

	claims := Claims{
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID,
			Issuer: issuer,
			Audience: jwt.ClaimStrings{audience},
			IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
			ID:	jti,
		},
	}


	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	signedToken, err := token.SignedString(s.privateKey)
	if err != nil{
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return signedToken, nil
}


func (s *JWTService) ValidateAccessToken(tokenString string) (string, error) {
	var claims Claims

	token, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
				return nil,  fmt.Errorf("unexpected signing algorithm: %s", t.Method.Alg())
			}
			return s.publicKey, nil
		},
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
	)
	if err != nil {
		return "", fmt.Errorf("validate access token: %w", err)
	}

	if !token.Valid {
		return "", fmt.Errorf("invalid access token")
	}

	if claims.TokenType != "access" {
		return "", fmt.Errorf("invalid token type")
	}

	if claims.Subject == "" {
		return "", fmt.Errorf("token subject is missing")
	}

	return claims.Subject, nil
}



func generateJTI() (string, error) {
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil{
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}