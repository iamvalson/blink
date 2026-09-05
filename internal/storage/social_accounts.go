package storage

import (
	"context"
	"database/sql"
	"time"
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
	platformUserID	string,
	accessToken		string,
	refreshToken	string,
	expiresAt		time.Time,
) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO social_accounts (
			user_id,
			platform,
			platform_user_id,
			access_token,
			refresh_token,
			expires_at
		)
		VALUES ($1, 'twitter', $2, $3, NULLIF($4, ''), $5)
		ON CONFLICT (user_id, platform)
		DO UPDATE SET
			platform_user_id = EXCLUDED.platform_user_id,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at = EXCLUDED.expires_at
	`, userID, platformUserID, accessToken, refreshToken, expiresAt)

	return err
}