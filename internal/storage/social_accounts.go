package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SocialPlatform is a typed string for platform identifiers.
// Use SocialPlatformTwitter etc. on the read/query side for clarity.
// The Upsert write path accepts a plain string so callers (e.g. the generic
// auth handler) don't need to import this package just to cast a platform slug.
type SocialPlatform string

var (
	ErrSocialAccountNotFound = errors.New("account not found")
)

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

func (r *SocialAccountRepository) GetByIDAndUserID(ctx context.Context, userID string, accountID string) (*model.SocialAccount, error) {
	const query = `
		SELECT id, user_id, platform, platform_user_id, access_token, refresh_token, expires_at, created_at, updated_at
		FROM social_accounts
		WHERE user_id = $1 AND id = $2
	`

	var account model.SocialAccount

	err := r.db.QueryRow(ctx, query, userID, accountID).Scan(&account.ID, &account.UserID, &account.Platform, &account.PlatformUserID, &account.AccessToken, &account.RefreshToken, &account.ExpiresAt, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSocialAccountNotFound
		}

		return nil, fmt.Errorf("get account by ids: %w", err)
	}

	return &account, nil
}

func (r *SocialAccountRepository) ListByUserID(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	const query = `
		SELECT 
			sa.id, sa.user_id, sa.platform, sa.platform_user_id, sa.expires_at, sa.created_at, sa.updated_at,
			EXISTS (
				SELECT 1 FROM post_targets pt
				JOIN publication_attempts pa ON pa.post_target_id = pt.id
				WHERE pt.social_account_id = sa.id
				  AND pt.status = 'FAILED'
				  AND pa.error_code = 'ACCOUNT_RECONNECT_REQUIRED'
				  AND (pa.completed_at IS NULL OR pa.completed_at >= sa.updated_at)
			) AS reconnect_required
		FROM social_accounts sa
		WHERE sa.user_id = $1
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list social accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*model.SocialAccount
	for rows.Next() {
		var a model.SocialAccount
		if err := rows.Scan(&a.ID, &a.UserID, &a.Platform, &a.PlatformUserID, &a.ExpiresAt, &a.CreatedAt, &a.UpdatedAt, &a.ReconnectRequired); err != nil {
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

// UpdateOAuthTokens atomically persists the complete refreshed credential
// state. A nil refresh token deliberately preserves the existing ciphertext.
func (r *SocialAccountRepository) UpdateOAuthTokens(ctx context.Context, accountID uuid.UUID, accessToken string, expiresAt *time.Time, refreshToken *string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin oauth token update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
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

	// Check whether exactly one row was updated
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"expected to update 1 social account, updated %d",
			tag.RowsAffected(),
		)
	}

	// Commit only if the update succeeded
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit oauth token update: %w", err)
	}

	return nil
}

func (r *SocialAccountRepository) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, userID string, accountID string) (*model.SocialAccount, error) {
	const query = `
		SELECT
            id,
            user_id,
            platform,
            platform_user_id,
            access_token,
            refresh_token,
            expires_at,
            created_at,
            updated_at
        FROM social_accounts
        WHERE user_id = $1 AND id = $2
        FOR UPDATE
	`

	var account model.SocialAccount

	err := tx.QueryRow(ctx, query, userID, accountID).Scan(
		&account.ID,
		&account.UserID,
		&account.Platform,
		&account.PlatformUserID,
		&account.AccessToken,
		&account.RefreshToken,
		&account.ExpiresAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSocialAccountNotFound
		}

		return nil, fmt.Errorf("failed to get account for update: %w", err)
	}

	return &account, nil
}

func (r *SocialAccountRepository) UpdateOAuthTokensTX(ctx context.Context, tx pgx.Tx, accessToken string, expiresAt time.Time, refreshToken *string, accountID string) error {
	const query = `
		UPDATE social_accounts
		SET access_token = $1, 
			expires_at = $2,
			refresh_token = COALESCE($3, refresh_token),
			updated_at = NOW()
		WHERE id = $4
	`

	tag, err := tx.Exec(ctx, query, accessToken, expiresAt, refreshToken, accountID)

	if err != nil {
		return fmt.Errorf("update oauth tokens: %w", err)
	}

	// Check whether exactly one row was updated
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"expected to update 1 social account, updated %d",
			tag.RowsAffected(),
		)
	}

	return nil
}
