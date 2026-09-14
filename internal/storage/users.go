package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserNotFound      = errors.New("user not found")
)

type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
}

type UserRepository struct {
	db userDB
}

type userDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewUserRepository(db userDB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(
	ctx context.Context,
	user User,
) (string, error) {
	const query = `
		INSERT INTO users(
			email,
			display_name,
			password_hash
		)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	var userID string

	err := r.db.QueryRow(ctx, query, user.Email, user.DisplayName, user.PasswordHash).Scan(&userID)

	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}

	return userID, nil
}

func (r *UserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*User, error) {
	const query = `
		SELECT id, email, display_name, password_hash FROM users WHERE email = $1
	`

	var user User

	err := r.db.QueryRow(ctx, query, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &user, nil
}
