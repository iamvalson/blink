package model

import (
	"time"

	"github.com/google/uuid"
)

type PostTarget struct {
    ID              uuid.UUID `json:"id"`
    PostID          uuid.UUID `json:"post_id"`
    SocialAccountID uuid.UUID `json:"social_account_id"`
    Status          string    `json:"status"` // PENDING, PUBLISHING, PUBLISHED, FAILED, CANCELLED
    CreatedAt       time.Time `json:"created_at"`
}

type PublicationAttempt struct {
    ID              uuid.UUID  `json:"id"`
    PostTargetID    uuid.UUID  `json:"post_target_id"`
    Status          string     `json:"status"` // PENDING, PROCESSING, SUCCEEDED, FAILED, CANCELLED
    AttemptCount    int        `json:"attempt_count"`
    PlatformPostID  *string    `json:"platform_post_id,omitempty"`
    PlatformURL     *string    `json:"platform_url,omitempty"`
    ErrorCode       *string    `json:"error_code,omitempty"`
    ErrorMessage    *string    `json:"error_message,omitempty"`
    StartedAt       *time.Time `json:"started_at,omitempty"`
    CompletedAt     *time.Time `json:"completed_at,omitempty"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
}

type OutboxEvent struct {
    ID            uuid.UUID      `json:"id"`
    AggregateType string         `json:"aggregate_type"` // e.g., "post"
    AggregateID   uuid.UUID      `json:"aggregate_id"`
    EventType     string         `json:"event_type"` // e.g., "POST_CREATED"
    Payload       map[string]interface{} `json:"payload"`
    Status        string         `json:"status"` // PENDING, PROCESSING, PUBLISHED, FAILED
    Attempts      int            `json:"attempts"`
    AvailableAt   time.Time      `json:"available_at"`
    CreatedAt     time.Time      `json:"created_at"`
    UpdatedAt     time.Time      `json:"updated_at"`
    PublishedAt   *time.Time     `json:"published_at,omitempty"`
}

type PublishInput struct {
    Caption   string
    MediaURL  *string
    MediaType *string
}

type PublishResult struct {
    PlatformPostID string
    PublicURL      string
}