package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
)

type PublishProcessor struct {
	posts         *storage.PostRepository
	publications  publicationStore
	connectors    map[string]connectors.PlatformConnector
	encryptionKey string
	scheduler     publishScheduler
}

type publishScheduler interface {
	Enqueue(*asynq.Task, ...asynq.Option) (string, error)
}

type publicationStore interface {
	GetPostForPublishing(context.Context, uuid.UUID) (*model.Post, error)
	GetPendingPostTargets(context.Context, uuid.UUID) ([]model.PostTarget, error)
	GetPublicationAttempt(context.Context, uuid.UUID) (*model.PublicationAttempt, error)
	GetPostTargetWithSocialAccount(context.Context, uuid.UUID) (*model.PostTarget, *model.SocialAccount, error)
	MarkAttemptProcessing(context.Context, uuid.UUID) error
	MarkAttemptUnknown(context.Context, uuid.UUID, string, string) error
	ResetAttemptForRetry(context.Context, uuid.UUID) error
	MarkAttemptSucceeded(context.Context, uuid.UUID, string, string) error
	MarkAttemptFailed(context.Context, uuid.UUID, string, string) error
	MarkAttemptRetrying(context.Context, uuid.UUID, string, string, string, time.Time) error
	MarkPostTargetPublished(context.Context, uuid.UUID) error
	UpdatePostStatus(context.Context, uuid.UUID, string) error
	RecordPermanentFailure(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *string, string, string, int, string, string, string, string, []byte) error
}

type oauthTokenStore interface {
	UpdateOAuthTokens(context.Context, uuid.UUID, string, *time.Time, *string) error
}

var oauthRefreshLocks sync.Map

func NewPublishProcessor(
	posts *storage.PostRepository,
	publicationsRepo publicationStore,
	platformConnectors map[string]connectors.PlatformConnector,
	encryptionKey string,
	schedulers ...publishScheduler,
) *PublishProcessor {
	var scheduler publishScheduler
	if len(schedulers) > 0 {
		scheduler = schedulers[0]
	}
	return &PublishProcessor{
		posts:         posts,
		publications:  publicationsRepo,
		connectors:    platformConnectors,
		encryptionKey: encryptionKey,
		scheduler:     scheduler,
	}
}

// ProcessPublishJob handles a POST_CREATED event and publishes to all targets
func (p *PublishProcessor) ProcessPublishJob(ctx context.Context, task *asynq.Task) error {
	job, err := jobs.ParsePublishJob(task.Payload())
	if err != nil {
		return fmt.Errorf("parse publish job: %w", err)
	}

	log.Info().
		Str("post_id", job.PostID.String()).
		Msg("Processing publish job")

	// Load post
	post, err := p.publications.GetPostForPublishing(ctx, job.PostID)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	// Check if post is already published
	if post.Status != "QUEUED" {
		log.Info().
			Str("post_id", job.PostID.String()).
			Str("status", post.Status).
			Msg("Post not in QUEUED status, skipping")
		return nil
	}

	// Get pending targets
	targets, err := p.publications.GetPendingPostTargets(ctx, job.PostID)
	if err != nil {
		return fmt.Errorf("get pending targets: %w", err)
	}

	if len(targets) == 0 {
		log.Warn().
			Str("post_id", job.PostID.String()).
			Msg("No pending targets found")
		return nil
	}

	// Publish to each target
	var firstErr error

	for _, target := range targets {
		if err := p.publishToTarget(ctx, post, &target); err != nil {
			log.Error().
				Err(err).
				Str("post_id", job.PostID.String()).
				Str("target_id", target.ID.String()).
				Msg("Failed to publish to target")
			if firstErr == nil {
				firstErr = err
			}
			if publishClassification(err) == connectors.ErrorRetryable {
				if scheduleErr := p.scheduleRetry(ctx, job.PostID, target.ID, task, err); scheduleErr != nil && firstErr == err {
					firstErr = scheduleErr
				}
			} else if publishClassification(err) == connectors.ErrorAmbiguous {
				if scheduleErr := p.scheduleReconciliation(ctx, job.PostID, target.ID); scheduleErr != nil && firstErr == err {
					firstErr = scheduleErr
				}
			}
		}
	}

	if firstErr != nil {
		// Blink schedules business retries explicitly; returning nil prevents
		// Asynq from creating a second retry sequence.
		return nil
	}

	if err := p.updatePostStatus(ctx, job.PostID, "PUBLISHED"); err != nil {
		log.Error().Err(err).Msg("Failed to update post status to PUBLISHED")
	}

	return nil
}

func (p *PublishProcessor) publishToTarget(
	ctx context.Context,
	post *model.Post,
	target *model.PostTarget,
) error {
	// Get publication attempt
	attempt, err := p.publications.GetPublicationAttempt(ctx, target.ID)
	if err != nil {
		return fmt.Errorf("get publication attempt: %w", err)
	}
	if attempt.Status == "SUCCEEDED" {
		return p.publications.MarkPostTargetPublished(ctx, target.ID)
	}
	if attempt.AttemptCount >= MaxAttempts && attempt.Status != "PROCESSING" && attempt.Status != "UNKNOWN" {
		return newPublishError(connectors.ErrorPermanent, errors.New("maximum publication attempts reached"))
	}

	recoveryRequired := attempt.Status == "PROCESSING" || attempt.Status == "UNKNOWN"
	if attempt.Status == "PENDING" || attempt.Status == "RETRYING" || attempt.Status == "FAILED" {
		if err := p.publications.MarkAttemptProcessing(ctx, attempt.ID); err != nil {
			if err == storage.ErrAttemptNotClaimed {
				return fmt.Errorf("attempt claim lost; retry job")
			}
			return fmt.Errorf("mark attempt processing: %w", err)
		}
		attempt.Status = "PROCESSING"
	}

	// Get target with social account
	_, account, err := p.publications.GetPostTargetWithSocialAccount(ctx, target.ID)
	if err != nil {
		return fmt.Errorf("get social account: %w", err)
	}

	accessToken, err := auth.DecryptToken(account.AccessToken, p.encryptionKey)
	if err != nil {
		if markErr := p.publications.MarkAttemptFailed(ctx, attempt.ID, "DECRYPTION_FAILED", err.Error()); markErr != nil {
			log.Error().Err(markErr).Msg("Failed to mark attempt failed")
		}
		return newPublishError(connectors.ErrorPermanent, connectors.ErrTokenDecryptFailed)
	}

	// Get connector for platform
	connector, ok := p.connectors[account.Platform]
	if !ok {
		errMsg := fmt.Sprintf("no connector for platform: %s", account.Platform)
		if err := p.publications.MarkAttemptFailed(ctx, attempt.ID, "UNKNOWN_PLATFORM", errMsg); err != nil {
			log.Error().Err(err).Msg("Failed to mark attempt failed")
		}
		return newPublishError(connectors.ErrorPermanent, errors.New(errMsg))
	}

	accessToken, refreshed, err := p.ensureAccessToken(ctx, account, connector)
	if err != nil {
		_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "TOKEN_REFRESH_FAILED", err.Error())
		return newPublishError(connectors.ClassifyError(err), err)
	}

	// Publish
	caption := ""
	if post.Caption != nil {
		caption = *post.Caption
	}

	if attempt.Status == "SUCCEEDED" {
		return p.publications.MarkPostTargetPublished(ctx, target.ID)
	}

	// PROCESSING and UNKNOWN both mean the previous external call may have
	// been accepted. Reconcile before allowing another publish request.
	if recoveryRequired {
		log.Info().Str("attempt_id", attempt.ID.String()).Msg("Reconciling ambiguous publication attempt")

		reconciliation, recErr := connector.ReconcilePublish(ctx, accessToken, account.PlatformUserID, attempt.ID.String(), caption)
		if recErr != nil {
			_ = p.publications.MarkAttemptUnknown(ctx, attempt.ID, "RECONCILIATION_FAILED", recErr.Error())
			return fmt.Errorf("reconciliation error: %w", recErr)
		}

		switch reconciliation.Outcome {
		case connectors.ReconciliationFound:
			if err := p.publications.MarkAttemptSucceeded(ctx, attempt.ID, reconciliation.PlatformPostID, reconciliation.PublicURL); err != nil {
				return fmt.Errorf("mark reconciled attempt succeeded: %w", err)
			}
			if err := p.publications.MarkPostTargetPublished(ctx, target.ID); err != nil {
				return fmt.Errorf("mark target published: %w", err)
			}
			return nil
		case connectors.ReconciliationUnknown:
			_ = p.publications.MarkAttemptUnknown(ctx, attempt.ID, "RECONCILIATION_UNKNOWN", "platform could not prove whether the publication exists")
			return fmt.Errorf("publication outcome remains unknown")
		case connectors.ReconciliationNotFoundConfirmed:
			if err := p.publications.ResetAttemptForRetry(ctx, attempt.ID); err != nil {
				return fmt.Errorf("reset attempt after reconciliation: %w", err)
			}
			attempt.Status = "PENDING"
		default:
			return fmt.Errorf("unsupported reconciliation outcome: %q", reconciliation.Outcome)
		}
	}

	if attempt.Status == "PENDING" {
		if err := p.publications.MarkAttemptProcessing(ctx, attempt.ID); err != nil {
			if err == storage.ErrAttemptNotClaimed {
				return fmt.Errorf("attempt claim lost; retry reconciliation")
			}
			return fmt.Errorf("mark attempt processing: %w", err)
		}
	}

	input := model.PublishInput{
		Caption:   caption,
		MediaURL:  post.MediaURL,
		MediaType: post.MediaType,
	}

	var mediaIDs []string
	if input.MediaURL != nil && *input.MediaURL != "" {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, *input.MediaURL, nil)
		if reqErr != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("create download request: %w", reqErr)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

		resp, respErr := http.DefaultClient.Do(req)
		if respErr != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("download media: %w", respErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("download media failed: %d", resp.StatusCode)
		}

		mediaType := "application/octet-stream"
		if input.MediaType != nil {
			mediaType = *input.MediaType
		}

		tempFile, err := os.CreateTemp("", "upload-*.tmp")
		if err != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()

		if _, err := io.Copy(tempFile, resp.Body); err != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("copy to temp file: %w", err)
		}

		if _, err := tempFile.Seek(0, 0); err != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("seek temp file: %w", err)
		}

		mediaID, uploadErr := connector.UploadMedia(ctx, accessToken, attempt.ID.String(), tempFile, mediaType)
		if errors.Is(uploadErr, connectors.ErrAuthFailed) && !refreshed {
			if _, seekErr := tempFile.Seek(0, 0); seekErr != nil {
				return newPublishError(connectors.ErrorPermanent, fmt.Errorf("rewind media for auth retry: %w", seekErr))
			}
			var refreshErr error
			accessToken, refreshed, refreshErr = p.refreshAccessToken(ctx, account, connector)
			if refreshErr != nil {
				_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "TOKEN_REFRESH_FAILED", refreshErr.Error())
				return newPublishError(connectors.ClassifyError(refreshErr), refreshErr)
			}
			mediaID, uploadErr = connector.UploadMedia(ctx, accessToken, attempt.ID.String(), tempFile, mediaType)
		}
		if uploadErr != nil {
			classification := publishClassification(uploadErr)
			if classification == connectors.ErrorAmbiguous {
				_ = p.publications.MarkAttemptUnknown(ctx, attempt.ID, "UPLOAD_OUTCOME_UNKNOWN", uploadErr.Error())
			} else if classification == connectors.ErrorPermanent {
				_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "UPLOAD_FAILED", uploadErr.Error())
			}
			return newPublishError(classification, fmt.Errorf("upload media to platform: %w", uploadErr))
		}
		mediaIDs = append(mediaIDs, mediaID)
	}

	publicURL, platformPostID, err := connector.Publish(ctx, accessToken, attempt.ID.String(), input.Caption, mediaIDs...)
	if errors.Is(err, connectors.ErrAuthFailed) && !refreshed {
		accessToken, _, refreshErr := p.refreshAccessToken(ctx, account, connector)
		if refreshErr != nil {
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "TOKEN_REFRESH_FAILED", refreshErr.Error())
			return newPublishError(connectors.ClassifyError(refreshErr), refreshErr)
		}
		publicURL, platformPostID, err = connector.Publish(ctx, accessToken, attempt.ID.String(), input.Caption, mediaIDs...)
	}
	if err != nil {
		classification := publishClassification(err)
		if classification == connectors.ErrorAmbiguous {
			_ = p.publications.MarkAttemptUnknown(ctx, attempt.ID, "PUBLISH_OUTCOME_UNKNOWN", err.Error())
		} else if classification == connectors.ErrorPermanent {
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "PUBLISH_FAILED", err.Error())
		}
		return newPublishError(classification, fmt.Errorf("publish to platform: %w", err))
	}

	// Mark as succeeded
	if err := p.publications.MarkAttemptSucceeded(ctx, attempt.ID, platformPostID, publicURL); err != nil {
		return fmt.Errorf("mark attempt succeeded: %w", err)
	}

	// Mark target as published
	if err := p.publications.MarkPostTargetPublished(ctx, target.ID); err != nil {
		return fmt.Errorf("mark target published: %w", err)
	}

	return nil
}

func (p *PublishProcessor) ensureAccessToken(ctx context.Context, account *model.SocialAccount, connector connectors.PlatformConnector) (string, bool, error) {
	accessToken, err := auth.DecryptToken(account.AccessToken, p.encryptionKey)
	if err != nil {
		return "", false, connectors.ErrTokenDecryptFailed
	}
	if !connectors.TokenNeedsRefresh(account.ExpiresAt, time.Now(), connectors.DefaultRefreshSkew) {
		return accessToken, false, nil
	}
	return p.refreshAccessToken(ctx, account, connector)
}

func (p *PublishProcessor) refreshAccessToken(ctx context.Context, account *model.SocialAccount, connector connectors.PlatformConnector) (string, bool, error) {
	lockValue, _ := oauthRefreshLocks.LoadOrStore(account.ID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	refresher, ok := connector.(connectors.OAuthTokenRefresher)
	if !ok || account.RefreshToken == nil || *account.RefreshToken == "" {
		return "", false, connectors.ErrInvalidRefreshToken
	}
	refreshToken, err := auth.DecryptToken(*account.RefreshToken, p.encryptionKey)
	if err != nil {
		return "", false, connectors.ErrTokenDecryptFailed
	}
	result, err := refresher.RefreshToken(ctx, refreshToken)
	if err != nil {
		return "", false, err
	}
	if result.AccessToken == "" {
		return "", false, connectors.ErrTokenRefreshFailed
	}
	encryptedAccessToken, err := auth.EncryptToken(result.AccessToken, p.encryptionKey)
	if err != nil {
		return "", false, fmt.Errorf("%w: encrypt access token", connectors.ErrTokenPersistence)
	}
	var encryptedRefreshToken *string
	if result.RefreshToken != nil {
		encrypted, encryptErr := auth.EncryptToken(*result.RefreshToken, p.encryptionKey)
		if encryptErr != nil {
			return "", false, fmt.Errorf("%w: encrypt refresh token", connectors.ErrTokenPersistence)
		}
		encryptedRefreshToken = &encrypted
	}
	store, ok := p.publications.(oauthTokenStore)
	if !ok {
		return "", false, connectors.ErrTokenPersistence
	}
	if err := store.UpdateOAuthTokens(ctx, account.ID, encryptedAccessToken, result.ExpiresAt, encryptedRefreshToken); err != nil {
		return "", false, fmt.Errorf("%w: %v", connectors.ErrTokenPersistence, err)
	}
	account.AccessToken = encryptedAccessToken
	if encryptedRefreshToken != nil {
		account.RefreshToken = encryptedRefreshToken
	}
	if result.ExpiresAt != nil {
		account.ExpiresAt = result.ExpiresAt
	}
	return result.AccessToken, true, nil
}

func (p *PublishProcessor) scheduleRetry(ctx context.Context, postID, targetID uuid.UUID, task *asynq.Task, cause error) error {
	attempt, err := p.publications.GetPublicationAttempt(ctx, targetID)
	if err != nil {
		return err
	}
	if !retryPolicy.ShouldRetry(cause, attempt.AttemptCount) {
		return p.recordPermanentFailure(ctx, postID, targetID, task, attempt, cause)
	}
	if p.scheduler == nil {
		return fmt.Errorf("retry scheduler is not configured")
	}
	delay := retryPolicy.NextRetryDelay(attempt.AttemptCount)
	nextRetryAt := time.Now().Add(delay)
	if err := p.publications.MarkAttemptRetrying(ctx, attempt.ID, "PUBLISH_RETRY", string(publishClassification(cause)), cause.Error(), nextRetryAt); err != nil {
		return err
	}
	retryTask, err := jobs.NewPublishTask(postID)
	if err != nil {
		return err
	}
	if _, err := p.scheduler.Enqueue(retryTask, asynq.ProcessIn(delay), asynq.MaxRetry(0), asynq.Timeout(5*time.Minute)); err != nil {
		return fmt.Errorf("enqueue publish retry: %w", err)
	}
	log.Info().
		Str("publication_attempt_id", attempt.ID.String()).
		Str("post_id", postID.String()).
		Str("target_id", targetID.String()).
		Int("attempt_count", attempt.AttemptCount).
		Int("max_attempts", MaxAttempts).
		Str("error_class", string(publishClassification(cause))).
		Dur("retry_delay", delay).
		Time("next_retry_at", nextRetryAt).
		Msg("scheduling publish retry")
	return nil
}

func (p *PublishProcessor) recordPermanentFailure(ctx context.Context, postID, targetID uuid.UUID, task *asynq.Task, attempt *model.PublicationAttempt, cause error) error {
	_, account, err := p.publications.GetPostTargetWithSocialAccount(ctx, targetID)
	if err != nil {
		return fmt.Errorf("get failed target platform: %w", err)
	}
	failureType, code, reason, response := failureDetails(cause)
	var jobID *string
	if id, ok := asynq.GetTaskID(ctx); ok && id != "" {
		jobID = &id
	}
	if err := p.publications.RecordPermanentFailure(ctx, attempt.ID, postID, targetID, jobID, task.Type(), account.Platform, MaxAttempts, failureType, string(publishClassification(cause)), code, reason, response); err != nil {
		log.Error().Err(err).Str("post_id", postID.String()).Str("target_id", targetID.String()).Int("attempt", attempt.AttemptCount).Int("max_attempts", MaxAttempts).Str("failure_type", failureType).Msg("failed to persist permanent publish failure")
		return err
	}
	log.Error().Str("post_id", postID.String()).Str("target_id", targetID.String()).Int("attempt", attempt.AttemptCount).Int("max_attempts", MaxAttempts).Str("failure_type", failureType).Msg("publish job permanently failed")
	return nil
}

var sensitiveValuePattern = regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|client[_-]?secret|authorization[_-]?code|cookie)(\s*[:=]\s*)([^\s,;]+)`)

func failureDetails(cause error) (string, string, string, []byte) {
	classification := publishClassification(cause)
	failureType := "UNKNOWN"
	switch classification {
	case connectors.ErrorPermanent:
		failureType = "PLATFORM_ERROR"
	case connectors.ErrorRetryable:
		failureType = "PLATFORM_ERROR"
	case connectors.ErrorAmbiguous:
		failureType = "UNKNOWN"
	}
	code := "PUBLISH_FAILED"
	statusCode := 0
	var classified *connectors.ClassifiedError
	if errors.As(cause, &classified) {
		if classified.Code != "" {
			code = classified.Code
		}
		statusCode = classified.StatusCode
		if strings.Contains(strings.ToUpper(classified.Code), "AUTH") || strings.Contains(strings.ToUpper(classified.Code), "TOKEN") {
			failureType = "AUTHENTICATION"
		}
	}
	reason := redactSensitive(cause.Error())
	metadata := map[string]any{"code": code, "classification": string(classification), "message": reason}
	if statusCode != 0 {
		metadata["status_code"] = statusCode
	}
	response, _ := json.Marshal(metadata)
	return failureType, code, reason, response
}

func redactSensitive(value string) string {
	return sensitiveValuePattern.ReplaceAllString(value, `${1}$2[REDACTED]`)
}

func (p *PublishProcessor) scheduleReconciliation(ctx context.Context, postID, targetID uuid.UUID) error {
	if p.scheduler == nil {
		return fmt.Errorf("reconciliation scheduler is not configured")
	}
	attempt, err := p.publications.GetPublicationAttempt(ctx, targetID)
	if err != nil {
		return err
	}
	if attempt.Status != "UNKNOWN" || attempt.ErrorCode == nil || !strings.HasSuffix(*attempt.ErrorCode, "OUTCOME_UNKNOWN") {
		return nil
	}
	task, err := jobs.NewPublishTask(postID)
	if err != nil {
		return err
	}
	if _, err := p.scheduler.Enqueue(task, asynq.ProcessIn(0), asynq.MaxRetry(0), asynq.Timeout(5*time.Minute)); err != nil {
		return fmt.Errorf("enqueue reconciliation: %w", err)
	}
	log.Info().Str("publication_attempt_id", attempt.ID.String()).Str("post_id", postID.String()).Msg("scheduling publication reconciliation")
	return nil
}

func (p *PublishProcessor) updatePostStatus(ctx context.Context, postID uuid.UUID, status string) error {
	if err := p.publications.UpdatePostStatus(ctx, postID, status); err != nil {
		return fmt.Errorf("update post status: %w", err)
	}

	log.Info().
		Str("post_id", postID.String()).
		Str("status", status).
		Msg("Post status updated")
	return nil
}
