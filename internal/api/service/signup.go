package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/storage"
)

// SignupLegalVersion identifies the terms and privacy notice shown together.
const SignupLegalVersion = "1.0"

var (
	ErrEmailAlreadyExists      = errors.New("email already exists")
	ErrLegalAcceptanceRequired = errors.New("accept the current Terms of Service and acknowledge the Privacy Policy to create an account")
)

type SignupService struct {
	users *storage.UserRepository
	jwt   *auth.JWTService
}

func NewSignupService(
	users *storage.UserRepository,
	jwt *auth.JWTService,
) *SignupService {
	return &SignupService{
		users: users,
		jwt:   jwt,
	}
}

type SignupInput struct {
	Email        string
	DisplayName  string
	Password     string
	AcceptTerms  bool
	LegalVersion string
}

type SignupResult struct {
	UserID      string
	AccessToken string
}

func (s *SignupService) Signup(
	ctx context.Context,
	input SignupInput,
) (*SignupResult, error) {
	if !input.AcceptTerms || input.LegalVersion != SignupLegalVersion {
		return nil, ErrLegalAcceptanceRequired
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	displayName := strings.TrimSpace(input.DisplayName)

	if err := auth.ValidatePassword(input.Password); err != nil {
		return nil, fmt.Errorf("validate password: %w", err)
	}

	if email == "" {
		return nil, errors.New("email is required")
	}

	if displayName == "" {
		return nil, errors.New("username is required")
	}

	// Check whether the user already exists
	_, err := s.users.GetByEmail(ctx, email)
	switch {
	case err == nil:
		return nil, ErrEmailAlreadyExists
	case !errors.Is(err, storage.ErrUserNotFound):
		return nil, fmt.Errorf("check existing user: %w", err)
	}

	// Hash password
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user
	user := storage.User{
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
	}

	userID, err := s.users.Create(ctx, user, input.LegalVersion)
	if err != nil {
		if errors.Is(err, storage.ErrUserAlreadyExists) {
			return nil, ErrEmailAlreadyExists
		}

		return nil, fmt.Errorf("create user: %w", err)
	}

	accessToken, err := s.jwt.CreateAccessToken(userID, false)
	if err != nil {
		return nil, fmt.Errorf("create access token: %w", err)
	}

	return &SignupResult{
		UserID:      userID,
		AccessToken: accessToken,
	}, nil

}
