package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql/driver"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/storage"
)

type hashedPasswordArgument struct {
	password string
}

func (argument hashedPasswordArgument) Match(value driver.Value) bool {
	hash, ok := value.(string)
	return ok && hash != argument.password && auth.CheckPassword(argument.password, hash)
}

func newServiceTestDependencies(t *testing.T) (*storage.UserRepository, *auth.JWTService, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		db.Close()
		t.Fatalf("failed to generate JWT keys: %v", err)
	}

	jwtService := auth.NewJWTService(privateKey, publicKey)
	users := storage.NewUserRepository(db)
	cleanup := func() {
		db.Close()
	}

	return users, jwtService, mock, cleanup
}

func TestSignupCreatesUserWithHashedPassword(t *testing.T) {
	users, jwtService, mock, cleanup := newServiceTestDependencies(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, display_name, password_hash FROM users WHERE email = $1")).
		WithArgs("person@example.com").
		WillReturnError(storage.ErrUserNotFound)
	mock.ExpectQuery("INSERT INTO users").
		WithArgs("person@example.com", "Person", hashedPasswordArgument{password: "strong-password"}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("user-123"))

	service := NewSignupService(users, jwtService)
	result, err := service.Signup(context.Background(), SignupInput{
		Email:       " PERSON@EXAMPLE.COM ",
		DisplayName: " Person ",
		Password:    "strong-password",
	})
	if err != nil {
		t.Fatalf("Signup failed: %v", err)
	}

	if result.UserID != "user-123" {
		t.Fatalf("expected user-123, got %s", result.UserID)
	}

	userID, err := jwtService.ValidateAccessToken(result.AccessToken)
	if err != nil {
		t.Fatalf("created access token is invalid: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("expected JWT subject user-123, got %s", userID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations were not met: %v", err)
	}
}

func TestSignupRejectsDuplicateEmail(t *testing.T) {
	users, jwtService, mock, cleanup := newServiceTestDependencies(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, display_name, password_hash FROM users WHERE email = $1")).
		WithArgs("person@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "display_name", "password_hash"}).AddRow(
			"user-123", "person@example.com", "Person", "existing-hash",
		))

	service := NewSignupService(users, jwtService)
	_, err := service.Signup(context.Background(), SignupInput{
		Email:       "person@example.com",
		DisplayName: "Person",
		Password:    "strong-password",
	})
	if err != ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations were not met: %v", err)
	}
}

func TestLoginSucceedsWithValidCredentials(t *testing.T) {
	users, jwtService, mock, cleanup := newServiceTestDependencies(t)
	defer cleanup()

	passwordHash, err := auth.HashPassword("strong-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, display_name, password_hash FROM users WHERE email = $1")).
		WithArgs("person@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "display_name", "password_hash"}).AddRow(
			"user-123", "person@example.com", "Person", passwordHash,
		))

	service := NewLoginService(users, jwtService)
	result, err := service.Login(context.Background(), LoginInput{
		Email:    " PERSON@EXAMPLE.COM ",
		Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if result.UserID != "user-123" {
		t.Fatalf("expected user-123, got %s", result.UserID)
	}

	userID, err := jwtService.ValidateAccessToken(result.AccessToken)
	if err != nil {
		t.Fatalf("login access token is invalid: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("expected JWT subject user-123, got %s", userID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations were not met: %v", err)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	users, jwtService, mock, cleanup := newServiceTestDependencies(t)
	defer cleanup()

	passwordHash, err := auth.HashPassword("strong-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, display_name, password_hash FROM users WHERE email = $1")).
		WithArgs("person@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "display_name", "password_hash"}).AddRow(
			"user-123", "person@example.com", "Person", passwordHash,
		))

	service := NewLoginService(users, jwtService)
	_, err = service.Login(context.Background(), LoginInput{
		Email:    "person@example.com",
		Password: "wrong-password",
	})
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations were not met: %v", err)
	}
}
