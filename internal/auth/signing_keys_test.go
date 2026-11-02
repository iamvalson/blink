package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSigningKeysPersistAcrossAPIInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets", "jwt.pem")
	public, private, err := LoadSigningKeys(path, true)
	if err != nil {
		t.Fatal(err)
	}
	token, err := NewJWTService(private, public).CreateAccessToken("user-123", true)
	if err != nil {
		t.Fatal(err)
	}
	public2, private2, err := LoadSigningKeys(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(private, private2) {
		t.Fatal("key changed on restart")
	}
	if user, err := NewJWTService(private2, public2).ValidateAccessToken(token); err != nil || user != "user-123" {
		t.Fatalf("session invalidated by restart: %s %v", user, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("generated key is not private")
	}
	before, _ := os.ReadFile(path)
	if GenerateSigningKeyFile(path) == nil {
		t.Fatal("overwrote existing key")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("key replaced")
	}
}
func TestSigningKeyCreationRaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jwt.pem")
	var wg sync.WaitGroup
	keys := make(chan []byte, 8)
	for range 8 {
		wg.Go(func() {
			_, private, err := LoadSigningKeys(path, true)
			if err != nil {
				t.Error(err)
				return
			}
			keys <- private
		})
	}
	wg.Wait()
	close(keys)
	var first []byte
	for key := range keys {
		if first == nil {
			first = key
		}
		if !bytes.Equal(first, key) {
			t.Fatal("racing instances used different keys")
		}
	}
}
func TestSigningKeyErrorsFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jwt.pem")
	if _, _, err := LoadSigningKeys(path, false); err == nil {
		t.Fatal("production generated a missing key")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("production wrote a key")
	}
	if err := os.WriteFile(path, []byte("invalid-secret-material"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadSigningKeys(path, true); err == nil {
		t.Fatal("invalid key silently replaced")
	}
}
