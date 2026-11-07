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
	ID             string
	Email          string
	DisplayName    string
	PasswordHash   string
	SessionVersion int64
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
	legalVersion string,
) (string, error) {
	const query = `
		INSERT INTO users(
			email,
			display_name,
			password_hash,
			legal_version,
			legal_accepted_at
		)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		RETURNING id
	`

	var userID string

	err := r.db.QueryRow(ctx, query, user.Email, user.DisplayName, user.PasswordHash, legalVersion).Scan(&userID)

	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}

	return userID, nil
}

func (r *UserRepository) GetByID(
	ctx context.Context,
	id string,
) (*User, error) {
	const query = `
		SELECT id, email, display_name FROM users WHERE id = $1
	`

	var user User

	err := r.db.QueryRow(ctx, query, id).Scan(&user.ID, &user.Email, &user.DisplayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*User, error) {
	const query = `
		SELECT id, email, display_name, password_hash, session_version FROM users WHERE email = $1
	`

	var user User

	err := r.db.QueryRow(ctx, query, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.SessionVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) SessionVersion(ctx context.Context, id string) (int64, error) {
	var version int64
	err := r.db.QueryRow(ctx, "SELECT session_version FROM users WHERE id=$1", id).Scan(&version)
	return version, err
}
