package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAttemptNotClaimed    = errors.New("publication attempt was already claimed")
	ErrAttemptStateConflict = errors.New("publication attempt state conflict")
)

type PublicationRepository struct {
	db *pgxpool.Pool
}

// RecordPermanentFailure atomically finalizes the attempt and target and upserts
// the durable terminal failure summary. post_target_id is the logical job key:
// one Asynq task may contain several platform targets.
func (r *PublicationRepository) RecordPermanentFailure(
	ctx context.Context,
	attemptID, postID, postTargetID uuid.UUID,
	jobID *string,
	taskType, platform string,
	maxAttempts int,
	failureType, failureClass, errorCode, failureReason string,
	platformResponse []byte,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin record permanent failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now()
	var attemptCount int
	if err := tx.QueryRow(ctx, `
		SELECT attempt_count FROM publication_attempts WHERE id = $1 FOR UPDATE
	`, attemptID).Scan(&attemptCount); err != nil {
		return fmt.Errorf("get failed publication attempt: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE publication_attempts
		SET status = 'FAILED', error_class = $1, error_message = $2,
			completed_at = $3, updated_at = $3, next_retry_at = NULL
		WHERE id = $4 AND status IN ('PROCESSING', 'UNKNOWN', 'FAILED')
	`, failureClass, failureReason, now, attemptID); err != nil {
		return fmt.Errorf("finalize publication attempt: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE post_targets SET status = 'FAILED' WHERE id = $1
	`, postTargetID); err != nil {
		return fmt.Errorf("mark post target permanently failed: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO publication_attempt_failures (
			publication_attempt_id, attempt_number, error_code, error_class,
			error_message, platform_response, occurred_at
		)
		SELECT id, attempt_count, $2, $3, $4, $5, $6
		FROM publication_attempts WHERE id = $1
		ON CONFLICT (publication_attempt_id, attempt_number) DO UPDATE SET
			error_code = EXCLUDED.error_code,
			error_class = EXCLUDED.error_class,
			error_message = EXCLUDED.error_message,
			platform_response = EXCLUDED.platform_response,
			occurred_at = EXCLUDED.occurred_at
	`, attemptID, errorCode, failureClass, failureReason, platformResponse, now); err != nil {
		return fmt.Errorf("record terminal attempt failure: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO dead_letter_jobs (
			post_id, post_target_id, job_id, task_type, platform, attempts,
			max_attempts, failure_reason, failure_type, platform_response,
			first_failed_at, last_failed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11, $11, $11)
		ON CONFLICT (post_target_id) DO UPDATE SET
			job_id = COALESCE(EXCLUDED.job_id, dead_letter_jobs.job_id),
			task_type = EXCLUDED.task_type,
			platform = EXCLUDED.platform,
			attempts = EXCLUDED.attempts,
			max_attempts = EXCLUDED.max_attempts,
			failure_reason = EXCLUDED.failure_reason,
			failure_type = EXCLUDED.failure_type,
			platform_response = EXCLUDED.platform_response,
			last_failed_at = EXCLUDED.last_failed_at,
			updated_at = EXCLUDED.updated_at
	`, postID, postTargetID, jobID, taskType, platform, attemptCount, maxAttempts,
		failureReason, failureType, platformResponse, now); err != nil {
		return fmt.Errorf("upsert dead-letter job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit permanent failure: %w", err)
	}
	return nil
}

func NewPublicationRepository(db *pgxpool.Pool) *PublicationRepository {
	return &PublicationRepository{db: db}
}

// GetPostForPublishing retrieves a post and all its pending targets
func (r *PublicationRepository) GetPostForPublishing(ctx context.Context, postID uuid.UUID) (*model.Post, error) {
	var post model.Post

	err := r.db.QueryRow(
		ctx,
		`
            SELECT id, user_id, caption, media_url, media_type, status, created_at, updated_at
            FROM posts
            WHERE id = $1
        `,
		postID,
	).Scan(
		&post.ID, &post.UserID, &post.Caption, &post.MediaURL, &post.MediaType,
		&post.Status, &post.CreatedAt, &post.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}

	return &post, nil
}

// GetPendingPostTargets retrieves all pending targets for a post
func (r *PublicationRepository) GetPendingPostTargets(ctx context.Context, postID uuid.UUID) ([]model.PostTarget, error) {
	rows, err := r.db.Query(
		ctx,
		`
            SELECT id, post_id, social_account_id, status, created_at
            FROM post_targets
            WHERE post_id = $1 AND status = 'PENDING'
        `,
		postID,
	)
	if err != nil {
		return nil, fmt.Errorf("query post targets: %w", err)
	}
	defer rows.Close()

	var targets []model.PostTarget
	for rows.Next() {
		var target model.PostTarget
		if err := rows.Scan(&target.ID, &target.PostID, &target.SocialAccountID, &target.Status, &target.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan post target: %w", err)
		}
		targets = append(targets, target)
	}

	return targets, rows.Err()
}

// GetPublicationAttempt retrieves a publication attempt by post target ID
func (r *PublicationRepository) GetPublicationAttempt(ctx context.Context, postTargetID uuid.UUID) (*model.PublicationAttempt, error) {
	var attempt model.PublicationAttempt

	err := r.db.QueryRow(
		ctx,
		`
			SELECT id, post_target_id, status, attempt_count, platform_post_id, platform_url,
			       error_code, error_message, error_class, next_retry_at,
			       started_at, completed_at, created_at, updated_at
            FROM publication_attempts
            WHERE post_target_id = $1
        `,
		postTargetID,
	).Scan(
		&attempt.ID, &attempt.PostTargetID, &attempt.Status, &attempt.AttemptCount,
		&attempt.PlatformPostID, &attempt.PlatformURL, &attempt.ErrorCode, &attempt.ErrorMessage,
		&attempt.ErrorClass, &attempt.NextRetryAt,
		&attempt.StartedAt, &attempt.CompletedAt, &attempt.CreatedAt, &attempt.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("get publication attempt: %w", err)
	}

	return &attempt, nil
}

// MarkAttemptProcessing claims an attempt for processing.
func (r *PublicationRepository) MarkAttemptProcessing(ctx context.Context, attemptID uuid.UUID) error {
	result, err := r.db.Exec(
		ctx,
		`
            UPDATE publication_attempts
            SET status = 'PROCESSING', started_at = COALESCE(started_at, NOW()),
                attempt_count = attempt_count + 1, updated_at = NOW()
			WHERE id = $1 AND status IN ('PENDING', 'RETRYING', 'UNKNOWN', 'FAILED')
        `,
		attemptID,
	)

	if err != nil {
		return fmt.Errorf("mark attempt processing: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAttemptNotClaimed
	}

	return nil
}

// MarkAttemptSucceeded updates an attempt to SUCCEEDED status
func (r *PublicationRepository) MarkAttemptSucceeded(
	ctx context.Context,
	attemptID uuid.UUID,
	platformPostID string,
	platformURL string,
) error {
	now := time.Now()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin mark attempt succeeded: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := tx.Exec(
		ctx,
		`
            UPDATE publication_attempts
            SET status = 'SUCCEEDED', platform_post_id = $1, platform_url = $2, completed_at = $3, updated_at = $3
            WHERE id = $4 AND status IN ('PROCESSING', 'UNKNOWN')
        `,
		platformPostID, platformURL, now, attemptID,
	)

	if err != nil {
		return fmt.Errorf("mark attempt succeeded: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAttemptStateConflict
	}

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO publish_results (
				publication_attempt_id, post_target_id, platform_post_id,
				published_url, status, published_at
			)
			SELECT id, post_target_id, $2, $3, 'SUCCEEDED', $4
			FROM publication_attempts
			WHERE id = $1
		ON CONFLICT (publication_attempt_id) DO UPDATE SET
				platform_post_id = EXCLUDED.platform_post_id,
				published_url = EXCLUDED.published_url,
				status = EXCLUDED.status,
				published_at = EXCLUDED.published_at
		`,
		attemptID, platformPostID, platformURL, now,
	)
	if err != nil {
		return fmt.Errorf("persist publish result: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit mark attempt succeeded: %w", err)
	}

	return nil
}

// MarkAttemptFailed updates an attempt to FAILED status with error details
func (r *PublicationRepository) MarkAttemptFailed(
	ctx context.Context,
	attemptID uuid.UUID,
	errorCode string,
	errorMessage string,
) error {
	now := time.Now()
	result, err := r.db.Exec(
		ctx,
		`
            UPDATE publication_attempts
			SET status = 'FAILED', error_code = $1, error_message = $2,
			    completed_at = $3, updated_at = $3, next_retry_at = NULL
            WHERE id = $4 AND status IN ('PROCESSING', 'UNKNOWN')
        `,
		errorCode, errorMessage, now, attemptID,
	)

	if err != nil {
		return fmt.Errorf("mark attempt failed: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAttemptStateConflict
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO publication_attempt_failures (
			publication_attempt_id, attempt_number, error_code, error_class,
			error_message, occurred_at
		)
		SELECT id, attempt_count, $1, error_class, $2, $3
		FROM publication_attempts WHERE id = $4
		ON CONFLICT (publication_attempt_id, attempt_number) DO UPDATE SET
			error_code = EXCLUDED.error_code, error_message = EXCLUDED.error_message,
			occurred_at = EXCLUDED.occurred_at
	`, errorCode, errorMessage, now, attemptID); err != nil {
		return fmt.Errorf("record failed attempt: %w", err)
	}

	return nil
}

// MarkAttemptRetrying persists a retry decision without consuming another delivery attempt.
func (r *PublicationRepository) MarkAttemptRetrying(ctx context.Context, attemptID uuid.UUID, errorCode, errorClass, errorMessage string, nextRetryAt time.Time) error {
	result, err := r.db.Exec(ctx, `
		UPDATE publication_attempts
		SET status = 'RETRYING', error_code = $1, error_class = $2, error_message = $3,
		    next_retry_at = $4, completed_at = NULL, updated_at = NOW()
		WHERE id = $5 AND status IN ('PROCESSING', 'PENDING')
	`, errorCode, errorClass, errorMessage, nextRetryAt, attemptID)
	if err != nil {
		return fmt.Errorf("mark attempt retrying: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAttemptStateConflict
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO publication_attempt_failures (
			publication_attempt_id, attempt_number, error_code, error_class,
			error_message, occurred_at
		)
		SELECT id, attempt_count, $1, $2, $3, NOW()
		FROM publication_attempts WHERE id = $4
		ON CONFLICT (publication_attempt_id, attempt_number) DO UPDATE SET
			error_code = EXCLUDED.error_code, error_class = EXCLUDED.error_class,
			error_message = EXCLUDED.error_message, occurred_at = EXCLUDED.occurred_at
	`, errorCode, errorClass, errorMessage, attemptID); err != nil {
		return fmt.Errorf("record retryable attempt failure: %w", err)
	}
	return nil
}

// MarkPostTargetPublished updates a post target to PUBLISHED status
func (r *PublicationRepository) MarkPostTargetPublished(ctx context.Context, postTargetID uuid.UUID) error {
	_, err := r.db.Exec(
		ctx,
		`
            UPDATE post_targets
            SET status = 'PUBLISHED'
            WHERE id = $1 AND status <> 'PUBLISHED'
        `,
		postTargetID,
	)

	if err != nil {
		return fmt.Errorf("mark post target published: %w", err)
	}

	return nil
}

// MarkPostTargetFailed updates a post target to FAILED status
func (r *PublicationRepository) MarkPostTargetFailed(ctx context.Context, postTargetID uuid.UUID) error {
	_, err := r.db.Exec(
		ctx,
		`
            UPDATE post_targets
            SET status = 'FAILED'
            WHERE id = $1
        `,
		postTargetID,
	)

	if err != nil {
		return fmt.Errorf("mark post target failed: %w", err)
	}

	return nil
}

// GetPostTargetWithSocialAccount retrieves a post target with associated social account
func (r *PublicationRepository) GetPostTargetWithSocialAccount(
	ctx context.Context,
	postTargetID uuid.UUID,
) (*model.PostTarget, *model.SocialAccount, error) {
	var target model.PostTarget
	var account model.SocialAccount

	err := r.db.QueryRow(
		ctx,
		`
            SELECT pt.id, pt.post_id, pt.social_account_id, pt.status, pt.created_at,
                   sa.id, sa.user_id, sa.platform, sa.platform_user_id, sa.access_token,
                   sa.refresh_token, sa.expires_at, sa.created_at
            FROM post_targets pt
            JOIN social_accounts sa ON pt.social_account_id = sa.id
            WHERE pt.id = $1
        `,
		postTargetID,
	).Scan(
		&target.ID, &target.PostID, &target.SocialAccountID, &target.Status, &target.CreatedAt,
		&account.ID, &account.UserID, &account.Platform, &account.PlatformUserID,
		&account.AccessToken, &account.RefreshToken, &account.ExpiresAt, &account.CreatedAt,
	)

	if err != nil {
		return nil, nil, fmt.Errorf("get post target with social account: %w", err)
	}

	return &target, &account, nil
}

// UpdateOAuthTokens atomically persists the complete refreshed credential
// state. A nil refresh token deliberately preserves the existing ciphertext.
func (r *PublicationRepository) UpdateOAuthTokens(ctx context.Context, accountID uuid.UUID, accessToken string, expiresAt *time.Time, refreshToken *string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin oauth token update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		UPDATE social_accounts
		SET access_token = $1,
		    expires_at = COALESCE($2, expires_at),
		    refresh_token = COALESCE($3, refresh_token),
		    updated_at = NOW()
		WHERE id = $4
	`, accessToken, expiresAt, refreshToken, accountID)
	if err != nil {
		return fmt.Errorf("update oauth tokens: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit oauth token update: %w", err)
	}
	return nil
}

// UpdatePostStatus updates a post's overall publishing status
func (r *PublicationRepository) UpdatePostStatus(ctx context.Context, postID uuid.UUID, status string) error {
	_, err := r.db.Exec(
		ctx,
		`
			UPDATE posts
			SET status = $1, updated_at = NOW()
			WHERE id = $2
		`,
		status, postID,
	)

	if err != nil {
		return fmt.Errorf("update post status: %w", err)
	}

	return nil
}

// MarkAttemptUnknown records that the external outcome cannot be proven.
func (r *PublicationRepository) MarkAttemptUnknown(ctx context.Context, attemptID uuid.UUID, errorCode string, errorMessage string) error {
	result, err := r.db.Exec(
		ctx,
		`
            UPDATE publication_attempts
            SET status = 'UNKNOWN', error_code = $1, error_message = $2, updated_at = NOW()
            WHERE id = $3 AND status IN ('PROCESSING', 'UNKNOWN')
        `,
		errorCode, errorMessage, attemptID,
	)
	if err != nil {
		return fmt.Errorf("mark attempt unknown: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAttemptStateConflict
	}
	return nil
}

// ResetAttemptForRetry makes a reconciled-absent attempt claimable again.
func (r *PublicationRepository) ResetAttemptForRetry(ctx context.Context, attemptID uuid.UUID) error {
	_, err := r.db.Exec(
		ctx,
		`
            UPDATE publication_attempts
			SET status = 'PENDING', next_retry_at = NULL, updated_at = NOW()
            WHERE id = $1 AND status IN ('PROCESSING', 'UNKNOWN')
        `,
		attemptID,
	)
	if err != nil {
		return fmt.Errorf("reset attempt for retry: %w", err)
	}
	return nil
}
