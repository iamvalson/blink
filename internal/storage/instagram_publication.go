package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type InstagramPublication struct {
	Children               []string
	Container, PublishedID *string
	SubmittedAt            *time.Time
	Deadline, NextCheck    time.Time
}

func (r *PublicationRepository) GetInstagramPublication(ctx context.Context, attempt uuid.UUID) (*InstagramPublication, error) {
	var op InstagramPublication
	err := r.db.QueryRow(ctx, `SELECT containers,container_id,published_id,submitted_at,deadline,next_check_at FROM instagram_publications WHERE attempt_id=$1`, attempt).
		Scan(&op.Children, &op.Container, &op.PublishedID, &op.SubmittedAt, &op.Deadline, &op.NextCheck)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &op, err
}

func (r *PublicationRepository) InstagramMedia(ctx context.Context, post, target uuid.UUID) (string, []uuid.UUID, error) {
	var format string
	var media []uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT COALESCE(instagram_format,''), ARRAY(SELECT COALESCE(media_id,'00000000-0000-0000-0000-000000000000'::uuid)
 FROM instagram_post_media WHERE post_id=$1 ORDER BY position)
 FROM post_targets WHERE post_id=$1 AND id=$2`, post, target).Scan(&format, &media)
	return format, media, err
}

// SaveInstagramStep advances the operation and atomically schedules its follow-up.
func (r *PublicationRepository) SaveInstagramStep(ctx context.Context, attempt uuid.UUID, op InstagramPublication, next *time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var post, target uuid.UUID
	var generation int
	err = tx.QueryRow(ctx, `SELECT p.id,pt.id,p.scheduling_generation FROM publication_attempts pa
 JOIN post_targets pt ON pt.id=pa.post_target_id JOIN posts p ON p.id=pt.post_id
 WHERE pa.id=$1 AND pa.status IN ('PROCESSING','UNKNOWN') AND pt.status='PENDING' AND p.status='PUBLISHING'
 FOR UPDATE OF p,pa,pt`, attempt).Scan(&post, &target, &generation)
	if err != nil {
		return err
	}
	if next != nil {
		op.NextCheck = *next
	}
	if op.Children == nil {
		op.Children = []string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO instagram_publications(attempt_id,containers,container_id,published_id,submitted_at,deadline,next_check_at)
 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(attempt_id) DO UPDATE SET containers=EXCLUDED.containers,
 container_id=EXCLUDED.container_id,published_id=EXCLUDED.published_id,submitted_at=EXCLUDED.submitted_at,
 deadline=EXCLUDED.deadline,next_check_at=EXCLUDED.next_check_at`, attempt, op.Children, op.Container, op.PublishedID, op.SubmittedAt, op.Deadline, op.NextCheck)
	if err != nil {
		return err
	}
	if next != nil {
		payload, err := json.Marshal(map[string]any{"post_id": post, "target_id": target, "attempt_id": attempt, "scheduling_generation": generation})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(aggregate_type,aggregate_id,event_type,payload,status,available_at)
  VALUES('post',$1,'POST_CREATED',$2,'PENDING',$3)`, post, payload, *next)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// WithInstagramGrant holds the account against disconnect until submission
// completes. A job that lost its grant cannot send an old token to Meta.
func (r *PublicationRepository) WithInstagramGrant(ctx context.Context, account, target uuid.UUID, submit func() error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var id uuid.UUID
	err = tx.QueryRow(ctx, `SELECT sa.id FROM social_accounts sa
 JOIN post_targets pt ON pt.social_account_id=sa.id
 WHERE sa.id=$1 AND pt.id=$2 AND pt.status='PENDING' AND sa.platform='instagram'
 FOR KEY SHARE OF sa`, account, target).Scan(&id)
	if err != nil {
		return err
	}
	return submit()
}
