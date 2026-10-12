package mediastore

import (
	"fmt"
	"strings"

	"github.com/iamvalson/blink/internal/config"
)

// InitStore initializes the configured mediastore.Store implementation.
// Both the API server and worker use this function to ensure identical configuration.
func InitStore(cfg *config.Config) (Store, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.MediaStorageDriver))
	isProd := strings.EqualFold(cfg.Env, "production")

	switch driver {
	case "r2":
		return NewR2Store(R2Config{
			AccountID:       cfg.R2AccountID,
			AccessKeyID:     cfg.R2AccessKeyID,
			SecretAccessKey: cfg.R2SecretAccessKey,
			BucketName:      cfg.R2BucketName,
			Endpoint:        cfg.R2Endpoint,
			Region:          cfg.R2Region,
		})
	case "local":
		if isProd {
			return nil, fmt.Errorf("local filesystem media storage is not permitted in production; set MEDIA_STORAGE_DRIVER=r2")
		}
		return NewLocalStore(cfg.MediaStoragePath)
	default:
		if isProd {
			return nil, fmt.Errorf("MEDIA_STORAGE_DRIVER must be 'r2' in production (got %q)", cfg.MediaStorageDriver)
		}
		return NewLocalStore(cfg.MediaStoragePath)
	}
}

