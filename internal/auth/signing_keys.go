package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LoadSigningKeys derives the public key from one persisted PKCS#8 private key.
// Development may create the file once; production must provision it explicitly.
func LoadSigningKeys(path string, create bool) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if path == "" {
		return nil, nil, errors.New("JWT_SIGNING_KEY_FILE is required")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := GenerateSigningKeyFile(path); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, nil, err
		}
		file, err = os.Open(path)
	}
	if err != nil {
		return nil, nil, errors.New("cannot read JWT_SIGNING_KEY_FILE; provision a persistent Ed25519 PKCS#8 PEM key")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 {
		return nil, nil, errors.New("invalid JWT signing key file")
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, nil, errors.New("JWT signing key must contain exactly one PKCS#8 PRIVATE KEY PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, errors.New("invalid PKCS#8 JWT signing key")
	}
	private, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, nil, errors.New("JWT signing key must use Ed25519")
	}
	return private.Public().(ed25519.PublicKey), private, nil
}

// GenerateSigningKeyFile never replaces an existing key. Link publishes a fully
// written file atomically, so racing starts cannot read a partially written PEM.
func GenerateSigningKeyFile(path string) error {
	if path == "" {
		return errors.New("signing key output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create signing key directory: %w", err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return errors.New("generate signing key failed")
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return errors.New("encode signing key failed")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".jwt-key-*")
	if err != nil {
		return fmt.Errorf("create signing key file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = file.Chmod(0600); err != nil {
		return err
	}
	if err = pem.Encode(file, &pem.Block{Type: "PRIVATE KEY", Bytes: encoded}); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Link(file.Name(), path); err != nil {
		return fmt.Errorf("publish signing key without overwriting: %w", err)
	}
	return nil
}
