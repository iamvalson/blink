package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOutboxClaimLost = errors.New("outbox claim lost")

type OutboxRepository struct {
	db *pgxpool.Pool
}

type ClaimedOutboxEvent struct {
	Event       model.OutboxEvent
	ClaimToken  uuid.UUID
	DecodeError error
}

func NewOutboxRepository(db *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) ClaimPendingEvents(
	ctx context.Context,
	limit int,
	lease time.Duration,
) ([]ClaimedOutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if lease <= 0 {
		return nil, errors.New("outbox lease must be positive")
	}

	token := uuid.New()

	rows, err := r.db.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM outbox_events
			WHERE
				(status = 'PENDING' AND available_at <= NOW())
				OR (
					status = 'PROCESSING'
					AND COALESCE(locked_until, updated_at) <= NOW()
				)
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events AS e
		SET status = 'PROCESSING',
		    attempts = e.attempts + 1,
		    claim_token = $2,
		    locked_until = NOW() + ($3::double precision * INTERVAL '1 second'),
		    updated_at = NOW()
		FROM candidates c
		WHERE e.id = c.id
		RETURNING e.id, e.aggregate_type, e.aggregate_id,
		          e.event_type, e.payload, e.status, e.attempts,
		          e.available_at, e.created_at, e.updated_at,
		          e.published_at, e.claim_token
	`, limit, token, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	var events []ClaimedOutboxEvent
	for rows.Next() {
		var claimed ClaimedOutboxEvent
		var payload []byte
		e := &claimed.Event

		if err := rows.Scan(
			&e.ID, &e.AggregateType, &e.AggregateID,
			&e.EventType, &payload, &e.Status, &e.Attempts,
			&e.AvailableAt, &e.CreatedAt, &e.UpdatedAt,
			&e.PublishedAt, &claimed.ClaimToken,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(payload, &e.Payload); err != nil {
			claimed.DecodeError = fmt.Errorf("decode event %s: %w", e.ID, err)
		}
		events = append(events, claimed)
	}

	return events, rows.Err()
}

// FinishClaim uses the claim token to prevent an expired dispatcher from
// completing an event now owned by another dispatcher.
func (r *OutboxRepository) FinishClaim(
	ctx context.Context,
	id, token uuid.UUID,
	status string,
	retryAt time.Time,
	reason string,
) error {
	if status != "PUBLISHED" && status != "PENDING" && status != "FAILED" {
		return fmt.Errorf("invalid outbox completion status: %s", status)
	}

	result, err := r.db.Exec(ctx, `
		UPDATE outbox_events
		SET status = $3::text,
		    available_at = CASE WHEN $3::text = 'PENDING' THEN $4 ELSE available_at END,
		    published_at = CASE WHEN $3::text = 'PUBLISHED' THEN NOW() ELSE published_at END,
		    last_error = NULLIF($5, ''),
		    claim_token = NULL,
		    locked_until = NULL,
		    updated_at = NOW()
		WHERE id = $1
		  AND claim_token = $2
		  AND status = 'PROCESSING'
	`, id, token, status, retryAt, reason)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrOutboxClaimLost
	}
	return nil
}

// RecoverStalledPosts repairs delivery gaps after worker crashes,
// exhausted infrastructure retries, or lost Redis jobs.
//
// It does not reset publication attempts. PROCESSING/UNKNOWN attempts
// must still be reconciled by the worker before any new publication.
func (r *OutboxRepository) RecoverStalledPosts(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		WITH candidates AS (
		SELECT p.id
		FROM posts p
		WHERE p.status IN ('QUEUED', 'PUBLISHING')
		  AND p.updated_at < NOW() - INTERVAL '10 minutes'
		  AND (
			EXISTS (
				SELECT 1
				FROM post_targets pt
				JOIN publication_attempts pa ON pa.post_target_id = pt.id
				WHERE pt.post_id = p.id
				  AND pt.status = 'PENDING'
				  AND COALESCE(pa.error_code, '') <> 'MANUAL_REVIEW_REQUIRED'
				  AND (pa.next_retry_at IS NULL OR pa.next_retry_at <= NOW())
			)
			OR NOT EXISTS (
				SELECT 1 FROM post_targets pt
				WHERE pt.post_id = p.id AND pt.status = 'PENDING'
			)
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM outbox_events e
			WHERE e.aggregate_type = 'post'
			  AND e.aggregate_id = p.id
			  AND (
				e.status IN ('PENDING', 'PROCESSING')
				OR e.created_at > NOW() - INTERVAL '10 minutes'
			  )
		  )
		ORDER BY p.updated_at
		LIMIT 100
		FOR UPDATE OF p SKIP LOCKED
		), claimed AS (
			UPDATE posts p SET updated_at = NOW()
			FROM candidates c WHERE p.id = c.id
			RETURNING p.id, p.scheduling_generation
		)
		INSERT INTO outbox_events (
			aggregate_type, aggregate_id, event_type, payload, status
		)
		SELECT 'post', id, 'POST_CREATED',
			jsonb_build_object('post_id', id,
				'scheduling_generation', scheduling_generation), 'PENDING'
		FROM claimed
	`)
	return err
}
