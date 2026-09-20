package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRepository struct {
    db *pgxpool.Pool
}

func NewOutboxRepository(db *pgxpool.Pool) *OutboxRepository {
    return &OutboxRepository{db: db}
}

// GetPendingEvents retrieves pending outbox events
func (r *OutboxRepository) GetPendingEvents(ctx context.Context, limit int) ([]*model.OutboxEvent, error) {
    rows, err := r.db.Query(
        ctx,
        `
            SELECT id, aggregate_type, aggregate_id, event_type, payload,
                   status, attempts, available_at, created_at, updated_at, published_at
            FROM outbox_events
            WHERE status = 'PENDING' AND available_at <= NOW()
            ORDER BY created_at ASC
            LIMIT $1
        `,
        limit,
    )
    if err != nil {
        return nil, fmt.Errorf("query pending events: %w", err)
    }
    defer rows.Close()

    var events []*model.OutboxEvent
    for rows.Next() {
        var event model.OutboxEvent
        var payloadJSON []byte

        if err := rows.Scan(
            &event.ID, &event.AggregateType, &event.AggregateID, &event.EventType,
            &payloadJSON, &event.Status, &event.Attempts, &event.AvailableAt,
            &event.CreatedAt, &event.UpdatedAt, &event.PublishedAt,
        ); err != nil {
            return nil, fmt.Errorf("scan event: %w", err)
        }

        if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
            return nil, fmt.Errorf("unmarshal payload: %w", err)
        }

        events = append(events, &event)
    }

    return events, rows.Err()
}

// MarkEventProcessing updates an event to PROCESSING status
func (r *OutboxRepository) MarkEventProcessing(ctx context.Context, eventID uuid.UUID) error {
    _, err := r.db.Exec(
        ctx,
        `
            UPDATE outbox_events
            SET status = 'PROCESSING', attempts = attempts + 1, updated_at = NOW()
            WHERE id = $1
        `,
        eventID,
    )

    if err != nil {
        return fmt.Errorf("mark event processing: %w", err)
    }

    return nil
}

// MarkEventPublished updates an event to PUBLISHED status
func (r *OutboxRepository) MarkEventPublished(ctx context.Context, eventID uuid.UUID) error {
    now := time.Now()

    _, err := r.db.Exec(
        ctx,
        `
            UPDATE outbox_events
            SET status = 'PUBLISHED', published_at = $1, updated_at = $1
            WHERE id = $2
        `,
        now, eventID,
    )

    if err != nil {
        return fmt.Errorf("mark event published: %w", err)
    }

    return nil
}

// MarkEventFailed updates an event to FAILED status
func (r *OutboxRepository) MarkEventFailed(ctx context.Context, eventID uuid.UUID, reason string) error {
    _, err := r.db.Exec(
        ctx,
        `
            UPDATE outbox_events
            SET status = 'FAILED', updated_at = NOW()
            WHERE id = $1
        `,
        eventID,
    )

    if err != nil {
        return fmt.Errorf("mark event failed: %w", err)
    }

    return nil
}