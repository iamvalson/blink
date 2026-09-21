package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)



var ErrPostNotFound = errors.New("post not found")

type PostRepository struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) *PostRepository {
	return &PostRepository{db: db}
}

// GetPostWithDetails retrieves a post and its publishing targets with attempt results
func (r *PostRepository) GetPostWithDetails(ctx context.Context, userID, postID uuid.UUID) (*model.PostWithDetails, error) {
	var post model.PostWithDetails
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, caption, media_url, media_type, status, created_at, updated_at
		FROM posts
		WHERE id = $1 AND user_id = $2
	`, postID, userID).Scan(
		&post.ID,
		&post.UserID,
		&post.Caption,
		&post.MediaURL,
		&post.MediaType,
		&post.Status,
		&post.CreatedAt,
		&post.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPostNotFound
		}
		return nil, fmt.Errorf("get post: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT 
			pt.id,
			pt.social_account_id,
			sa.platform,
			sa.platform_user_id,
			pt.status,
			pa.platform_post_id,
			pa.platform_url,
			pa.error_code,
			pa.error_message,
			pa.completed_at
		FROM post_targets pt
		JOIN social_accounts sa ON sa.id = pt.social_account_id
		LEFT JOIN publication_attempts pa ON pa.post_target_id = pt.id
		WHERE pt.post_id = $1
		ORDER BY pt.created_at ASC
	`, postID)
	if err != nil {
		return nil, fmt.Errorf("get post targets: %w", err)
	}
	defer rows.Close()

	post.Targets = make([]model.PostTargetDetail, 0)
	for rows.Next() {
		var target model.PostTargetDetail
		err := rows.Scan(
			&target.ID,
			&target.SocialAccountID,
			&target.Platform,
			&target.PlatformUserID,
			&target.Status,
			&target.PlatformPostID,
			&target.PlatformURL,
			&target.ErrorCode,
			&target.ErrorMessage,
			&target.PublishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan post target: %w", err)
		}
		post.Targets = append(post.Targets, target)
	}

	return &post, nil
}



// CreatePost creates a post, its targets, publication attempts, idempotency record and outbox event in one transaction
func (r *PostRepository) CreatePost(
	ctx context.Context,
	userID uuid.UUID,
	idempotencyKey string,
	input model.CreatePostInput,
) (*model.Post, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	requestHash, err := hashCreatePostRequest(input)
	if err != nil{
		return nil, fmt.Errorf("hash create post request: %w", err)
	}

	// Create idempotency record
	var idempotencyID uuid.UUID

	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO idempotency_keys (
				user_id,
				key,
				request_hash,
				status,
				expires_at
			) VALUES (
				$1,
				$2,
				$3,
				'PROCESSING',
				NOW() + INTERVAL '24 hours' 
			) ON CONFLICT (user_id, key) DO NOTHING
			RETURNING id
		`, userID, idempotencyKey, requestHash,
	).Scan(&idempotencyID)

	if err != nil{
		if errors.Is(err, pgx.ErrNoRows) {
			return r.handleExistingIdempotencyKey(
				ctx,
				tx,
				userID,
				idempotencyKey,
				requestHash,
			)
		}

		return nil, fmt.Errorf("create idempotency key: %w", err)
	}



	// Validate all social accounts belongs to the user
	for _, socialAccountID := range input.Targets{
		var exists bool

		err := tx.QueryRow(
			ctx,
			`
				SELECT EXISTS (
					SELECT 1
					FROM social_accounts
					WHERE id = $1
					AND user_id = $2
				)
			`,
			socialAccountID,
			userID,
		).Scan(&exists)
		if err != nil { 
			return nil, fmt.Errorf("validate social account: %w", err) 
		} 
		if !exists { 
			return nil, fmt.Errorf("social account %s not found", socialAccountID) 
		}
	}



	// Create the post
	var post model.Post

	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO posts (
				user_id,
				caption,
				media_url,
				media_type,
				status
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				'QUEUED'
			)
			RETURNING
				id,
				user_id,
				caption,
				media_url,
				media_type,
				status,
				created_at,
				updated_at
		`,
		userID,
		input.Caption,
		input.MediaURL,
		input.MediaType,
	).Scan(
		&post.ID, 
		&post.UserID, 
		&post.Caption, 
		&post.MediaURL, 
		&post.MediaType, 
		&post.Status, 
		&post.CreatedAt, 
		&post.UpdatedAt, 
	) 
	if err != nil { 
		return nil, fmt.Errorf("create post: %w", err) 
	}


	// Create post targets
	for _, socialAccountID := range input.Targets{
		var postTargetID uuid.UUID

		err := tx.QueryRow(
			ctx,
			`
				INSERT INTO post_targets (
					post_id,
					social_account_id,
					status
				) VALUES (
					$1,
					$2,
					'PENDING'
				)
				RETURNING id
			`,
			post.ID,
			socialAccountID,
		).Scan(&postTargetID)

		if err != nil { 
			return nil, fmt.Errorf("create post target: %w", err) 
		}



		// Create publication attempt
		_, err = tx.Exec(
			ctx,
			`
				INSERT INTO publication_attempts(
					post_target_id,
					status,
					attempt_count
				)
				VALUES (
					$1,
					'PENDING',
					0
				)
			`,
			postTargetID,
		)
		if err != nil { 
			return nil, fmt.Errorf("create publication attempt: %w", err) 
		}
	}



	// Create outbox event
	outboxPayload := map[string]interface{}{
		"post_id": post.ID,
	}


	payload, err := json.Marshal(outboxPayload)
	if err != nil{
		return nil, fmt.Errorf("marshal outbox payload: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO outbox_events (
				aggregate_type,
				aggregate_id,
				event_type,
				payload,
				status
			)
			VALUES (
				'post',
				$1,
				'POST_CREATED',
				$2,
				'PENDING'
			)
		`, post.ID, payload,
	)

	if err != nil { 
		return nil, fmt.Errorf("create outbox event: %w", err) 
	}



	// Store this response against the idempotency record
	responseBody, err := json.Marshal(post)
	if err != nil{
		return nil, fmt.Errorf("marshal idempotency response: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`
			UPDATE idempotency_keys
			SET 
				status = 'COMPLETED',
				resource_type = 'post',
				resource_id = $1,
				response_status = 202,
				response_body = $2,
				updated_at = NOW()
			WHERE id = $3
		`,
		post.ID,
		responseBody,
		idempotencyID,
	)

	if err != nil { 
		return nil, fmt.Errorf("complete idempotency key: %w", err) 
	}




	// Commit Transaction
	if err := tx.Commit(ctx); err != nil{
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &post, nil
}





func (r *PostRepository) handleExistingIdempotencyKey(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	idempotencyKey string,
	requestHash string,
) (*model.Post, error) {
	var record model.IdempotencyRecord

	err := tx.QueryRow(
		ctx,
		`
		SELECT
			id,
			request_hash,
			status,
			resource_type,
			resource_id,
			response_status,
			response_body,
			expires_at
		FROM idempotency_keys
		WHERE user_id = $1
		AND key = $2
		`,
		userID,
		idempotencyKey,
	).Scan(
		&record.ID,
		&record.RequestHash,
		&record.Status,
		&record.ResourceType,
		&record.ResourceID,
		&record.ResponseStatus,
		&record.ResponseBody,
		&record.ExpiresAt,
	)


	if err != nil{
		return nil, fmt.Errorf("get existing idempotency key: %w", err)
	}


	// The same key cannot be used for a different request
	if record.RequestHash != requestHash{
		return nil, errors.New( "idempotency key has already been used with a different request", )
	}


	// The original request is still being processed
	if record.Status == "PROCESSING" {
		return nil, errors.New( "request with this idempotency key is already being processed", )
	}


	// The request already completed
	if record.Status == "COMPLETED" && record.ResourceType != nil && *record.ResourceType == "post" && record.ResourceID != nil {
		var post model.Post

		err := tx.QueryRow(
			ctx,
			`
				SELECT
					id,
					user_id,
					caption,
					media_url,
					media_type,
					status,
					created_at,
					updated_at
				FROM posts
				WHERE id = $1
			`,
			*record.ResourceID,
		).Scan(
			&post.ID, 
			&post.UserID, 
			&post.Caption, 
			&post.MediaURL, 
			&post.MediaType, 
			&post.Status, 
			&post.CreatedAt, 
			&post.UpdatedAt,
		)

		if err != nil { 
			return nil, fmt.Errorf("get idempotent post: %w", err) 
		} 
		return &post, nil
	}

	return nil, errors.New("invalid idempotency state")
}


func hashCreatePostRequest(input model.CreatePostInput) (string, error){
	data := struct {
		Caption *string `json:"caption"` 
		MediaURL *string `json:"media_url"` 
		MediaType *string `json:"media_type"` 
		Targets []uuid.UUID `json:"targets"`
	} {
		Caption: input.Caption, 
		MediaURL: input.MediaURL, 
		MediaType: input.MediaType, 
		Targets: input.Targets,
	}

	encoded, err := json.Marshal(data)
	if err != nil{
		return "", err
	}

	hash := sha256.Sum256(encoded)

	return hex.EncodeToString(hash[:]), nil
}