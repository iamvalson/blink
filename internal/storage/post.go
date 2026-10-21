package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPostNotFound            = errors.New("post not found")
	ErrPostDeleteForbidden     = errors.New("post cannot be deleted while publishing is in progress")
	ErrPostCannotBeRescheduled = errors.New("post cannot be rescheduled in its current status")
	ErrInvalidScheduledTime    = errors.New("scheduled time must be in the future")
	ErrPostCannotBeCancelled   = errors.New("post cannot be cancelled in its current status")
)

type PostRepository struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) *PostRepository {
	return &PostRepository{db: db}
}

// ListPostsFilter defines optional filters for listing a user's posts.
type ListPostsFilter struct {
	Status   string // e.g. "QUEUED", "PUBLISHED", "FAILED"; empty means all
	Platform string // e.g. "youtube"; empty means all
	Search   string // partial match against caption/title
	Limit    int    // 0 means default (50)
	Offset   int
}

// GetPostWithDetails retrieves a post and its publishing targets with attempt results
func (r *PostRepository) GetPostWithDetails(ctx context.Context, userID, postID uuid.UUID) (*model.PostWithDetails, error) {
	var post model.PostWithDetails
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, caption, media_url, media_type, media_id, status, content_format,
		       scheduled_at, published_at, scheduling_generation, created_at, updated_at
		FROM posts
		WHERE id = $1 AND user_id = $2
	`, postID, userID).Scan(
		&post.ID,
		&post.UserID,
		&post.Caption,
		&post.MediaURL,
		&post.MediaType,
		&post.MediaID,
		&post.Status,
		&post.ContentFormat,
		&post.ScheduledAt,
		&post.PublishedAt,
		&post.SchedulingGeneration,
		&post.CreatedAt,
		&post.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPostNotFound
		}
		return nil, fmt.Errorf("get post: %w", err)
	}
	if post.Caption != nil {
		firstLine := strings.SplitN(*post.Caption, "\n", 2)[0]
		firstLine = strings.TrimSpace(firstLine)
		if firstLine != "" {
			post.Title = &firstLine
		}
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

// ListPosts retrieves paginated posts for a user with optional filters.
func (r *PostRepository) ListPosts(ctx context.Context, userID uuid.UUID, filter ListPostsFilter) ([]model.PostWithDetails, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Build WHERE clauses dynamically
	conditions := []string{"p.user_id = $1"}
	args := []interface{}{userID}
	argIdx := 2

	if filter.Status != "" {
		if filter.Status == "OVERDUE" {
			conditions = append(conditions, "p.status = 'SCHEDULED' AND p.scheduled_at < NOW()")
		} else {
			conditions = append(conditions, fmt.Sprintf("p.status = $%d", argIdx))
			args = append(args, filter.Status)
			argIdx++
		}
	}
	if filter.Platform != "" {
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM post_targets pt JOIN social_accounts sa ON sa.id = pt.social_account_id WHERE pt.post_id = p.id AND sa.platform = $%d)", argIdx))
		args = append(args, filter.Platform)
		argIdx++
	}
	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("p.caption ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM posts p WHERE %s", where)
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count posts: %w", err)
	}

	// Fetch posts
	listQuery := fmt.Sprintf(`
		SELECT p.id, p.user_id, p.caption, p.media_url, p.media_type, p.media_id,
		       p.status, p.content_format, p.scheduled_at, p.published_at, p.scheduling_generation,
		       p.created_at, p.updated_at
		FROM posts p
		WHERE %s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, filter.Offset)

	rows, err := r.db.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	var posts []model.PostWithDetails
	for rows.Next() {
		var p model.PostWithDetails
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Caption, &p.MediaURL, &p.MediaType, &p.MediaID,
			&p.Status, &p.ContentFormat, &p.ScheduledAt, &p.PublishedAt, &p.SchedulingGeneration,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan post: %w", err)
		}
		if p.Caption != nil {
			firstLine := strings.SplitN(*p.Caption, "\n", 2)[0]
			firstLine = strings.TrimSpace(firstLine)
			if firstLine != "" {
				p.Title = &firstLine
			}
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate posts: %w", err)
	}

	// Fetch targets for all posts in one query
	if len(posts) > 0 {
		postIDs := make([]uuid.UUID, len(posts))
		for i, p := range posts {
			postIDs[i] = p.ID
		}
		targetRows, err := r.db.Query(ctx, `
			SELECT
				pt.id, pt.post_id, pt.social_account_id, sa.platform, sa.platform_user_id,
				pt.status, pa.platform_post_id, pa.platform_url, pa.error_code,
				pa.error_message, pa.completed_at
			FROM post_targets pt
			JOIN social_accounts sa ON sa.id = pt.social_account_id
			LEFT JOIN publication_attempts pa ON pa.post_target_id = pt.id
			WHERE pt.post_id = ANY($1)
			ORDER BY pt.post_id, pt.created_at ASC
		`, postIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("list post targets: %w", err)
		}
		defer targetRows.Close()

		// index posts by ID for O(1) assignment
		postIndex := make(map[uuid.UUID]*model.PostWithDetails, len(posts))
		for i := range posts {
			posts[i].Targets = make([]model.PostTargetDetail, 0)
			postIndex[posts[i].ID] = &posts[i]
		}

		for targetRows.Next() {
			var postID uuid.UUID
			var t model.PostTargetDetail
			if err := targetRows.Scan(
				&t.ID, &postID, &t.SocialAccountID, &t.Platform, &t.PlatformUserID,
				&t.Status, &t.PlatformPostID, &t.PlatformURL, &t.ErrorCode,
				&t.ErrorMessage, &t.PublishedAt,
			); err != nil {
				return nil, 0, fmt.Errorf("scan list target: %w", err)
			}
			if p, ok := postIndex[postID]; ok {
				p.Targets = append(p.Targets, t)
			}
		}
		if err := targetRows.Err(); err != nil {
			return nil, 0, fmt.Errorf("iterate targets: %w", err)
		}
	}

	return posts, total, nil
}

// DeletePost deletes a post and its associated records.
// Posts in QUEUED or PUBLISHING status cannot be deleted to avoid racing with active workers.
func (r *PostRepository) DeletePost(ctx context.Context, userID, postID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify ownership and check status
	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM posts WHERE id = $1 AND user_id = $2`,
		postID, userID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPostNotFound
		}
		return fmt.Errorf("get post status: %w", err)
	}

	if status == "QUEUED" || status == "PUBLISHING" {
		return ErrPostDeleteForbidden
	}

	if _, err := tx.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID); err != nil {
		return fmt.Errorf("delete post: %w", err)
	}

	return tx.Commit(ctx)
}

// RetryPost requeues a failed post for publishing by resetting target/attempt state.
// Only FAILED posts can be retried.
func (r *PostRepository) RetryPost(
	ctx context.Context,
	userID, postID uuid.UUID,
) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var generation int

	err = tx.QueryRow(ctx, `
		SELECT status, scheduling_generation
		FROM posts
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, postID, userID).Scan(&status, &generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPostNotFound
	}
	if err != nil {
		return err
	}

	if status != "FAILED" && status != "PARTIALLY_PUBLISHED" {
		return fmt.Errorf("post cannot be retried in status %s", status)
	}

	generation++

	_, err = tx.Exec(ctx, `
		UPDATE publication_attempts pa
		SET status = 'PENDING',
		    attempt_count = 0,
		    error_code = NULL,
		    error_class = NULL,
		    error_message = NULL,
		    next_retry_at = NULL,
		    started_at = NULL,
		    completed_at = NULL,
		    updated_at = NOW()
		FROM post_targets pt
		WHERE pa.post_target_id = pt.id
		  AND pt.post_id = $1
		  AND pt.status = 'FAILED'
		  AND pa.status = 'FAILED'
	`, postID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE post_targets pt
		SET status = 'PENDING'
		WHERE pt.post_id = $1
		  AND pt.status = 'FAILED'
		  AND EXISTS (
			SELECT 1 FROM publication_attempts pa
			WHERE pa.post_target_id = pt.id AND pa.status = 'PENDING'
		  )
	`, postID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET status = 'QUEUED',
		    scheduled_at = NULL,
		    scheduling_generation = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, postID, generation)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'FAILED',
		    claim_token = NULL,
		    locked_until = NULL,
		    last_error = 'superseded by manual retry',
		    updated_at = NOW()
		WHERE aggregate_type = 'post'
		  AND aggregate_id = $1
		  AND status IN ('PENDING', 'PROCESSING')
	`, postID)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"post_id":               postID,
		"scheduling_generation": generation,
	})
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (
			aggregate_type, aggregate_id, event_type, payload, status
		)
		VALUES ('post', $1, 'POST_CREATED', $2, 'PENDING')
	`, postID, payload)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CreatePost creates a post, its targets, publication attempts, idempotency record and outbox event in one transaction
func (r *PostRepository) CreatePost(
	ctx context.Context,
	userID uuid.UUID,
	idempotencyKey string,
	input model.CreatePostInput,
) (*model.Post, error) {
	if len(input.MediaIDs) > 1 {
		return nil, errors.New("only one media attachment is supported")
	}
	if len(input.MediaIDs) == 1 {
		if input.MediaID != nil && *input.MediaID != input.MediaIDs[0] {
			return nil, errors.New("media_id and media_ids must reference the same media")
		}
		id := input.MediaIDs[0]
		input.MediaID = &id
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	requestHash, err := hashCreatePostRequest(input)
	if err != nil {
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

	if err != nil {
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

	// Validate all social accounts belong to the user and gather platforms
	var targetPlatforms []string
	for _, socialAccountID := range input.Targets {
		var platform string
		err := tx.QueryRow(
			ctx,
			`
				SELECT platform
				FROM social_accounts
				WHERE id = $1
				AND user_id = $2
			`,
			socialAccountID,
			userID,
		).Scan(&platform)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("social account %s not found", socialAccountID)
			}
			return nil, fmt.Errorf("validate social account: %w", err)
		}
		targetPlatforms = append(targetPlatforms, platform)
	}

	// Reject blob: URLs and unauthenticated remote URLs
	if input.MediaURL != nil && *input.MediaURL != "" {
		if strings.HasPrefix(*input.MediaURL, "blob:") {
			return nil, errors.New("browser blob: URLs cannot be used for post creation")
		}
		if input.MediaID == nil && len(input.MediaIDs) == 0 {
			return nil, errors.New("remote media URLs without durable media records are not permitted")
		}
	}

	// Validate media if referenced
	var mediaContentType *string
	mediaIDsToCheck := input.MediaIDs
	if len(mediaIDsToCheck) == 0 && input.MediaID != nil {
		mediaIDsToCheck = []uuid.UUID{*input.MediaID}
	}

	for _, mID := range mediaIDsToCheck {
		var (
			mediaUserID  uuid.UUID
			mediaType    string
			uploadStatus string
		)
		err := tx.QueryRow(
			ctx,
			`
				SELECT user_id, content_type, upload_status
				FROM media
				WHERE id = $1
			`,
			mID,
		).Scan(&mediaUserID, &mediaType, &uploadStatus)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("media not found: %s", mID)
			}
			return nil, fmt.Errorf("validate media: %w", err)
		}
		if mediaUserID != userID {
			return nil, fmt.Errorf("media %s does not belong to user", mID)
		}
		if uploadStatus != "complete" {
			return nil, fmt.Errorf("media upload is not complete: %s (status: %s)", mID, uploadStatus)
		}
		mediaContentType = &mediaType
		if input.MediaType == nil {
			input.MediaType = &mediaType
		}
		if input.MediaID == nil {
			input.MediaID = &mID
		}
	}

	// Validate and normalise content_format before platform checks
	requestedFormat := input.ContentFormat
	if requestedFormat == "" {
		requestedFormat = model.ContentFormatVideo
	}
	switch requestedFormat {
	case model.ContentFormatVideo, model.ContentFormatShort:
		// valid
	default:
		return nil, fmt.Errorf("invalid content_format: %s", requestedFormat)
	}

	// Platform-specific validation
	for _, p := range targetPlatforms {
		if p == "youtube" {
			if input.MediaID == nil {
				return nil, errors.New("YouTube publishing requires a video")
			}
			if mediaContentType != nil && !strings.HasPrefix(*mediaContentType, "video/") {
				return nil, errors.New("YouTube publishing requires a video")
			}
			if input.MediaType != nil && !strings.HasPrefix(*input.MediaType, "video/") {
				return nil, errors.New("YouTube publishing requires a video")
			}
		} else if p == "twitter" {
			if input.MediaID != nil {
				return nil, errors.New("Twitter/X media uploads are not supported yet")
			}
			if mediaContentType != nil {
				ct := *mediaContentType
				if !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "video/") {
					return nil, fmt.Errorf("unsupported media type for Twitter/X: %s", ct)
				}
			}
		}
	}

	// requestedFormat has already been validated and defaulted above.
	contentFormat := requestedFormat

	// Determine initial status based on scheduled_at
	var initialStatus string
	if input.ScheduledAt != nil {
		if !input.ScheduledAt.After(time.Now()) {
			return nil, ErrInvalidScheduledTime
		}
		initialStatus = model.StatusScheduled
	} else {
		initialStatus = model.StatusQueued
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
				media_id,
				status,
				content_format,
				scheduled_at,
				scheduling_generation
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				1
			)
			RETURNING
				id,
				user_id,
				caption,
				media_url,
				media_type,
				media_id,
				status,
				content_format,
				scheduled_at,
				published_at,
				scheduling_generation,
				created_at,
				updated_at
		`,
		userID,
		input.Caption,
		input.MediaURL,
		input.MediaType,
		input.MediaID,
		initialStatus,
		contentFormat,
		input.ScheduledAt,
	).Scan(
		&post.ID,
		&post.UserID,
		&post.Caption,
		&post.MediaURL,
		&post.MediaType,
		&post.MediaID,
		&post.Status,
		&post.ContentFormat,
		&post.ScheduledAt,
		&post.PublishedAt,
		&post.SchedulingGeneration,
		&post.CreatedAt,
		&post.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create post: %w", err)
	}

	// Create post targets
	for _, socialAccountID := range input.Targets {
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

	// Create outbox event only for immediately queued posts
	if initialStatus == model.StatusQueued {
		outboxPayload := map[string]interface{}{
			"post_id":               post.ID,
			"scheduling_generation": post.SchedulingGeneration,
		}

		payload, err := json.Marshal(outboxPayload)
		if err != nil {
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
	}

	// Store this response against the idempotency record
	responseBody, err := json.Marshal(post)
	if err != nil {
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
	if err := tx.Commit(ctx); err != nil {
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

	if err != nil {
		return nil, fmt.Errorf("get existing idempotency key: %w", err)
	}

	// The same key cannot be used for a different request
	if record.RequestHash != requestHash {
		return nil, errors.New("idempotency key has already been used with a different request")
	}

	// The original request is still being processed
	if record.Status == "PROCESSING" {
		return nil, errors.New("request with this idempotency key is already being processed")
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
					media_id,
					status,
					content_format,
					scheduled_at,
					published_at,
					scheduling_generation,
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
			&post.MediaID,
			&post.Status,
			&post.ContentFormat,
			&post.ScheduledAt,
			&post.PublishedAt,
			&post.SchedulingGeneration,
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

func hashCreatePostRequest(input model.CreatePostInput) (string, error) {
	data := struct {
		Caption       *string     `json:"caption"`
		MediaURL      *string     `json:"media_url"`
		MediaType     *string     `json:"media_type"`
		MediaID       *uuid.UUID  `json:"media_id,omitempty"`
		MediaIDs      []uuid.UUID `json:"media_ids,omitempty"`
		Targets       []uuid.UUID `json:"targets"`
		ContentFormat string      `json:"content_format"`
		ScheduledAt   *time.Time  `json:"scheduled_at,omitempty"`
	}{
		Caption:       input.Caption,
		MediaURL:      input.MediaURL,
		MediaType:     input.MediaType,
		MediaID:       input.MediaID,
		MediaIDs:      input.MediaIDs,
		Targets:       input.Targets,
		ContentFormat: input.ContentFormat,
		ScheduledAt:   input.ScheduledAt,
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(encoded)

	return hex.EncodeToString(hash[:]), nil
}

// ReschedulePost changes the scheduled time of a scheduled post and increments its scheduling generation.
func (r *PostRepository) ReschedulePost(ctx context.Context, userID, postID uuid.UUID, newScheduledAt time.Time) (*model.PostWithDetails, error) {
	if !newScheduledAt.After(time.Now()) {
		return nil, ErrInvalidScheduledTime
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	var currentGen int
	err = tx.QueryRow(ctx, `
		SELECT status, scheduling_generation
		FROM posts
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, postID, userID).Scan(&currentStatus, &currentGen)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPostNotFound
		}
		return nil, fmt.Errorf("query post for reschedule: %w", err)
	}

	// Reject if post is actively publishing or already published
	if currentStatus != model.StatusScheduled &&
		currentStatus != model.StatusQueued {
		return nil, ErrPostCannotBeRescheduled
	}

	newGen := currentGen + 1
	if _, err := tx.Exec(ctx, `
		UPDATE posts
		SET scheduled_at = $1, status = 'SCHEDULED', scheduling_generation = $2, updated_at = NOW()
		WHERE id = $3
	`, newScheduledAt, newGen, postID); err != nil {
		return nil, fmt.Errorf("update post schedule: %w", err)
	}

	// Invalidate any old pending or processing outbox events for this post
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'FAILED', updated_at = NOW()
		WHERE aggregate_type = 'post' AND aggregate_id = $1 AND status IN ('PENDING', 'PROCESSING')
	`, postID); err != nil {
		return nil, fmt.Errorf("invalidate outbox events: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reschedule transaction: %w", err)
	}

	return r.GetPostWithDetails(ctx, userID, postID)
}

// CancelSchedule cancels a scheduled post and logically invalidates any queued jobs.
func (r *PostRepository) CancelSchedule(ctx context.Context, userID, postID uuid.UUID) (*model.PostWithDetails, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	var currentGen int
	err = tx.QueryRow(ctx, `
		SELECT status, scheduling_generation
		FROM posts
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, postID, userID).Scan(&currentStatus, &currentGen)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPostNotFound
		}
		return nil, fmt.Errorf("query post for cancel: %w", err)
	}

	if currentStatus != model.StatusScheduled &&
		currentStatus != model.StatusQueued {
		return nil, ErrPostCannotBeCancelled
	}

	newGen := currentGen + 1
	if _, err := tx.Exec(ctx, `
		UPDATE posts
		SET status = 'CANCELLED', scheduling_generation = $1, updated_at = NOW()
		WHERE id = $2
	`, newGen, postID); err != nil {
		return nil, fmt.Errorf("cancel post: %w", err)
	}

	// Cancel post targets
	if _, err := tx.Exec(ctx, `
		UPDATE post_targets
		SET status = 'CANCELLED'
		WHERE post_id = $1 AND status = 'PENDING'
	`, postID); err != nil {
		return nil, fmt.Errorf("cancel post targets: %w", err)
	}

	// Invalidate any old pending or processing outbox events
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'FAILED', updated_at = NOW()
		WHERE aggregate_type = 'post' AND aggregate_id = $1 AND status IN ('PENDING', 'PROCESSING')
	`, postID); err != nil {
		return nil, fmt.Errorf("invalidate outbox events: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cancel transaction: %w", err)
	}

	return r.GetPostWithDetails(ctx, userID, postID)
}

// ListCalendarPosts returns posts within the date interval for the calendar.
func (r *PostRepository) ListCalendarPosts(ctx context.Context, userID uuid.UUID, from, to time.Time, status, platform string) ([]model.PostWithDetails, error) {
	if to.Before(from) {
		return nil, errors.New("'to' date must be after 'from' date")
	}
	if to.Sub(from) > 90*24*time.Hour {
		return nil, errors.New("date range cannot exceed 90 days")
	}

	conditions := []string{
		"p.user_id = $1",
		"COALESCE(p.scheduled_at, p.published_at, p.created_at) >= $2",
		"COALESCE(p.scheduled_at, p.published_at, p.created_at) < $3",
	}
	args := []interface{}{userID, from, to}
	argIdx := 4

	if status != "" {
		if status == "OVERDUE" {
			conditions = append(conditions, "p.status = 'SCHEDULED' AND p.scheduled_at < NOW()")
		} else {
			conditions = append(conditions, fmt.Sprintf("p.status = $%d", argIdx))
			args = append(args, status)
			argIdx++
		}
	}
	if platform != "" {
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM post_targets pt JOIN social_accounts sa ON sa.id = pt.social_account_id WHERE pt.post_id = p.id AND sa.platform = $%d)", argIdx))
		args = append(args, platform)
	}

	where := strings.Join(conditions, " AND ")
	query := fmt.Sprintf(`
		SELECT p.id, p.user_id, p.caption, p.media_url, p.media_type, p.media_id,
		       p.status, p.content_format, p.scheduled_at, p.published_at, p.scheduling_generation,
		       p.created_at, p.updated_at
		FROM posts p
		WHERE %s
		ORDER BY COALESCE(p.scheduled_at, p.published_at, p.created_at) ASC
	`, where)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list calendar posts: %w", err)
	}
	defer rows.Close()

	var posts []model.PostWithDetails
	for rows.Next() {
		var p model.PostWithDetails
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Caption, &p.MediaURL, &p.MediaType, &p.MediaID,
			&p.Status, &p.ContentFormat, &p.ScheduledAt, &p.PublishedAt, &p.SchedulingGeneration,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan calendar post: %w", err)
		}
		if p.Caption != nil {
			firstLine := strings.SplitN(*p.Caption, "\n", 2)[0]
			firstLine = strings.TrimSpace(firstLine)
			if firstLine != "" {
				p.Title = &firstLine
			}
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar posts: %w", err)
	}

	// Fetch targets for posts
	if len(posts) > 0 {
		postIDs := make([]uuid.UUID, len(posts))
		for i, p := range posts {
			postIDs[i] = p.ID
		}
		targetRows, err := r.db.Query(ctx, `
			SELECT
				pt.id, pt.post_id, pt.social_account_id, sa.platform, sa.platform_user_id,
				pt.status, pa.platform_post_id, pa.platform_url, pa.error_code,
				pa.error_message, pa.completed_at
			FROM post_targets pt
			JOIN social_accounts sa ON sa.id = pt.social_account_id
			LEFT JOIN publication_attempts pa ON pa.post_target_id = pt.id
			WHERE pt.post_id = ANY($1)
			ORDER BY pt.post_id, pt.created_at ASC
		`, postIDs)
		if err != nil {
			return nil, fmt.Errorf("list calendar post targets: %w", err)
		}
		defer targetRows.Close()

		postIndex := make(map[uuid.UUID]*model.PostWithDetails, len(posts))
		for i := range posts {
			posts[i].Targets = make([]model.PostTargetDetail, 0)
			postIndex[posts[i].ID] = &posts[i]
		}

		for targetRows.Next() {
			var postID uuid.UUID
			var t model.PostTargetDetail
			if err := targetRows.Scan(
				&t.ID, &postID, &t.SocialAccountID, &t.Platform, &t.PlatformUserID,
				&t.Status, &t.PlatformPostID, &t.PlatformURL, &t.ErrorCode,
				&t.ErrorMessage, &t.PublishedAt,
			); err != nil {
				return nil, fmt.Errorf("scan calendar target: %w", err)
			}
			if p, ok := postIndex[postID]; ok {
				p.Targets = append(p.Targets, t)
			}
		}
		if err := targetRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate calendar targets: %w", err)
		}
	}

	return posts, nil
}

// ListUpcomingPosts returns future scheduled posts ordered by scheduled_at ascending.
func (r *PostRepository) ListUpcomingPosts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.PostWithDetails, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM posts
		WHERE user_id = $1 AND status = 'SCHEDULED' AND scheduled_at > NOW()
	`, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count upcoming posts: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, caption, media_url, media_type, media_id,
		       status, content_format, scheduled_at, published_at, scheduling_generation,
		       created_at, updated_at
		FROM posts
		WHERE user_id = $1 AND status = 'SCHEDULED' AND scheduled_at > NOW()
		ORDER BY scheduled_at ASC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query upcoming posts: %w", err)
	}
	defer rows.Close()

	var posts []model.PostWithDetails
	for rows.Next() {
		var p model.PostWithDetails
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Caption, &p.MediaURL, &p.MediaType, &p.MediaID,
			&p.Status, &p.ContentFormat, &p.ScheduledAt, &p.PublishedAt, &p.SchedulingGeneration,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan upcoming post: %w", err)
		}
		if p.Caption != nil {
			firstLine := strings.SplitN(*p.Caption, "\n", 2)[0]
			firstLine = strings.TrimSpace(firstLine)
			if firstLine != "" {
				p.Title = &firstLine
			}
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate upcoming posts: %w", err)
	}

	// Fetch targets
	if len(posts) > 0 {
		postIDs := make([]uuid.UUID, len(posts))
		for i, p := range posts {
			postIDs[i] = p.ID
		}
		targetRows, err := r.db.Query(ctx, `
			SELECT
				pt.id, pt.post_id, pt.social_account_id, sa.platform, sa.platform_user_id,
				pt.status, pa.platform_post_id, pa.platform_url, pa.error_code,
				pa.error_message, pa.completed_at
			FROM post_targets pt
			JOIN social_accounts sa ON sa.id = pt.social_account_id
			LEFT JOIN publication_attempts pa ON pa.post_target_id = pt.id
			WHERE pt.post_id = ANY($1)
			ORDER BY pt.post_id, pt.created_at ASC
		`, postIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("list upcoming post targets: %w", err)
		}
		defer targetRows.Close()

		postIndex := make(map[uuid.UUID]*model.PostWithDetails, len(posts))
		for i := range posts {
			posts[i].Targets = make([]model.PostTargetDetail, 0)
			postIndex[posts[i].ID] = &posts[i]
		}

		for targetRows.Next() {
			var postID uuid.UUID
			var t model.PostTargetDetail
			if err := targetRows.Scan(
				&t.ID, &postID, &t.SocialAccountID, &t.Platform, &t.PlatformUserID,
				&t.Status, &t.PlatformPostID, &t.PlatformURL, &t.ErrorCode,
				&t.ErrorMessage, &t.PublishedAt,
			); err != nil {
				return nil, 0, fmt.Errorf("scan upcoming target: %w", err)
			}
			if p, ok := postIndex[postID]; ok {
				p.Targets = append(p.Targets, t)
			}
		}
		if err := targetRows.Err(); err != nil {
			return nil, 0, fmt.Errorf("iterate upcoming targets: %w", err)
		}
	}

	return posts, total, nil
}

// ClaimDueScheduledPosts claims scheduled posts whose scheduled_at is due using FOR UPDATE SKIP LOCKED.
// It atomically updates post status to QUEUED and inserts corresponding outbox events.
func (r *PostRepository) ClaimDueScheduledPosts(ctx context.Context, limit int) ([]model.Post, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, user_id, caption, media_url, media_type, media_id, status, content_format,
		       scheduled_at, published_at, scheduling_generation, created_at, updated_at
		FROM posts
		WHERE status = 'SCHEDULED' AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("select due scheduled posts: %w", err)
	}
	defer rows.Close()

	var claimed []model.Post
	for rows.Next() {
		var p model.Post
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Caption, &p.MediaURL, &p.MediaType, &p.MediaID,
			&p.Status, &p.ContentFormat, &p.ScheduledAt, &p.PublishedAt, &p.SchedulingGeneration,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed post: %w", err)
		}
		claimed = append(claimed, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due posts: %w", err)
	}

	if len(claimed) == 0 {
		return nil, nil
	}

	for _, p := range claimed {
		// Update status to QUEUED
		if _, err := tx.Exec(ctx, `
			UPDATE posts
			SET status = 'QUEUED', updated_at = NOW()
			WHERE id = $1
		`, p.ID); err != nil {
			return nil, fmt.Errorf("update post to queued: %w", err)
		}

		// Insert outbox event with post_id and scheduling_generation
		payload, err := json.Marshal(map[string]interface{}{
			"post_id":               p.ID,
			"scheduling_generation": p.SchedulingGeneration,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal outbox payload: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_events (
				aggregate_type, aggregate_id, event_type, payload, status
			)
			VALUES ('post', $1, 'POST_CREATED', $2, 'PENDING')
		`, p.ID, payload); err != nil {
			return nil, fmt.Errorf("insert scheduled outbox event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim transaction: %w", err)
	}

	return claimed, nil
}
