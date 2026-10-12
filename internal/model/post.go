package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Content format constants for posts.
// These describe the intended publishing format and are independent of post status.
const (
	ContentFormatVideo = "VIDEO"
	ContentFormatShort = "SHORT"
)

// Post status constants.
const (
	StatusDraft              = "DRAFT"
	StatusScheduled          = "SCHEDULED"
	StatusQueued             = "QUEUED"
	StatusPublishing         = "PUBLISHING"
	StatusPublished          = "PUBLISHED"
	StatusPartiallyPublished = "PARTIALLY_PUBLISHED"
	StatusFailed             = "FAILED"
	StatusCancelled          = "CANCELLED"
)

type CreatePostInput struct {
	Caption       *string     `json:"caption"`
	Title         *string     `json:"title,omitempty"`
	MediaURL      *string     `json:"media_url"`
	MediaType     *string     `json:"media_type"`
	MediaID       *uuid.UUID  `json:"media_id,omitempty"`
	MediaIDs      []uuid.UUID `json:"media_ids,omitempty"`
	Targets       []uuid.UUID `json:"targets"`
	ContentFormat string      `json:"content_format,omitempty"`
	ScheduledAt   *time.Time  `json:"scheduled_at,omitempty"`
}

func (c *CreatePostInput) UnmarshalJSON(data []byte) error {
	type Alias CreatePostInput
	aux := &struct {
		AltMediaIDs []uuid.UUID `json:"mediaIds"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(c.MediaIDs) == 0 && len(aux.AltMediaIDs) > 0 {
		c.MediaIDs = aux.AltMediaIDs
	}
	if c.MediaID == nil && len(c.MediaIDs) > 0 {
		c.MediaID = &c.MediaIDs[0]
	} else if c.MediaID != nil && len(c.MediaIDs) == 0 {
		c.MediaIDs = []uuid.UUID{*c.MediaID}
	}
	return nil
}

type ReschedulePostInput struct {
	ScheduledAt time.Time `json:"scheduled_at"`
}

type Post struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               uuid.UUID  `json:"user_id"`
	Caption              *string    `json:"caption,omitempty"`
	Title                *string    `json:"title,omitempty"`
	MediaURL             *string    `json:"media_url,omitempty"`
	MediaType            *string    `json:"media_type,omitempty"`
	MediaID              *uuid.UUID `json:"media_id,omitempty"`
	Status               string     `json:"status"`
	ContentFormat        string     `json:"content_format"`
	ScheduledAt          *time.Time `json:"scheduled_at,omitempty"`
	PublishedAt          *time.Time `json:"published_at,omitempty"`
	SchedulingGeneration int        `json:"scheduling_generation"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
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
	ID                   uuid.UUID          `json:"id"`
	UserID               uuid.UUID          `json:"user_id"`
	Caption              *string            `json:"caption,omitempty"`
	Title                *string            `json:"title,omitempty"`
	MediaURL             *string            `json:"media_url,omitempty"`
	MediaType            *string            `json:"media_type,omitempty"`
	MediaID              *uuid.UUID         `json:"media_id,omitempty"`
	Status               string             `json:"status"`
	ContentFormat        string             `json:"content_format"`
	ScheduledAt          *time.Time         `json:"scheduled_at,omitempty"`
	PublishedAt          *time.Time         `json:"published_at,omitempty"`
	SchedulingGeneration int                `json:"scheduling_generation"`
	Targets              []PostTargetDetail `json:"targets"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
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

