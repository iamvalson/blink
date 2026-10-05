package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port int
	Env  string
	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// Logging
	LogLevel        string
	ShutdownTimeout time.Duration

	// Encryption
	EncryptionKey string

	// Platform Mode (real, mock)
	PlatformMode string

	// CORS Origins
	CORSOrigins []string
}

func Load() (*Config, error) {
	// Load .env file in development (ignore if not present)
	_ = godotenv.Load()

	return &Config{
		Port:            getEnvInt("PORT", 8000),
		Env:             getEnv("ENV", "development"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://blink:devpass@localhost:5432/blink_db"),
		RedisURL:        getEnv("REDIS_URL", "redis://localhost:6379"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		EncryptionKey:   getEnv("ENCRYPTION_KEY", ""),
		PlatformMode:    getEnv("PLATFORM_MODE", "real"),
		CORSOrigins:     getEnvStringSlice("CORS_ORIGINS", []string{"http://localhost:3000"}),
	}, nil
}

func getEnv(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
			return duration
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvStringSlice(key string, defaultVal []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultVal
	}
	parts := strings.Split(value, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return defaultVal
	}
	return result
}
