package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5"
)

// AcquirePublishLock returns a repository bound to the session holding
// the advisory lock. Do not use that returned repository concurrently.
func (r *PublicationRepository) AcquirePublishLock(
	ctx context.Context,
	postID uuid.UUID,
) (*PublicationRepository, func(), bool, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	key := "blink:publish:" + postID.String()
	var acquired bool

	err = conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtextextended($1, 0))`,
		key,
	).Scan(&acquired)
	if err != nil {
		// The server may have acquired the lock before the response was lost.
		raw := conn.Hijack()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = raw.Close(cleanupCtx)
		return nil, nil, false, err
	}
	if !acquired {
		conn.Release()
		return nil, nil, false, nil
	}

	release := func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()

		var unlocked bool
		err := conn.QueryRow(cleanupCtx,
			`SELECT pg_advisory_unlock(hashtextextended($1, 0))`,
			key,
		).Scan(&unlocked)

		if err != nil || !unlocked {
			// Never return a potentially locked session to the pool.
			raw := conn.Hijack()
			_ = raw.Close(cleanupCtx)
			return
		}
		conn.Release()
	}

	return &PublicationRepository{
		db:   conn,
		pool: r.pool,
	}, release, true, nil
}

// ClaimPostPublishing competes atomically with cancellation/rescheduling.
// Those operations already lock the same post row.
func (r *PublicationRepository) ClaimPostPublishing(
	ctx context.Context,
	postID uuid.UUID,
	generation int,
) (bool, error) {
	result, err := r.db.Exec(ctx, `
		UPDATE posts
		SET status = 'PUBLISHING', updated_at = NOW()
		WHERE id = $1
		  AND scheduling_generation = $2
		  AND status IN ('QUEUED', 'SCHEDULED', 'PUBLISHING')
		  AND (scheduled_at IS NULL OR scheduled_at <= NOW())
	`, postID, generation)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

// recomputePostStatusTx locks the parent before reading target outcomes.
func recomputePostStatusTx(
	ctx context.Context,
	tx pgx.Tx,
	postID uuid.UUID,
) error {
	var current string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM posts WHERE id = $1 FOR UPDATE`,
		postID,
	).Scan(&current); err != nil {
		return err
	}

	if current == "CANCELLED" || current == "SCHEDULED" || current == "DRAFT" {
		return nil
	}

	var total, published, failed, cancelled int
	err := tx.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
			COUNT(*) FILTER (WHERE status = 'FAILED'),
			COUNT(*) FILTER (WHERE status = 'CANCELLED')
		FROM post_targets
		WHERE post_id = $1
	`, postID).Scan(&total, &published, &failed, &cancelled)
	if err != nil {
		return err
	}
	if total == 0 {
		return nil
	}

	status := "PUBLISHING"
	switch {
	case published == total:
		status = "PUBLISHED"
	case published+failed+cancelled == total && published > 0:
		status = "PARTIALLY_PUBLISHED"
	case failed+cancelled == total && failed > 0:
		status = "FAILED"
	case cancelled == total:
		status = "CANCELLED"
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET status = $2::text,
		    published_at = CASE
		        WHEN $2::text IN ('PUBLISHED', 'PARTIALLY_PUBLISHED')
		        THEN COALESCE(published_at, NOW())
		        ELSE published_at
		    END,
		    updated_at = NOW()
		WHERE id = $1
	`, postID, status)
	return err
}

func (r *PublicationRepository) RecomputePostStatus(
	ctx context.Context,
	postID uuid.UUID,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := recomputePostStatusTx(ctx, tx, postID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkAttemptRetrying atomically stores the retry decision and delivery intent.
func (r *PublicationRepository) MarkAttemptRetrying(
	ctx context.Context,
	attemptID uuid.UUID,
	errorCode, errorClass, errorMessage string,
	nextRetryAt time.Time,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var postID, targetID uuid.UUID
	var generation int

	err = tx.QueryRow(ctx, `
		SELECT p.id, pt.id, p.scheduling_generation
		FROM publication_attempts pa
		JOIN post_targets pt ON pt.id = pa.post_target_id
		JOIN posts p ON p.id = pt.post_id
		WHERE pa.id = $1
		  AND p.status = 'PUBLISHING'
		  AND pt.status = 'PENDING'
		FOR UPDATE OF p, pa
	`, attemptID).Scan(&postID, &targetID, &generation)
	if err != nil {
		return err
	}

	result, err := tx.Exec(ctx, `
		UPDATE publication_attempts
		SET status = 'RETRYING',
		    error_code = $2,
		    error_class = $3,
		    error_message = $4,
		    next_retry_at = $5,
		    completed_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
		  AND status IN ('PROCESSING', 'PENDING', 'FAILED')
	`, attemptID, errorCode, errorClass, errorMessage, nextRetryAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrAttemptStateConflict
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO publication_attempt_failures (
			publication_attempt_id, attempt_number,
			error_code, error_class, error_message, occurred_at
		)
		SELECT id, attempt_count, $2, $3, $4, NOW()
		FROM publication_attempts WHERE id = $1
		ON CONFLICT (publication_attempt_id, attempt_number)
		DO UPDATE SET
			error_code = EXCLUDED.error_code,
			error_class = EXCLUDED.error_class,
			error_message = EXCLUDED.error_message,
			occurred_at = EXCLUDED.occurred_at
	`, attemptID, errorCode, errorClass, errorMessage)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"post_id":               postID,
		"target_id":             targetID,
		"scheduling_generation": generation,
	})
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (
			aggregate_type, aggregate_id, event_type,
			payload, status, available_at
		)
		VALUES ('post', $1, 'POST_CREATED', $2, 'PENDING', $3)
	`, postID, payload, nextRetryAt)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PublicationRepository) QueueReconciliation(
	ctx context.Context,
	postID, targetID uuid.UUID,
	generation int,
) error {
	result, err := r.db.Exec(ctx, `
		INSERT INTO outbox_events (
			aggregate_type, aggregate_id, event_type,
			payload, status, available_at
		)
		SELECT
			'post', p.id, 'POST_CREATED',
			jsonb_build_object(
				'post_id', p.id,
				'target_id', pt.id,
				'scheduling_generation', p.scheduling_generation
			),
			'PENDING',
			NOW() + INTERVAL '30 seconds'
		FROM posts p
		JOIN post_targets pt ON pt.post_id = p.id
		JOIN publication_attempts pa ON pa.post_target_id = pt.id
		WHERE p.id = $1
		  AND pt.id = $2
		  AND p.scheduling_generation = $3
		  AND p.status = 'PUBLISHING'
		  AND pt.status = 'PENDING'
		  AND pa.status = 'UNKNOWN'
		  AND COALESCE(pa.error_code, '') <> 'MANUAL_REVIEW_REQUIRED'
	`, postID, targetID, generation)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("reconciliation intent was not persisted")
	}
	return nil
}

// MarkPostTargetPublished updates the target and parent status atomically.
func (r *PublicationRepository) MarkPostTargetPublished(
	ctx context.Context,
	targetID uuid.UUID,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var postID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE post_targets
		SET status = 'PUBLISHED'
		WHERE id = $1 AND status IN ('PENDING', 'PUBLISHED')
		RETURNING post_id
	`, targetID).Scan(&postID)
	if err != nil {
		return fmt.Errorf("mark target published: %w", err)
	}

	if err := recomputePostStatusTx(ctx, tx, postID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetMediaByID reuses the locked worker session rather than borrowing another
// pool connection while every worker might already hold one.
func (r *PublicationRepository) GetMediaByID(ctx context.Context, id uuid.UUID) (*model.Media, error) {
	return NewMediaRepository(r.db).GetMediaByID(ctx, id)
}
