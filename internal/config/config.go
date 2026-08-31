package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct{
	// Server
	Port	int
	Env 	string

	// Database
	DatabaseURL		string

	// Redis
	RedisURL		string

	// Logging
	LogLevel		string

	// Encryption
	EncryptionKey	string
}

func Load() (*Config, error) {
	// Load .env file in development (ignore if not present)
	_ = godotenv.Load()

	return &Config{
		Port:			getEnvInt("PORT", 8000),
		Env:			getEnv("ENV", "development"),
		DatabaseURL:	getEnv("DATABASE_URL", "postgres://blink:devpass@localhost:5432/blink_db"),
		RedisURL:		getEnv("REDIS_URL", "redis://localhost:6379"),
		LogLevel:		getEnv("LOG_LEVEL", "info"),
		EncryptionKey: 	getEnv("ENCRYPTION_KEY", ""),
	}, nil
}

func getEnv(key, defaultVal string) string{
	if value := os.Getenv(key); value != "" {
		return value
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