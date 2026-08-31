package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

func normalizeKey(key string) ([]byte, error) {
	if len(key) == 32 {
		return []byte(key), nil
	}

	if len(key) == 64 && strings.TrimSpace(key) != "" {
		decoded, err := hex.DecodeString(key)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}

	return nil, fmt.Errorf("encryption key must be 32 bytes or a 64-character hex string, got %d", len(key))
}

// EncryptToken encrypts a toke string using AES-256-GCM
func EncryptToken(token string, key string) (string, error) {
	keyBytes, err := normalizeKey(key)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(token), nil)
	return hex.EncodeToString(ciphertext), nil
}


func DecryptToken(encryted string, key string) (string, error){
	keyBytes, err := normalizeKey(key)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext, err := hex.DecodeString(encryted)
	if err != nil{
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}