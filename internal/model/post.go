package model

import (
	"time"

	"github.com/google/uuid"
)


type CreatePostInput struct {
	Caption		*string		`json:"caption"`
	MediaURL	*string		`json:"media_url"`
	MediaType	*string		`json:"media_type"`
	Targets		[]uuid.UUID	`json:"targets"`
}

type Post struct { 
	ID uuid.UUID `json:"id"` 
	UserID uuid.UUID `json:"user_id"` 
	Caption *string `json:"caption,omitempty"` 
	MediaURL *string `json:"media_url,omitempty"` 
	MediaType *string `json:"media_type,omitempty"` 
	Status string `json:"status"` 
	CreatedAt time.Time `json:"created_at"` 
	UpdatedAt time.Time `json:"updated_at"` 
}

type PostTargetDetail struct {
	ID              uuid.UUID  `json:"id"`
	SocialAccountID uuid.UUID  `json:"social_account_id"`
	Platform        string     `json:"platform"`
	PlatformUserID  string     `json:"platform_user_id"`
	Status          string     `json:"status"`
	PlatformPostID  *string    `json:"platform_post_id,omitempty"`
	PlatformURL     *string    `json:"platform_url,omitempty"`
	ErrorCode       *string    `json:"error_code,omitempty"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
}

type PostWithDetails struct {
	ID        uuid.UUID          `json:"id"`
	UserID    uuid.UUID          `json:"user_id"`
	Caption   *string            `json:"caption,omitempty"`
	MediaURL  *string            `json:"media_url,omitempty"`
	MediaType *string            `json:"media_type,omitempty"`
	Status    string             `json:"status"`
	Targets   []PostTargetDetail `json:"targets"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}


type IdempotencyRecord struct {
	ID uuid.UUID 
	RequestHash string 
	Status string 
	ResourceType *string 
	ResourceID *uuid.UUID 
	ResponseStatus *int 
	ResponseBody []byte 
	ExpiresAt time.Time
}
