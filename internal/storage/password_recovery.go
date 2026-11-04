package storage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidResetToken = errors.New("invalid or expired reset token")

type ResetDelivery struct {
	ID             string
	EncryptedEmail string
	Attempt        int
}

type PasswordRecoveryRepository struct{ db *pgxpool.Pool }

func NewPasswordRecoveryRepository(db *pgxpool.Pool) *PasswordRecoveryRepository {
	return &PasswordRecoveryRepository{db: db}
}

// Issue is the same indexed query for known/unknown emails; sending happens later.
func (r *PasswordRecoveryRepository) Issue(ctx context.Context, email string, hash []byte, encrypted string, expires time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO password_resets(user_id,id,token_hash,encrypted_email,expires_at)
		SELECT id,$2,$3,$4,$5 FROM users WHERE email=$1
		ON CONFLICT (user_id) DO UPDATE SET
		    id=EXCLUDED.id, token_hash=EXCLUDED.token_hash, encrypted_email=EXCLUDED.encrypted_email,
		    expires_at=EXCLUDED.expires_at, requested_at=NOW(), consumed_at=NULL,
		    delivery_status='PENDING', delivery_attempts=0, next_attempt_at=NOW(),
		    lease_until=NULL, provider_id=NULL
		WHERE password_resets.requested_at <= NOW()-INTERVAL '60 seconds'`,
		email, uuid.NewString(), hash, encrypted, expires)
	return err
}

func (r *PasswordRecoveryRepository) Valid(ctx context.Context, hash []byte) (bool, error) {
	var valid bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(
	    SELECT 1 FROM password_resets WHERE token_hash=$1
	    AND consumed_at IS NULL AND expires_at>NOW())`, hash).Scan(&valid)
	return valid, err
}

// Consume atomically changes the password, revokes sessions and spends the link.
func (r *PasswordRecoveryRepository) Consume(ctx context.Context, hash []byte, passwordHash string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var userID string
	err = tx.QueryRow(ctx, `SELECT user_id FROM password_resets
	    WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>NOW() FOR UPDATE`, hash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidResetToken
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2, session_version=session_version+1,
	    updated_at=NOW() WHERE id=$1`, userID, passwordHash)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE password_resets SET consumed_at=NOW(),
	    delivery_status='CONSUMED', encrypted_email=NULL, lease_until=NULL WHERE user_id=$1`, userID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PasswordRecoveryRepository) Claim(ctx context.Context) (*ResetDelivery, error) {
	var delivery ResetDelivery
	err := r.db.QueryRow(ctx, `
	    WITH candidate AS (
	        SELECT user_id FROM password_resets
	        WHERE expires_at>NOW() AND consumed_at IS NULL AND encrypted_email IS NOT NULL
	          AND delivery_attempts<5 AND next_attempt_at<=NOW()
	          AND (delivery_status='PENDING' OR (delivery_status='SENDING' AND lease_until<=NOW()))
	        ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1
	    )
	    UPDATE password_resets p SET delivery_status='SENDING',
	        delivery_attempts=delivery_attempts+1, lease_until=NOW()+INTERVAL '30 seconds'
	    FROM candidate c WHERE p.user_id=c.user_id
	    RETURNING p.id,p.encrypted_email,p.delivery_attempts`).Scan(&delivery.ID, &delivery.EncryptedEmail, &delivery.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &delivery, err
}

func (r *PasswordRecoveryRepository) Delivered(ctx context.Context, id, providerID string, attempt int) error {
	_, err := r.db.Exec(ctx, `UPDATE password_resets SET delivery_status='SENT',
	    encrypted_email=NULL, lease_until=NULL, provider_id=$2
	    WHERE id=$1 AND delivery_status='SENDING' AND delivery_attempts=$3`, id, providerID, attempt)
	return err
}

func (r *PasswordRecoveryRepository) DeliveryFailed(ctx context.Context, id string, attempt int, permanent bool) error {
	_, err := r.db.Exec(ctx, `UPDATE password_resets SET
	    delivery_status=CASE WHEN $2 OR delivery_attempts>=5 THEN 'FAILED' ELSE 'PENDING' END,
	    encrypted_email=CASE WHEN $2 OR delivery_attempts>=5 THEN NULL ELSE encrypted_email END,
	    lease_until=NULL, next_attempt_at=NOW()+INTERVAL '1 minute'
	    WHERE id=$1 AND delivery_status='SENDING' AND delivery_attempts=$3`, id, permanent, attempt)
	return err
}

// Remove undeliverable/expired secrets even if no further recovery is requested.
func (r *PasswordRecoveryRepository) Cleanup(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE password_resets SET encrypted_email=NULL,
	    delivery_status='FAILED',lease_until=NULL
	    WHERE encrypted_email IS NOT NULL AND
	    (expires_at<=NOW() OR (delivery_attempts>=5 AND lease_until<=NOW()))`)
	return err
}
