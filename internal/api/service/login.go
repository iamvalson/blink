package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/storage"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type LoginService struct {
	users *storage.UserRepository
	jwt   *auth.JWTService
}

func NewLoginService(users *storage.UserRepository, jwt *auth.JWTService) *LoginService {
	return &LoginService{
		users: users,
		jwt:   jwt,
	}
}

type LoginInput struct {
	Email    string
	Password string
}

type LoginResult struct {
	UserID      string
	AccessToken string
}

func (l *LoginService) Login(
	ctx context.Context,
	input LoginInput,
) (*LoginResult, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))

	if email == "" {
		return nil, errors.New("email is required")
	}

	if input.Password == "" {
		return nil, errors.New("password is required")
	}

	// Get user
	user, err := l.users.GetByEmail(ctx, email)

	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}

		return nil, fmt.Errorf("get user by email: %w", err)
	}

	// Check Password
	if !auth.CheckPassword(input.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Generate JWT
	accessToken, err := l.jwt.CreateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}

	return &LoginResult{
		UserID:      user.ID,
		AccessToken: accessToken,
	}, nil
}
