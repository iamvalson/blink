package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port           int
	Env            string
	SecureCookie   bool
	CookieSecure   bool
	CookieSameSite http.SameSite
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

	// Frontend config
	FrontendURL string

	// Media Storage
	MediaStorageDriver          string
	MediaStoragePath            string
	R2AccountID                 string
	R2AccessKeyID               string
	R2SecretAccessKey           string
	R2BucketName                string
	R2Endpoint                  string
	R2Region                    string
	MediaMaxUploadBytes         int64
	MediaPresignedURLTTLSeconds int

	// CORS Origins
	CORSOrigins []string
}

func Load() (*Config, error) {
	// Load .env file in development (ignore if not present)
	_ = godotenv.Load()
	env := getEnv("ENV", "development")

	production := strings.EqualFold(env, "production")
	cookieSameSite := http.SameSiteNoneMode
	if production {
		cookieSameSite = http.SameSiteLaxMode
	}

	mediaDriver := strings.ToLower(strings.TrimSpace(getEnv("MEDIA_STORAGE_DRIVER", "")))
	if mediaDriver == "" {
		if production {
			mediaDriver = "r2"
		} else {
			mediaDriver = "local"
		}
	}

	r2AccountID := getEnv("R2_ACCOUNT_ID", "")
	r2Endpoint := getEnv("R2_ENDPOINT", "")
	if r2Endpoint == "" && r2AccountID != "" {
		r2Endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", r2AccountID)
	}

	cfg := &Config{
		Port:                        getEnvInt("PORT", 8000),
		Env:                         env,
		SecureCookie:                production,
		CookieSecure:                true,
		CookieSameSite:              cookieSameSite,
		DatabaseURL:                 getEnv("DATABASE_URL", "postgres://blink:devpass@localhost:5432/blink_db"),
		RedisURL:                    getEnv("REDIS_URL", "redis://localhost:6379"),
		LogLevel:                    getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout:             getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		EncryptionKey:               getEnv("ENCRYPTION_KEY", ""),
		PlatformMode:                getEnv("PLATFORM_MODE", "real"),
		FrontendURL:                 getEnv("FRONTEND_URL", "http://localhost:3000"),
		MediaStorageDriver:          mediaDriver,
		MediaStoragePath:            getEnv("MEDIA_STORAGE_PATH", "./media_uploads"),
		R2AccountID:                 r2AccountID,
		R2AccessKeyID:               getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:           getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:                getEnv("R2_BUCKET_NAME", "blink-media"),
		R2Endpoint:                  r2Endpoint,
		R2Region:                    getEnv("R2_REGION", "auto"),
		MediaMaxUploadBytes:         getEnvInt64("MEDIA_MAX_UPLOAD_BYTES", 2*1024*1024*1024), // 2 GiB
		MediaPresignedURLTTLSeconds: getEnvInt("MEDIA_PRESIGNED_URL_TTL_SECONDS", 900),       // 15 min
		CORSOrigins:                 getEnvStringSlice("CORS_ORIGINS", []string{"http://localhost:3000"}),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	isProd := strings.EqualFold(c.Env, "production")
	driver := strings.ToLower(strings.TrimSpace(c.MediaStorageDriver))

	if isProd {
		if driver != "r2" {
			return errors.New("MEDIA_STORAGE_DRIVER must be 'r2' in production; local filesystem storage is not allowed")
		}
	}

	if driver == "r2" {
		if c.R2AccountID == "" && c.R2Endpoint == "" {
			return errors.New("either R2_ACCOUNT_ID or R2_ENDPOINT is required when MEDIA_STORAGE_DRIVER is 'r2'")
		}
		if c.R2AccessKeyID == "" {
			return errors.New("R2_ACCESS_KEY_ID is required when MEDIA_STORAGE_DRIVER is 'r2'")
		}
		if c.R2SecretAccessKey == "" {
			return errors.New("R2_SECRET_ACCESS_KEY is required when MEDIA_STORAGE_DRIVER is 'r2'")
		}
		if c.R2BucketName == "" {
			return errors.New("R2_BUCKET_NAME is required when MEDIA_STORAGE_DRIVER is 'r2'")
		}
	} else if driver != "local" {
		return fmt.Errorf("unsupported MEDIA_STORAGE_DRIVER %q: must be 'r2' or 'local'", c.MediaStorageDriver)
	}

	return nil
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

func getEnvInt64(key string, defaultVal int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
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
