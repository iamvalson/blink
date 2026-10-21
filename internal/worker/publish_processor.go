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
	logger "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/metrics"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

var mediaDownloadClient = &http.Client{Timeout: 5 * time.Minute}

type publicationLocker interface {
	AcquirePublishLock(
		context.Context,
		uuid.UUID,
	) (*storage.PublicationRepository, func(), bool, error)
}

type publicationExecutionStore interface {
	publicationStore
	ClaimPostPublishing(context.Context, uuid.UUID, int) (bool, error)
	RecomputePostStatus(context.Context, uuid.UUID) error
}

type acquirePublishLock func(context.Context, uuid.UUID) (publicationExecutionStore, func(), bool, error)

type mediaStore interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

type mediaRepository interface {
	GetMediaByID(ctx context.Context, id uuid.UUID) (*model.Media, error)
}

type PublishProcessor struct {
	posts         *storage.PostRepository
	publications  publicationStore
	acquire       acquirePublishLock
	connectors    map[string]connectors.PlatformConnector
	encryptionKey string
	scheduler     publishScheduler
	mediaRepo     mediaRepository
	mediaStore    mediaStore
}

func (p *PublishProcessor) WithMediaStore(repo mediaRepository, store mediaStore) *PublishProcessor {
	p.mediaRepo = repo
	p.mediaStore = store
	return p
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
	RecordPermanentFailure(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *string, string, string, int, string, string, string, string, []byte) error
}

type oauthTokenStore interface {
	UpdateOAuthTokens(context.Context, uuid.UUID, string, *time.Time, *string) error
}

var oauthRefreshLocks sync.Map

func getTaskID(ctx context.Context) string {
	if id, ok := asynq.GetTaskID(ctx); ok {
		return id
	}
	return ""
}

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
	var acquire acquirePublishLock
	if locker, ok := publicationsRepo.(publicationLocker); ok {
		acquire = func(ctx context.Context, id uuid.UUID) (publicationExecutionStore, func(), bool, error) {
			return locker.AcquirePublishLock(ctx, id)
		}
	}
	return &PublishProcessor{
		acquire:       acquire,
		posts:         posts,
		publications:  publicationsRepo,
		connectors:    platformConnectors,
		encryptionKey: encryptionKey,
		scheduler:     scheduler,
	}
}

// ProcessPublishJob handles a POST_CREATED event and publishes to all targets
func (p *PublishProcessor) ProcessPublishJob(
	ctx context.Context,
	task *asynq.Task,
) error {
	job, err := jobs.ParsePublishJob(task.Payload())
	if err != nil {
		return fmt.Errorf("%w: invalid publish payload: %v", asynq.SkipRetry, err)
	}
	if job.SchedulingGeneration < 1 {
		// The database recovery sweep will create a current-generation task.
		return nil
	}

	if p.acquire == nil {
		return errors.New("publication repository does not support worker locking")
	}

	repo, release, acquired, err := p.acquire(ctx, job.PostID)
	if err != nil {
		return err
	}
	if !acquired {
		// Another worker currently owns this post. Infrastructure retry
		// will revisit this delivery.
		return errors.New("post is already being processed")
	}
	defer release()

	claimed, err := repo.ClaimPostPublishing(
		ctx, job.PostID, job.SchedulingGeneration,
	)
	if err != nil {
		return err
	}
	if !claimed {
		return nil // Cancelled, stale, terminal, or not due.
	}

	// Do not modify the shared processor: workers run concurrently.
	processor := *p
	processor.publications = repo
	// Use the connection already held by this job for persisted media reads.
	// Otherwise all pool connections can be held by workers waiting for media.
	if mediaRepo, ok := repo.(mediaRepository); ok && processor.mediaRepo != nil {
		processor.mediaRepo = mediaRepo
	}

	post, err := repo.GetPostForPublishing(ctx, job.PostID)
	if err != nil {
		return err
	}

	targets, err := repo.GetPendingPostTargets(ctx, post.ID)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if job.TargetID != nil && target.ID != *job.TargetID {
			continue
		}

		attempt, err := repo.GetPublicationAttempt(ctx, target.ID)
		if err != nil {
			return err
		}

		if attempt.ErrorCode != nil &&
			*attempt.ErrorCode == "MANUAL_REVIEW_REQUIRED" {
			continue
		}

		if attempt.Status == "RETRYING" &&
			attempt.NextRetryAt != nil &&
			attempt.NextRetryAt.After(time.Now()) {
			continue
		}

		platform, publishErr := processor.processTarget(ctx, post, &target)
		if publishErr == nil {
			metrics.JobSucceeded(platform)
			continue
		}

		metrics.JobFailed(platform)
		switch publishClassification(publishErr) {
		case connectors.ErrorRetryable:
			if err := processor.scheduleRetry(
				ctx, post.ID, target.ID, task, publishErr,
			); err != nil {
				return errors.Join(publishErr, err)
			}

		case connectors.ErrorPermanent:
			if err := processor.recordPermanentFailure(
				ctx, post.ID, target.ID, task, nil, publishErr,
			); err != nil {
				return errors.Join(publishErr, err)
			}

		case connectors.ErrorAmbiguous:
			if err := processor.scheduleReconciliation(
				ctx, post.ID, target.ID,
			); err != nil {
				return errors.Join(publishErr, err)
			}

		default:
			return publishErr
		}
	}

	return repo.RecomputePostStatus(ctx, post.ID)
}

func (p *PublishProcessor) processTarget(ctx context.Context, post *model.Post, target *model.PostTarget) (string, error) {
	platform := metrics.PlatformUnknown
	if _, account, err := p.publications.GetPostTargetWithSocialAccount(ctx, target.ID); err == nil {
		platform = metrics.NormalizePlatform(account.Platform)
	}
	metrics.JobProcessed(platform)
	metrics.WorkerJobStarted()
	defer metrics.WorkerJobFinished()
	return platform, p.publishToTarget(ctx, post, target)
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
	contextLogger := logger.WithJobContext(getTaskID(ctx), post.ID.String(), post.UserID.String(), account.Platform, attempt.AttemptCount)
	platform := metrics.NormalizePlatform(account.Platform)

	if _, err = auth.DecryptToken(account.AccessToken, p.encryptionKey); err != nil {
		if recoveryRequired {
			return p.requireManualReview(ctx, attempt.ID, "Credentials could not be decrypted for reconciliation")
		}
		if markErr := p.publications.MarkAttemptFailed(ctx, attempt.ID, "DECRYPTION_FAILED", err.Error()); markErr != nil {
			contextLogger.Error().Err(logger.RedactError(markErr)).Str("event", "publish_failed").Str("status", "failed").Msg("failed to mark attempt failed")
		}
		return newPublishError(connectors.ErrorPermanent, connectors.ErrTokenDecryptFailed)
	}

	// Get connector for platform
	connector, ok := p.connectors[account.Platform]
	if !ok {
		errMsg := fmt.Sprintf("no connector for platform: %s", account.Platform)
		if recoveryRequired {
			return p.requireManualReview(ctx, attempt.ID, errMsg)
		}
		if err := p.publications.MarkAttemptFailed(ctx, attempt.ID, "UNKNOWN_PLATFORM", errMsg); err != nil {
			contextLogger.Error().Err(logger.RedactError(err)).Str("event", "publish_failed").Str("status", "failed").Msg("failed to mark attempt failed")
		}
		return newPublishError(connectors.ErrorPermanent, errors.New(errMsg))
	}

	accessToken, refreshed, err := p.ensureAccessToken(ctx, account, connector)
	if err != nil {
		if recoveryRequired {
			return p.requireManualReview(ctx, attempt.ID, "Credentials unavailable for reconciliation: "+redactSensitive(err.Error()))
		}
		_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "TOKEN_REFRESH_FAILED", err.Error())
		return newPublishError(connectors.ClassifyError(err), err)
	}

	// Publish
	caption := ""
	if post.Caption != nil {
		caption = *post.Caption
	}
	start := time.Now()
	contextLogger.Info().Str("event", "publish_started").Str("status", "started").Msg("publishing to platform")

	if attempt.Status == "SUCCEEDED" {
		return p.publications.MarkPostTargetPublished(ctx, target.ID)
	}

	// PROCESSING and UNKNOWN both mean the previous external call may have
	// been accepted. Reconcile before allowing another publish request.
	if recoveryRequired {
		reconciliation, recErr := connector.ReconcilePublish(
			ctx,
			accessToken,
			account.PlatformUserID,
			attempt.ID.String(),
			caption,
		)

		if recErr == nil &&
			reconciliation.Outcome == connectors.ReconciliationFound {
			if err := p.publications.MarkAttemptSucceeded(
				ctx,
				attempt.ID,
				reconciliation.PlatformPostID,
				reconciliation.PublicURL,
			); err != nil {
				return fmt.Errorf("persist reconciled success: %w", err)
			}

			return p.publications.MarkPostTargetPublished(ctx, target.ID)
		}

		reason := "Publication could not be confirmed. Review the platform before retrying."
		if recErr != nil {
			reason = "Reconciliation failed: " + redactSensitive(recErr.Error())
		}

		return p.requireManualReview(ctx, attempt.ID, reason)
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
		Caption:       caption,
		MediaURL:      post.MediaURL,
		MediaType:     post.MediaType,
		ContentFormat: post.ContentFormat,
	}

	var mediaIDs []string
	if post.MediaID != nil && p.mediaRepo != nil && p.mediaStore != nil {
		mediaRecord, err := p.mediaRepo.GetMediaByID(ctx, *post.MediaID)
		if err != nil {
			errMsg := fmt.Sprintf("persisted media not found: %s", *post.MediaID)
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "MEDIA_NOT_FOUND", errMsg)
			return newPublishError(connectors.ErrorPermanent, errors.New(errMsg))
		}
		if mediaRecord.UploadStatus != "complete" {
			errMsg := fmt.Sprintf("media upload incomplete: %s (status: %s)", *post.MediaID, mediaRecord.UploadStatus)
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "MEDIA_INCOMPLETE", errMsg)
			return newPublishError(connectors.ErrorPermanent, errors.New(errMsg))
		}

		mediaReader, err := p.mediaStore.Get(ctx, mediaRecord.StorageKey)
		if err != nil {
			if errors.Is(err, mediastore.ErrNotFound) {
				errMsg := fmt.Sprintf("persisted media not found in storage: %s", mediaRecord.StorageKey)
				_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "MEDIA_NOT_FOUND", errMsg)
				return newPublishError(connectors.ErrorPermanent, errors.New(errMsg))
			}
			errMsg := fmt.Sprintf("storage error reading media %s: %v", mediaRecord.StorageKey, err)
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return newPublishError(connectors.ErrorRetryable, errors.New(errMsg))
		}
		defer mediaReader.Close()

		tempFile, err := os.CreateTemp("", "upload-*.tmp")
		if err != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()

		if _, err := io.Copy(tempFile, mediaReader); err != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("copy media to temp file: %w", err)
		}

		if _, err := tempFile.Seek(0, 0); err != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("seek temp file: %w", err)
		}

		mediaID, uploadErr := connector.UploadMedia(ctx, accessToken, attempt.ID.String(), tempFile, mediaRecord.ContentType)
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
			mediaID, uploadErr = connector.UploadMedia(ctx, accessToken, attempt.ID.String(), tempFile, mediaRecord.ContentType)
		}
		if uploadErr != nil {
			if rateLimitErr, ok := extractRateLimitError(uploadErr); ok {
				metrics.RateLimitEvent(rateLimitErr.Platform)
			}
			classification := publishClassification(uploadErr)
			if classification == connectors.ErrorAmbiguous {
				_ = p.publications.MarkAttemptUnknown(ctx, attempt.ID, "UPLOAD_OUTCOME_UNKNOWN", uploadErr.Error())
			} else if classification == connectors.ErrorPermanent {
				_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "UPLOAD_FAILED", uploadErr.Error())
			}
			return newPublishError(classification, fmt.Errorf("upload media to platform: %w", uploadErr))
		}
		mediaIDs = append(mediaIDs, mediaID)
	} else if input.MediaURL != nil && *input.MediaURL != "" {
		if strings.HasPrefix(*input.MediaURL, "blob:") {
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "INVALID_MEDIA_URL", "browser blob: URL cannot be used for remote publishing")
			return newPublishError(connectors.ErrorPermanent, errors.New("browser blob: URL cannot be used for remote publishing"))
		}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, *input.MediaURL, nil)
		if reqErr != nil {
			_ = p.publications.ResetAttemptForRetry(ctx, attempt.ID)
			return fmt.Errorf("create download request: %w", reqErr)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

		resp, respErr := mediaDownloadClient.Do(req)
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
			if rateLimitErr, ok := extractRateLimitError(uploadErr); ok {
				metrics.RateLimitEvent(rateLimitErr.Platform)
			}
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

	// Pre-publication prerequisite check: YouTube requires a video!
	if account.Platform == "youtube" && len(mediaIDs) == 0 {
		errMsg := "YouTube publishing requires a video"
		_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "MEDIA_REQUIRED", errMsg)
		contextLogger.Error().Str("event", "publish_failed").Str("status", "failed").Msg(errMsg)
		return newPublishError(connectors.ErrorPermanent, connectors.NewClassifiedError(connectors.ErrorPermanent, "MEDIA_REQUIRED", errors.New(errMsg)))
	}

	publishStart := time.Now()
	publicURL, platformPostID, err := connector.Publish(ctx, accessToken, attempt.ID.String(), input.Caption, mediaIDs...)
	metrics.ObservePublishDuration(platform, time.Since(publishStart).Seconds())
	if errors.Is(err, connectors.ErrAuthFailed) && !refreshed {
		accessToken, _, refreshErr := p.refreshAccessToken(ctx, account, connector)
		if refreshErr != nil {
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "TOKEN_REFRESH_FAILED", refreshErr.Error())
			return newPublishError(connectors.ClassifyError(refreshErr), refreshErr)
		}
		publishStart = time.Now()
		publicURL, platformPostID, err = connector.Publish(ctx, accessToken, attempt.ID.String(), input.Caption, mediaIDs...)
		metrics.ObservePublishDuration(platform, time.Since(publishStart).Seconds())
	}
	if err != nil {
		classification := publishClassification(err)
		if rateLimitErr, ok := extractRateLimitError(err); ok {
			metrics.RateLimitEvent(rateLimitErr.Platform)
		}
		if classification == connectors.ErrorAmbiguous {
			_ = p.publications.MarkAttemptUnknown(ctx, attempt.ID, "PUBLISH_OUTCOME_UNKNOWN", err.Error())
		} else if classification == connectors.ErrorPermanent {
			_ = p.publications.MarkAttemptFailed(ctx, attempt.ID, "PUBLISH_FAILED", err.Error())
		}
		contextLogger.Error().Err(logger.RedactError(err)).Str("event", "publish_failed").Str("status", "failed").Dur("duration", time.Since(start)).Msg("publish failed")
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

	contextLogger.Info().Str("event", "publish_succeeded").Str("status", "success").Dur("duration", time.Since(start)).Str("platform_post_id", platformPostID).Str("public_url", publicURL).Msg("publish succeeded")
	return nil
}

// A credential or reconciliation failure cannot prove that an earlier external
// call failed. Keep the outcome unknown so manual post retry cannot duplicate it.
func (p *PublishProcessor) requireManualReview(ctx context.Context, attemptID uuid.UUID, reason string) error {
	if err := p.publications.MarkAttemptUnknown(ctx, attemptID, "MANUAL_REVIEW_REQUIRED", reason); err != nil {
		return err
	}
	return newPublishError(connectors.ErrorAmbiguous, errors.New(reason))
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

	refreshLogger := logger.WithJobContext("", "", account.UserID.String(), account.Platform, 0)
	refreshLogger.Info().Str("event", "oauth_refresh_started").Str("status", "started").Msg("oauth refresh started")
	start := time.Now()

	refresher, ok := connector.(connectors.OAuthTokenRefresher)
	if !ok || account.RefreshToken == nil || *account.RefreshToken == "" {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(connectors.ErrInvalidRefreshToken)).Msg("oauth refresh failed")
		return "", false, connectors.ErrInvalidRefreshToken
	}
	refreshToken, err := auth.DecryptToken(*account.RefreshToken, p.encryptionKey)
	if err != nil {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(err)).Msg("oauth refresh failed")
		return "", false, connectors.ErrTokenDecryptFailed
	}
	result, err := refresher.RefreshToken(ctx, refreshToken)
	refreshResult := "failure"
	defer func() {
		metrics.TokenRefresh(account.Platform, refreshResult)
	}()
	if err != nil {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(err)).Msg("oauth refresh failed")
		return "", false, err
	}
	if result.AccessToken == "" {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(connectors.ErrTokenRefreshFailed)).Msg("oauth refresh failed")
		return "", false, connectors.ErrTokenRefreshFailed
	}
	encryptedAccessToken, err := auth.EncryptToken(result.AccessToken, p.encryptionKey)
	if err != nil {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(fmt.Errorf("%w: encrypt access token", connectors.ErrTokenPersistence))).Msg("oauth refresh failed")
		return "", false, fmt.Errorf("%w: encrypt access token", connectors.ErrTokenPersistence)
	}
	var encryptedRefreshToken *string
	if result.RefreshToken != nil {
		encrypted, encryptErr := auth.EncryptToken(*result.RefreshToken, p.encryptionKey)
		if encryptErr != nil {
			refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(fmt.Errorf("%w: encrypt refresh token", connectors.ErrTokenPersistence))).Msg("oauth refresh failed")
			return "", false, fmt.Errorf("%w: encrypt refresh token", connectors.ErrTokenPersistence)
		}
		encryptedRefreshToken = &encrypted
	}
	store, ok := p.publications.(oauthTokenStore)
	if !ok {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(connectors.ErrTokenPersistence)).Msg("oauth refresh failed")
		return "", false, connectors.ErrTokenPersistence
	}
	if err := store.UpdateOAuthTokens(ctx, account.ID, encryptedAccessToken, result.ExpiresAt, encryptedRefreshToken); err != nil {
		refreshLogger.Error().Str("event", "oauth_refresh_failed").Str("status", "failed").Dur("duration", time.Since(start)).Err(logger.RedactError(fmt.Errorf("%w: %v", connectors.ErrTokenPersistence, err))).Msg("oauth refresh failed")
		return "", false, fmt.Errorf("%w: %v", connectors.ErrTokenPersistence, err)
	}
	account.AccessToken = encryptedAccessToken
	if encryptedRefreshToken != nil {
		account.RefreshToken = encryptedRefreshToken
	}
	if result.ExpiresAt != nil {
		account.ExpiresAt = result.ExpiresAt
	}
	refreshResult = "success"
	refreshLogger.Info().Str("event", "oauth_refresh_succeeded").Str("status", "success").Dur("duration", time.Since(start)).Msg("oauth refresh succeeded")
	return result.AccessToken, true, nil
}

func (p *PublishProcessor) scheduleRetry(
	ctx context.Context,
	postID, targetID uuid.UUID,
	task *asynq.Task,
	cause error,
) error {
	attempt, err := p.publications.GetPublicationAttempt(ctx, targetID)
	if err != nil {
		return err
	}

	if !retryPolicy.ShouldRetry(cause, attempt.AttemptCount) {
		return p.recordPermanentFailure(
			ctx, postID, targetID, task, attempt, cause,
		)
	}

	delay := retryPolicy.ResolveRetryDelay(cause, attempt.AttemptCount)

	// This method now persists the retry AND its outbox event atomically.
	return p.publications.MarkAttemptRetrying(
		ctx,
		attempt.ID,
		"PUBLISH_RETRY",
		string(publishClassification(cause)),
		redactSensitive(cause.Error()),
		time.Now().Add(delay),
	)
}

func (p *PublishProcessor) recordPermanentFailure(ctx context.Context, postID, targetID uuid.UUID, task *asynq.Task, attempt *model.PublicationAttempt, cause error) error {
	if attempt == nil {
		var getErr error
		attempt, getErr = p.publications.GetPublicationAttempt(ctx, targetID)
		if getErr != nil {
			return getErr
		}
	}
	_, account, err := p.publications.GetPostTargetWithSocialAccount(ctx, targetID)
	if err != nil {
		return fmt.Errorf("get failed target platform: %w", err)
	}
	failureType, code, reason, response := failureDetails(cause)
	var jobID *string
	if id, ok := asynq.GetTaskID(ctx); ok && id != "" {
		jobID = &id
	}
	taskType := "post:publish"
	if task != nil {
		taskType = task.Type()
	}
	contextLogger := logger.WithJobContext(getTaskID(ctx), postID.String(), "", account.Platform, attempt.AttemptCount)
	if err := p.publications.RecordPermanentFailure(ctx, attempt.ID, postID, targetID, jobID, taskType, account.Platform, MaxAttempts, failureType, string(publishClassification(cause)), code, reason, response); err != nil {
		contextLogger.Error().Err(logger.RedactError(err)).Str("event", "publish_permanently_failed").Str("status", "permanent_failure").Int("max_attempts", MaxAttempts).Str("failure_type", failureType).Msg("failed to persist permanent publish failure")
		return err
	}
	metrics.JobPermanentlyFailed(account.Platform)
	contextLogger.Error().Err(logger.RedactError(cause)).Str("event", "publish_permanently_failed").Str("status", "permanent_failure").Int("max_attempts", MaxAttempts).Str("failure_type", failureType).Msg("publish job permanently failed")
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
	if rateLimitErr, ok := extractRateLimitError(cause); ok {
		metadata["retry_after"] = rateLimitErr.RetryAfter.String()
		if rateLimitErr.ResetAt != nil {
			metadata["rate_limit_reset"] = rateLimitErr.ResetAt.UTC().Format(time.RFC3339)
		}
		if rateLimitErr.Platform != "" {
			metadata["platform"] = rateLimitErr.Platform
		}
	}
	response, _ := json.Marshal(metadata)
	return failureType, code, reason, response
}

func extractRateLimitError(err error) (*connectors.RateLimitError, bool) {
	var rateLimitErr *connectors.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr, true
	}
	return nil, false
}

func redactSensitive(value string) string {
	return sensitiveValuePattern.ReplaceAllString(value, `${1}$2[REDACTED]`)
}

func (p *PublishProcessor) scheduleReconciliation(
	ctx context.Context,
	postID, targetID uuid.UUID,
) error {
	attempt, err := p.publications.GetPublicationAttempt(ctx, targetID)
	if err != nil {
		return err
	}

	if attempt.ErrorCode != nil &&
		*attempt.ErrorCode == "MANUAL_REVIEW_REQUIRED" {
		return nil
	}

	if attempt.Status != "UNKNOWN" {
		// Preserve infrastructure retries when durable unknown state
		// could not be recorded.
		return errors.New("ambiguous attempt has not been persisted as UNKNOWN")
	}

	post, err := p.publications.GetPostForPublishing(ctx, postID)
	if err != nil {
		return err
	}

	repo, ok := p.publications.(interface {
		QueueReconciliation(
			context.Context, uuid.UUID, uuid.UUID, int,
		) error
	})
	if !ok {
		return errors.New("durable reconciliation is not supported")
	}

	return repo.QueueReconciliation(
		ctx, postID, targetID, post.SchedulingGeneration,
	)
}
