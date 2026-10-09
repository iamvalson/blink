package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SocialPlatform is a typed string for platform identifiers.
// Use SocialPlatformTwitter etc. on the read/query side for clarity.
// The Upsert write path accepts a plain string so callers (e.g. the generic
// auth handler) don't need to import this package just to cast a platform slug.
type SocialPlatform string

const (
	SocialPlatformTwitter SocialPlatform = "twitter"
)

type SocialAccountRepository struct {
	db *pgxpool.Pool
}

func NewSocialAccountRepository(db *pgxpool.Pool) *SocialAccountRepository {
	return &SocialAccountRepository{db: db}
}

func (r *SocialAccountRepository) Upsert(
	ctx context.Context,
	userID string,
	platform string, // plain string — no import of this package needed by callers
	platformUserID string,
	accessToken string,
	refreshToken string,
	expiresAt time.Time,
) error {
	const query = `
		INSERT INTO social_accounts (
			user_id,
			platform,
			platform_user_id,
			access_token,
			refresh_token,
			expires_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		ON CONFLICT (user_id, platform)
		DO UPDATE SET
			platform_user_id = EXCLUDED.platform_user_id,
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(EXCLUDED.refresh_token, social_accounts.refresh_token),
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`
	_, err := r.db.Exec(ctx, query, userID, platform, platformUserID, accessToken, refreshToken, expiresAt)

	if err != nil {
		return fmt.Errorf("upsert social account: %w", err)
	}

	return nil
}

func (r *SocialAccountRepository) ListByUserID(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	const query = `
		SELECT id, user_id, platform, platform_user_id, expires_at, created_at, updated_at
		FROM social_accounts
		WHERE user_id = $1
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list social accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*model.SocialAccount
	for rows.Next() {
		var a model.SocialAccount
		if err := rows.Scan(&a.ID, &a.UserID, &a.Platform, &a.PlatformUserID, &a.ExpiresAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan social account: %w", err)
		}
		accounts = append(accounts, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate social accounts: %w", err)
	}

	return accounts, nil
}

func (r *SocialAccountRepository) DeleteByUserIDAndPlatform(ctx context.Context, userID string, platform string) error {
	const query = `
		DELETE FROM social_accounts
		WHERE user_id = $1 AND platform = $2
	`
	_, err := r.db.Exec(ctx, query, userID, platform)
	if err != nil {
		return fmt.Errorf("delete social account: %w", err)
	}

	return nil
}
