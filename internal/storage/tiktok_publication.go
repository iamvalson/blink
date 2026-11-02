package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *PublicationRepository) GetTikTokOptions(ctx context.Context, target uuid.UUID) (model.TikTokOptions, error) {
	var raw []byte
	var options model.TikTokOptions
	if err := r.db.QueryRow(ctx, `SELECT tiktok_options FROM post_targets WHERE id=$1`, target).Scan(&raw); err != nil {
		return options, err
	}
	if len(raw) == 0 {
		return options, errors.New("TikTok publishing options are missing")
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		return options, err
	}
	return options, nil
}
func (r *PublicationRepository) GetAsyncPublication(ctx context.Context, attempt uuid.UUID) (*model.AsyncPublication, error) {
	var op model.AsyncPublication
	err := r.db.QueryRow(ctx, `SELECT publish_id,remote_status,poll_count,next_check_at,deadline FROM tiktok_publications WHERE publication_attempt_id=$1`, attempt).Scan(&op.PublishID, &op.RemoteStatus, &op.PollCount, &op.NextCheckAt, &op.Deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &op, err
}
func (r *PublicationRepository) BeginAsyncPublication(ctx context.Context, attempt uuid.UUID, deadline time.Time) error {
	_, err := r.db.Exec(ctx, `INSERT INTO tiktok_publications(publication_attempt_id,deadline) VALUES($1,$2)`, attempt, deadline)
	return err
}

// Only a confirmed initialization rejection may clear the marker. Never clear on timeout.
func (r *PublicationRepository) ClearRejectedAsyncPublication(ctx context.Context, attempt uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM tiktok_publications WHERE publication_attempt_id=$1 AND publish_id IS NULL`, attempt)
	return err
}

// SaveAsyncPublication makes the operation ID and first check durable together.
func (r *PublicationRepository) SaveAsyncPublication(ctx context.Context, attempt uuid.UUID, publishID string, next time.Time) error {
	return r.persistAsyncCheck(ctx, attempt, publishID, "PROCESSING_DOWNLOAD", next, false)
}
func (r *PublicationRepository) QueueAsyncCheck(ctx context.Context, attempt uuid.UUID, status string, next time.Time) error {
	return r.persistAsyncCheck(ctx, attempt, "", status, next, true)
}
func (r *PublicationRepository) persistAsyncCheck(ctx context.Context, attempt uuid.UUID, publishID, status string, next time.Time, increment bool) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var postID, targetID uuid.UUID
	var generation int
	err = tx.QueryRow(ctx, `SELECT p.id,pt.id,p.scheduling_generation FROM publication_attempts pa JOIN post_targets pt ON pt.id=pa.post_target_id JOIN posts p ON p.id=pt.post_id WHERE pa.id=$1 AND pa.status IN ('PROCESSING','UNKNOWN') AND pt.status='PENDING' AND p.status='PUBLISHING' FOR UPDATE OF p,pa`, attempt).Scan(&postID, &targetID, &generation)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE tiktok_publications SET publish_id=COALESCE(NULLIF($2,''),publish_id),remote_status=$3,next_check_at=$4,poll_count=poll_count+CASE WHEN $5 THEN 1 ELSE 0 END,updated_at=NOW() WHERE publication_attempt_id=$1 AND ($2='' OR publish_id IS NULL OR publish_id=$2)`, attempt, publishID, status, next, increment)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrAttemptStateConflict
	}
	payload, err := json.Marshal(map[string]any{"post_id": postID, "target_id": targetID, "attempt_id": attempt, "scheduling_generation": generation})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(aggregate_type,aggregate_id,event_type,payload,status,available_at) VALUES('post',$1,'TIKTOK_STATUS_CHECK',$2,'PENDING',$3)`, postID, payload, next)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordAsyncTerminal records the platform's terminal response before finalizing
// the Blink attempt. A failed finalization still reconciles using the same ID.
func (r *PublicationRepository) RecordAsyncTerminal(ctx context.Context, attempt uuid.UUID, status string) error {
	if status != "PUBLISH_COMPLETE" && status != "FAILED" {
		return errors.New("invalid terminal TikTok status")
	}
	tag, err := r.db.Exec(ctx, `UPDATE tiktok_publications SET remote_status=$2,next_check_at=NULL,updated_at=NOW() WHERE publication_attempt_id=$1 AND publish_id IS NOT NULL`, attempt, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrAttemptStateConflict
	}
	return nil
}
