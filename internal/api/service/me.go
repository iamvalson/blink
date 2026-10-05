package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamvalson/blink/internal/storage"
)

// ErrUserNotFound is returned when the authenticated user cannot be found in
// the database (e.g. account deleted between token issuance and this call).
var ErrUserNotFound = errors.New("user not found")

// MeService resolves the currently authenticated user's profile.
type MeService struct {
	users *storage.UserRepository
}

func NewMeService(users *storage.UserRepository) *MeService {
	return &MeService{users: users}
}

// MeResult carries the public fields of the authenticated user's profile.
type MeResult struct {
	ID          string
	Email       string
	DisplayName string
}

// Me fetches the authenticated user's profile by their ID extracted from the
// JWT. It deliberately does not return the password hash.
func (s *MeService) Me(ctx context.Context, userID string) (*MeResult, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	return &MeResult{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}, nil
}
