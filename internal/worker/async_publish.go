package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/jobs"
	logger "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/model"
)

type asyncPublicationStore interface {
	RecordAsyncTerminal(context.Context, uuid.UUID, string) error
	GetTikTokOptions(context.Context, uuid.UUID) (model.TikTokOptions, error)
	GetAsyncPublication(context.Context, uuid.UUID) (*model.AsyncPublication, error)
	BeginAsyncPublication(context.Context, uuid.UUID, time.Time) error
	ClearRejectedAsyncPublication(context.Context, uuid.UUID) error
	SaveAsyncPublication(context.Context, uuid.UUID, string, time.Time) error
	QueueAsyncCheck(context.Context, uuid.UUID, string, time.Time) error
}

func (p *PublishProcessor) WithAsyncPublishing(s *mediastore.DeliverySigner, interval, deadline time.Duration) *PublishProcessor {
	p.deliverySigner = s
	p.asyncPollInterval = interval
	p.asyncPollDeadline = deadline
	p.videoProbe = probeTikTokVideo
	return p
}
func (p *PublishProcessor) ProcessTikTokStatusJob(ctx context.Context, task *asynq.Task) error {
	job, err := jobs.ParsePublishJob(task.Payload())
	if err != nil || job.TargetID == nil || job.AttemptID == nil {
		return fmt.Errorf("%w: invalid status-check payload", asynq.SkipRetry)
	}
	return p.ProcessPublishJob(ctx, task)
}
func (p *PublishProcessor) publishAsyncVideo(ctx context.Context, post *model.Post, target *model.PostTarget, attempt *model.PublicationAttempt, account *model.SocialAccount, connector connectors.PlatformConnector, async connectors.AsyncVideoPublisher) (resultErr error) {
	contextLogger := logger.WithJobContext(getTaskID(ctx), post.ID.String(), post.UserID.String(), account.Platform, attempt.AttemptCount)
	stage := "preflight"
	contextLogger.Info().Str("event", "async_publish_started").Str("target_id", target.ID.String()).Msg("processing asynchronous publication")
	defer func() {
		if resultErr != nil {
			contextLogger.Warn().Err(logger.RedactError(resultErr)).Str("event", "async_publish_failed").Str("stage", stage).Str("classification", string(publishClassification(resultErr))).Msg("asynchronous publication attempt failed")
		} else {
			contextLogger.Info().Str("event", "async_publish_step_completed").Str("stage", stage).Msg("asynchronous publication step completed")
		}
	}()
	store, ok := p.publications.(asyncPublicationStore)
	if !ok {
		return errors.New("async publication repository is not configured")
	}
	op, err := store.GetAsyncPublication(ctx, attempt.ID)
	if err != nil {
		return err
	}
	interval := p.asyncPollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	deadline := p.asyncPollDeadline
	if deadline <= 0 {
		deadline = 24 * time.Hour
	}
	if op != nil {
		stage = "reconciliation"
		if op.PublishID == nil {
			return p.requireManualReview(ctx, attempt.ID, "TikTok initialization may have been accepted without a saved publish ID. Review TikTok before retrying.")
		}
		// Replay a confirmed delivery if the worker stopped before finalizing it.
		if op.SendToInbox && (op.RemoteStatus == "SEND_TO_USER_INBOX" || op.RemoteStatus == "PUBLISH_COMPLETE") {
			return p.completeTikTokInboxUpload(ctx, attempt.ID, target.ID)
		}
		if time.Now().After(op.Deadline) || op.PollCount >= 3000 {
			return p.requireManualReview(ctx, attempt.ID, "TikTok polling limit reached; publication remains unconfirmed")
		}
		if op.NextCheckAt != nil && op.NextCheckAt.After(time.Now()) {
			return nil
		}
		token, _, tokenErr := p.ensureAccessToken(ctx, account, connector)
		if tokenErr != nil {
			if connectors.ClassifyError(tokenErr) == connectors.ErrorRetryable {
				return store.QueueAsyncCheck(ctx, attempt.ID, op.RemoteStatus, time.Now().Add(interval))
			}
			return p.requireManualReview(ctx, attempt.ID, "TikTok credentials require reconnection; saved publish ID retained")
		}
		stage = "polling_status"
		contextLogger.Info().Str("event", "tiktok_status_check_started").Int("poll_count", op.PollCount).Msg("checking TikTok publication status")
		status, fetchErr := async.FetchPublishStatus(ctx, token, *op.PublishID)
		if errors.Is(fetchErr, connectors.ErrAuthFailed) {
			token, _, err = p.refreshAccessToken(ctx, account, connector)
			if err == nil {
				status, fetchErr = async.FetchPublishStatus(ctx, token, *op.PublishID)
			} else {
				fetchErr = err
			}
		}
		if fetchErr != nil {
			contextLogger.Warn().Err(logger.RedactError(fetchErr)).Str("event", "async_status_query_failed").Msg("TikTok status query failed; retaining saved operation")
			if connectors.ClassifyError(fetchErr) == connectors.ErrorPermanent {
				return p.requireManualReview(ctx, attempt.ID, "TikTok status could not be queried; saved publish ID retained")
			}
			delay := interval
			if rate, ok := extractRateLimitError(fetchErr); ok {
				if d := retryPolicy.ResolveRetryDelay(rate, 1); d > delay {
					delay = d
				}
			}
			return store.QueueAsyncCheck(ctx, attempt.ID, op.RemoteStatus, time.Now().Add(delay))
		}
		contextLogger.Info().Str("event", "tiktok_status_received").Str("status", status.Status).Msg("received TikTok publication status")
		if op.SendToInbox && (status.Status == "SEND_TO_USER_INBOX" || status.Status == "PUBLISH_COMPLETE") {
			if err := store.RecordAsyncTerminal(ctx, attempt.ID, status.Status); err != nil {
				return err
			}
			return p.completeTikTokInboxUpload(ctx, attempt.ID, target.ID)
		}
		if status.Status == "PUBLISH_COMPLETE" || status.Status == "FAILED" {
			if err := store.RecordAsyncTerminal(ctx, attempt.ID, status.Status); err != nil {
				return err
			}
		}
		switch status.Status {
		case "PUBLISH_COMPLETE":
			stage = "confirming_publication"
			id := ""
			if len(status.PublicPostIDs) > 0 {
				id = status.PublicPostIDs[0]
			}
			if err := p.publications.MarkAttemptSucceeded(ctx, attempt.ID, id, ""); err != nil {
				return err
			}
			if err := p.publications.MarkPostTargetPublished(ctx, target.ID); err != nil {
				return err
			}
			contextLogger.Info().Str("event", "publish_succeeded").Str("status", "success").Msg("TikTok publication confirmed")
			return nil
		case "FAILED":
			stage = "platform_rejection"
			// A documented terminal result is not an ambiguous external side effect.
			return newPublishError(connectors.ErrorPermanent, connectors.NewClassifiedError(connectors.ErrorPermanent, "TIKTOK_REJECTED", fmt.Errorf("TikTok rejected publication (%s)", safeFailureReason(status.FailureReason))))
		case "PROCESSING_DOWNLOAD", "PROCESSING_UPLOAD":
			return store.QueueAsyncCheck(ctx, attempt.ID, status.Status, time.Now().Add(interval))
		default:
			return p.requireManualReview(ctx, attempt.ID, "TikTok returned an unrecognized video delivery state; saved publish ID retained")
		}
	}
	// No initialization marker means no initialization call was allowed to start.
	// Recovery can safely repeat preflight; the marker is written before the call.
	if p.deliverySigner == nil {
		return newPublishError(connectors.ErrorPermanent, errors.New("TikTok verified media delivery is not configured"))
	}
	if post.MediaID == nil || p.mediaRepo == nil || p.mediaStore == nil {
		return newPublishError(connectors.ErrorPermanent, connectors.ErrMediaRequired)
	}
	media, err := p.mediaRepo.GetMediaByID(ctx, *post.MediaID)
	if err != nil {
		return newPublishError(connectors.ErrorPermanent, connectors.ErrMediaNotFound)
	}
	if media.UserID != post.UserID || media.UploadStatus != "complete" || media.FileSize <= 0 || media.FileSize > 4*1024*1024*1024 {
		return newPublishError(connectors.ErrorPermanent, connectors.ErrMediaIncomplete)
	}
	switch media.ContentType {
	case "video/mp4", "video/quicktime", "video/webm":
	default:
		return newPublishError(connectors.ErrorPermanent, connectors.ErrUnsupportedMedia)
	}
	options, err := store.GetTikTokOptions(ctx, target.ID)
	if err != nil {
		return newPublishError(connectors.ErrorPermanent, err)
	}
	contextLogger.Info().Str("event", "tiktok_media_validation_started").Msg("loading stored video for TikTok validation")
	// Probe actual stored bytes. A client-reported duration is not authoritative.
	reader, err := p.mediaStore.Get(ctx, media.StorageKey)
	if err != nil {
		return newPublishError(connectors.ErrorRetryable, errors.New("TikTok media unavailable from storage"))
	}
	defer reader.Close()
	file, err := os.CreateTemp("", "blink-tiktok-*.video")
	if err != nil {
		return newPublishError(connectors.ErrorRetryable, errors.New("TikTok media staging unavailable"))
	}
	defer os.Remove(file.Name())
	defer file.Close()
	n, err := io.Copy(file, io.LimitReader(reader, media.FileSize+1))
	if err != nil {
		return newPublishError(connectors.ErrorRetryable, errors.New("TikTok media staging interrupted"))
	}
	if n != media.FileSize {
		return newPublishError(connectors.ErrorPermanent, errors.New("TikTok stored media size mismatch"))
	}
	probe := p.videoProbe
	if probe == nil {
		probe = probeTikTokVideo
	}
	stage = "validating_video"
	duration, err := probe(ctx, file.Name())
	if err != nil {
		return newPublishError(connectors.ClassifyError(err), err)
	}
	contextLogger.Info().Str("event", "tiktok_media_validated").Float64("duration_seconds", duration).Msg("TikTok video validation passed")
	options.DurationSeconds = duration
	caption := target.CaptionForPost(post)
	input := connectors.VideoPublishInput{Caption: caption, MediaURL: p.deliverySigner.URL(media.ID, time.Now()), Options: options}
	token, _, err := p.ensureAccessToken(ctx, account, connector)
	if err != nil {
		return newPublishError(connectors.ClassifyError(err), err)
	}
	stage = "validating_creator"
	contextLogger.Info().Str("event", "tiktok_creator_validation_started").Msg("checking TikTok delivery requirements")
	if err = async.ValidateVideo(ctx, token, input); errors.Is(err, connectors.ErrAuthFailed) {
		token, _, err = p.refreshAccessToken(ctx, account, connector)
		if err == nil {
			err = async.ValidateVideo(ctx, token, input)
		}
	}
	if err != nil {
		return newPublishError(connectors.ClassifyError(err), err)
	}
	contextLogger.Info().Str("event", "tiktok_creator_validated").Msg("TikTok delivery requirements validated")
	stage = "initializing_publication"
	if err = store.BeginAsyncPublication(ctx, attempt.ID, time.Now().Add(deadline)); err != nil {
		return err
	}
	contextLogger.Info().Str("event", "tiktok_initialization_started").Msg("initializing TikTok video delivery")
	publishID, initErr := async.InitializeVideo(ctx, token, input)
	if initErr != nil {
		if connectors.ClassifyError(initErr) == connectors.ErrorAmbiguous {
			return p.requireManualReview(ctx, attempt.ID, "TikTok initialization outcome is unknown; do not retry without reviewing TikTok")
		}
		if err = store.ClearRejectedAsyncPublication(ctx, attempt.ID); err != nil {
			return err
		}
		return newPublishError(connectors.ClassifyError(initErr), initErr)
	}
	// Cancellation after an accepted init must not prevent saving its identifier.
	stage = "saving_operation"
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = store.SaveAsyncPublication(saveCtx, attempt.ID, publishID, time.Now().Add(interval)); err != nil {
		return errors.New("persist accepted TikTok operation failed; initialization marker retained for review")
	}
	contextLogger.Info().Str("event", "tiktok_processing").Msg("TikTok operation saved; waiting for publication confirmation")
	return nil
}
func safeFailureReason(reason string) string {
	switch reason {
	case "file_format_check_failed", "duration_check_failed", "frame_rate_check_failed", "picture_size_check_failed", "internal", "video_pull_failed", "publish_cancelled", "auth_removed", "spam_risk_too_many_posts", "spam_risk_user_banned_from_posting", "spam_risk_text", "spam_risk":
		return reason
	}
	return "unrecognized_failure"
}

func (p *PublishProcessor) completeTikTokInboxUpload(ctx context.Context, attemptID, targetID uuid.UUID) error {
	if err := p.publications.MarkAttemptSucceeded(ctx, attemptID, "", ""); err != nil {
		return err
	}
	return p.publications.MarkPostTargetPublished(ctx, targetID)
}
