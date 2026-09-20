package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PublicationRepository struct {
    db *pgxpool.Pool
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
                   error_code, error_message, started_at, completed_at, created_at, updated_at
            FROM publication_attempts
            WHERE post_target_id = $1
        `,
        postTargetID,
    ).Scan(
        &attempt.ID, &attempt.PostTargetID, &attempt.Status, &attempt.AttemptCount,
        &attempt.PlatformPostID, &attempt.PlatformURL, &attempt.ErrorCode, &attempt.ErrorMessage,
        &attempt.StartedAt, &attempt.CompletedAt, &attempt.CreatedAt, &attempt.UpdatedAt,
    )
    
    if err != nil {
        return nil, fmt.Errorf("get publication attempt: %w", err)
    }
    
    return &attempt, nil
}

// MarkAttemptProcessing updates an attempt to PROCESSING status
func (r *PublicationRepository) MarkAttemptProcessing(ctx context.Context, attemptID uuid.UUID) error {
    _, err := r.db.Exec(
        ctx,
        `
            UPDATE publication_attempts
            SET status = 'PROCESSING', started_at = NOW(), updated_at = NOW()
            WHERE id = $1
        `,
        attemptID,
    )
    
    if err != nil {
        return fmt.Errorf("mark attempt processing: %w", err)
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
    
    _, err := r.db.Exec(
        ctx,
        `
            UPDATE publication_attempts
            SET status = 'SUCCEEDED', platform_post_id = $1, platform_url = $2,
                completed_at = $3, updated_at = $3
            WHERE id = $4
        `,
        platformPostID, platformURL, now, attemptID,
    )
    
    if err != nil {
        return fmt.Errorf("mark attempt succeeded: %w", err)
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
    
    _, err := r.db.Exec(
        ctx,
        `
            UPDATE publication_attempts
            SET status = 'FAILED', error_code = $1, error_message = $2,
                completed_at = $3, updated_at = $3, attempt_count = attempt_count + 1
            WHERE id = $4
        `,
        errorCode, errorMessage, now, attemptID,
    )
    
    if err != nil {
        return fmt.Errorf("mark attempt failed: %w", err)
    }
    
    return nil
}

// MarkPostTargetPublished updates a post target to PUBLISHED status
func (r *PublicationRepository) MarkPostTargetPublished(ctx context.Context, postTargetID uuid.UUID) error {
    _, err := r.db.Exec(
        ctx,
        `
            UPDATE post_targets
            SET status = 'PUBLISHED', created_at = NOW()
            WHERE id = $1
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