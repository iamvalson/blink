package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadShutdownTimeout(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "7s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ShutdownTimeout != 7*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 7s", cfg.ShutdownTimeout)
	}
}

func TestLoadShutdownTimeoutDefaultsForInvalidValue(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 30s", cfg.ShutdownTimeout)
	}
}

func TestLoadShutdownTimeoutDefault(t *testing.T) {
	if err := os.Unsetenv("SHUTDOWN_TIMEOUT"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 30s", cfg.ShutdownTimeout)
	}
}

func TestLoadSecureCookieIsEnabledOnlyInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("R2_ACCOUNT_ID", "test-account")
	t.Setenv("R2_ACCESS_KEY_ID", "test-key")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-secret")

	production, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !production.SecureCookie {
		t.Fatal("SecureCookie = false in production, want true")
	}

	t.Setenv("ENV", "development")
	development, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if development.SecureCookie {
		t.Fatal("SecureCookie = true in development, want false")
	}
}

func TestLoadR2ConfigValidationInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("MEDIA_STORAGE_DRIVER", "r2")

	// Missing account ID and endpoint
	_ = os.Unsetenv("R2_ACCOUNT_ID")
	_ = os.Unsetenv("R2_ENDPOINT")
	_ = os.Unsetenv("R2_ACCESS_KEY_ID")
	_ = os.Unsetenv("R2_SECRET_ACCESS_KEY")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error in production when R2 credentials are missing, got nil")
	}

	// Missing secret key
	t.Setenv("R2_ACCOUNT_ID", "acc-123")
	t.Setenv("R2_ACCESS_KEY_ID", "key-123")
	_ = os.Unsetenv("R2_SECRET_ACCESS_KEY")

	_, err = Load()
	if err == nil {
		t.Fatal("expected error when R2_SECRET_ACCESS_KEY is missing, got nil")
	}

	// Complete R2 configuration in production
	t.Setenv("R2_SECRET_ACCESS_KEY", "secret-123")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error with valid R2 config: %v", err)
	}
	if cfg.R2Endpoint != "https://acc-123.r2.cloudflarestorage.com" {
		t.Fatalf("expected endpoint https://acc-123.r2.cloudflarestorage.com, got %s", cfg.R2Endpoint)
	}
	if cfg.R2BucketName != "blink-media" {
		t.Fatalf("expected default bucket blink-media, got %s", cfg.R2BucketName)
	}
}

func TestLoadR2ConfigCustomEndpoint(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("MEDIA_STORAGE_DRIVER", "r2")
	t.Setenv("R2_ENDPOINT", "https://custom.endpoint.com")
	t.Setenv("R2_ACCESS_KEY_ID", "key")
	t.Setenv("R2_SECRET_ACCESS_KEY", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.R2Endpoint != "https://custom.endpoint.com" {
		t.Fatalf("expected custom endpoint, got %s", cfg.R2Endpoint)
	}
}

func TestLoadProductionRejectsLocalDriver(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("MEDIA_STORAGE_DRIVER", "local")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production to reject local storage driver, got nil")
	}
}
