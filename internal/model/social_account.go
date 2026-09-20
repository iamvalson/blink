package model

import (
	"time"

	"github.com/google/uuid"
)

type SocialAccount struct {
    ID              uuid.UUID  `json:"id"`
    UserID          uuid.UUID  `json:"user_id"`
    Platform        string     `json:"platform"` // twitter, youtube, etc
    PlatformUserID  string     `json:"platform_user_id"`
    AccessToken     string     `json:"-"` // Never expose in JSON
    RefreshToken    *string    `json:"-"` // Never expose in JSON
    ExpiresAt       *time.Time `json:"expires_at,omitempty"`
    CreatedAt       time.Time  `json:"created_at"`
}