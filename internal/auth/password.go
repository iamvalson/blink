package auth

import (
	"errors"

	"github.com/alexedwards/argon2id"
)

const (
	minPasswordLength = 8
	maxPasswordLength = 128
)


var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong  = errors.New("password must not exceed 128 characters")
	ErrInvalidPassword  = errors.New("invalid password")
)



func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil{
		return "", err
	}

	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil{
		return "", err
	}

	return hash, nil
}


func CheckPassword(password, hash string) bool {
	if password == "" || hash == "" {
		return false
	}

	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil{
		return false
	}

	return match
}




func ValidatePassword(password string) error {
	length := len([]rune(password))

	switch {
	case length < minPasswordLength:
		return ErrPasswordTooShort
	case length > maxPasswordLength:
		return ErrPasswordTooLong
	}

	return nil
}