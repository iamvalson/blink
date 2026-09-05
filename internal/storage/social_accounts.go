package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type SocialPlatform string

const (
	SocialPlatformTwitter SocialPlatform = "twitter"
)


type SocialAccountRepository struct {
	db *sql.DB
}


func NewSocialAccountRepository(db *sql.DB) *SocialAccountRepository {
	return &SocialAccountRepository{db: db}
}


func (r *SocialAccountRepository) Upsert(
	ctx context.Context,
	userID	string,
	platform SocialPlatform,
	platformUserID	string,
	accessToken		string,
	refreshToken	string,
	expiresAt		time.Time,
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
			expires_at = EXCLUDED.expires_at
	`
	_, err := r.db.ExecContext(ctx, query, userID, platform, platformUserID, accessToken, refreshToken, expiresAt)

	if err != nil {
		return fmt.Errorf("upsert social account: %w", err)
	}

	return nil
}