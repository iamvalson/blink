package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
)

func (r *SocialAccountRepository) InstagramAccountsDue(ctx context.Context, cutoff time.Time) ([]model.SocialAccount, error) {
	rows, err := r.db.Query(ctx, `SELECT id,user_id FROM social_accounts WHERE platform='instagram' AND expires_at>NOW() AND expires_at<=$1 ORDER BY expires_at`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []model.SocialAccount
	for rows.Next() {
		var account model.SocialAccount
		if err := rows.Scan(&account.ID, &account.UserID); err != nil {
			return nil, err
		}
		results = append(results, account)
	}
	return results, rows.Err()
}

func (r *SocialAccountRepository) RemoveInstagramGrant(ctx context.Context, remoteID string, issued time.Time, receipt *uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Connection-owned targets, attempts, and platform results are removed by
	// foreign keys. Blink-authored posts and media remain in the user's library.
	if _, err = tx.Exec(ctx, `DELETE FROM social_accounts WHERE platform='instagram' AND (platform_user_id=$1 OR instagram_subject=$1) AND created_at<=$2`, remoteID, issued); err != nil {
		return err
	}
	if receipt != nil {
		if _, err = tx.Exec(ctx, `INSERT INTO instagram_deletion_receipts(id) VALUES($1)`, *receipt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *SocialAccountRepository) InstagramDeletionCompleted(ctx context.Context, id uuid.UUID) (bool, error) {
	var found bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM instagram_deletion_receipts WHERE id=$1)`, id).Scan(&found)
	return found, err
}
