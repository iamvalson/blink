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
