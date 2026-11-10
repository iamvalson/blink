package worker

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/instagram"
	logger "github.com/iamvalson/blink/internal/log"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

type instagramPublicationStore interface {
	WithInstagramGrant(context.Context, uuid.UUID, uuid.UUID, func() error) error
	GetInstagramPublication(context.Context, uuid.UUID) (*storage.InstagramPublication, error)
	InstagramMedia(context.Context, uuid.UUID, uuid.UUID) (string, []uuid.UUID, error)
	SaveInstagramStep(context.Context, uuid.UUID, storage.InstagramPublication, *time.Time) error
}

type instagramMediaSigner interface {
	PresignRead(context.Context, string, time.Duration) (string, error)
}

func (p *PublishProcessor) publishInstagram(ctx context.Context, post *model.Post, target *model.PostTarget, attempt *model.PublicationAttempt, account *model.SocialAccount, client *instagram.Connector) (resultErr error) {
	log := logger.WithJobContext(getTaskID(ctx), post.ID.String(), post.UserID.String(), instagram.Platform, attempt.AttemptCount).
		With().Str("target_id", target.ID.String()).Str("attempt_id", attempt.ID.String()).Logger()
	stage := "load_operation"
	defer func() {
		if resultErr != nil {
			log.Warn().Str("event", "instagram_publish_step_failed").Str("stage", stage).
				Str("classification", string(publishClassification(resultErr))).Msg("Instagram publishing step failed")
		}
	}()
	finish := func(id, link string) error {
		stage = "finalize"
		if err := p.finishInstagram(ctx, attempt, target, id, link); err != nil {
			return err
		}
		log.Info().Str("event", "instagram_published").Str("media_id", id).
			Bool("has_permalink", link != "").Msg("Instagram publication confirmed")
		return nil
	}
	manualReview := func(reason string) error {
		log.Warn().Str("event", "instagram_manual_review").Str("stage", stage).Msg(reason)
		return p.requireManualReview(ctx, attempt.ID, reason)
	}

	repo, ok := p.publications.(instagramPublicationStore)
	if !ok {
		return errors.New("Instagram publication storage unavailable")
	}
	op, err := repo.GetInstagramPublication(ctx, attempt.ID)
	if err != nil {
		return err
	}
	now := time.Now()
	if op == nil {
		log.Info().Str("event", "instagram_publish_started").Msg("Starting Instagram publication")
		op = &storage.InstagramPublication{Deadline: now.Add(20 * time.Minute), NextCheck: now}
	}
	if op.PublishedID != nil {
		return finish(*op.PublishedID, "")
	}
	if op.NextCheck.After(now) {
		log.Debug().Str("event", "instagram_check_not_due").Time("next_check_at", op.NextCheck).Msg("Instagram check is already scheduled")
		return nil
	}
	if !now.Before(op.Deadline) {
		stage = "processing_deadline"
		if op.SubmittedAt != nil {
			return manualReview("Instagram publication could not be confirmed. Check Instagram before publishing again.")
		}
		return newPublishError(connectors.ErrorPermanent, errors.New("Instagram media processing timed out"))
	}
	queue := func(delay time.Duration) error {
		next := time.Now().Add(delay)
		if err := repo.SaveInstagramStep(ctx, attempt.ID, *op, &next); err != nil {
			return err
		}
		log.Info().Str("event", "instagram_check_scheduled").Str("stage", stage).Time("next_check_at", next).
			Msg("Scheduled Instagram follow-up")
		return nil
	}
	stage = "credentials"
	if p.tokenManager == nil {
		return errors.New("Instagram token manager unavailable")
	}
	token, err := p.tokenManager.InstagramAccessToken(ctx, account.ID.String(), account.UserID.String(), client)
	if err != nil {
		if op.SubmittedAt != nil {
			return manualReview("Reconnect Instagram to inspect an unconfirmed publication.")
		}
		return newPublishError(connectors.ClassifyError(err), err)
	}
	handleRemoteError := func(err error) error {
		if op.SubmittedAt != nil {
			log.Warn().Str("event", "instagram_confirmation_check_failed").Str("stage", stage).
				Str("classification", string(connectors.ClassifyError(err))).Msg("Instagram confirmation check failed; checking again")
			return queue(time.Minute)
		}
		if errors.Is(err, connectors.ErrAuthFailed) {
			err = connectors.ErrInvalidRefreshToken
		}
		class := connectors.ClassifyError(err)
		if class == connectors.ErrorAmbiguous {
			class = connectors.ErrorRetryable
		}
		return newPublishError(class, err)
	}
	if op.Container == nil {
		if len(op.Children) == 0 {
			stage = "publishing_limit"
			if err := client.CheckPublishingLimit(ctx, token, account.PlatformUserID); err != nil {
				return handleRemoteError(err)
			}
		}
		stage = "load_media"
		format, media, err := repo.InstagramMedia(ctx, post.ID, target.ID)
		if err != nil {
			return err
		}
		if len(media) == 0 || format == "" {
			return newPublishError(connectors.ErrorPermanent, connectors.ErrMediaRequired)
		}
		var params url.Values
		caption := target.CaptionForPost(post)
		child := format == "CAROUSEL" && len(op.Children) < len(media)
		if format == "CAROUSEL" && !child {
			stage = "carousel_status"
			for _, id := range op.Children {
				status, err := client.ContainerStatus(ctx, token, id)
				if err != nil {
					return handleRemoteError(err)
				}
				log.Info().Str("event", "instagram_carousel_item_status").Str("container_id", id).Str("status", status).Msg("Received Instagram carousel item status")
				switch status {
				case "FINISHED":
				case "ERROR", "EXPIRED":
					return newPublishError(connectors.ErrorPermanent, errors.New("Instagram rejected a carousel item"))
				default:
					return queue(45 * time.Second)
				}
			}
			params = url.Values{"media_type": {"CAROUSEL"}, "children": {strings.Join(op.Children, ",")}, "caption": {caption}}
		} else {
			index := 0
			if child {
				index = len(op.Children)
			}
			stage = "prepare_media"
			record, mediaURL, err := p.instagramMediaURL(ctx, post.UserID, media[index], format)
			if err != nil {
				return fmt.Errorf("Instagram item %d: %w", index+1, err)
			}
			log.Info().Str("event", "instagram_media_prepared").Str("format", format).Int("item", index+1).Int("item_count", len(media)).Msg("Validated and prepared Instagram media")
			itemFormat := format
			if child {
				itemFormat = "IMAGE"
				if strings.HasPrefix(record.ContentType, "video/") {
					itemFormat = "VIDEO"
				}
			}
			params = instagram.MediaParams(itemFormat, record.ContentType, mediaURL, caption, child)
		}
		stage = "create_container"
		id, err := client.CreateContainer(ctx, token, account.PlatformUserID, params)
		if err != nil {
			return handleRemoteError(err)
		}
		log.Info().Str("event", "instagram_container_created").Str("container_id", id).Str("format", format).Bool("carousel_item", child).Msg("Created Instagram media container")
		if child {
			op.Children = append(op.Children, id)
			return queue(time.Second)
		}
		op.Container = &id
		return queue(30 * time.Second)
	}

	stage = "container_status"
	status, err := client.ContainerStatus(ctx, token, *op.Container)
	if err != nil {
		return handleRemoteError(err)
	}
	log.Info().Str("event", "instagram_container_status").Str("container_id", *op.Container).Str("status", status).Msg("Received Instagram container status")
	switch status {
	case "PUBLISHED":
		// Meta confirmed publication. A lost publish response may leave the media ID unknown.
		return finish("", "")
	case "ERROR", "EXPIRED":
		if op.SubmittedAt != nil {
			return manualReview("Instagram did not confirm the result after submission.")
		}
		return newPublishError(connectors.ErrorPermanent, errors.New("Instagram rejected or expired the media container"))
	case "FINISHED":
		if op.SubmittedAt != nil {
			return queue(time.Minute)
		}
	default:
		return queue(45 * time.Second)
	}

	// Commit the submission marker and recovery job before making the remote write.
	// A timeout or crash after this point always reconciles the same container.
	stage = "record_submission"
	op.SubmittedAt = &now
	if err := queue(time.Minute); err != nil {
		return err
	}
	stage = "submit_container"
	var id string
	err = repo.WithInstagramGrant(ctx, account.ID, target.ID, func() error {
		var err error
		log.Info().Str("event", "instagram_publish_submitted").Str("container_id", *op.Container).Msg("Sending publish request to Instagram")
		id, err = client.PublishContainer(ctx, token, account.PlatformUserID, *op.Container)
		return err
	})
	if err != nil {
		var rejected *connectors.ClassifiedError
		if errors.As(err, &rejected) && rejected.StatusCode >= 400 && rejected.StatusCode < 500 && rejected.Class == connectors.ErrorPermanent {
			op.SubmittedAt = nil
			op.NextCheck = now
			if saveErr := repo.SaveInstagramStep(ctx, attempt.ID, *op, nil); saveErr != nil {
				return saveErr
			}
			return newPublishError(connectors.ErrorPermanent, err)
		}
		log.Warn().Str("event", "instagram_publish_unconfirmed").Str("container_id", *op.Container).
			Str("classification", string(connectors.ClassifyError(err))).Msg("Instagram publish result is unconfirmed; checking the same container")
		return nil
	}
	stage = "save_publish_result"
	op.PublishedID = &id
	if err := repo.SaveInstagramStep(ctx, attempt.ID, *op, nil); err != nil {
		return err
	}
	return finish(id, client.Permalink(ctx, token, id))
}

func (p *PublishProcessor) finishInstagram(ctx context.Context, attempt *model.PublicationAttempt, target *model.PostTarget, id, link string) error {
	if err := p.publications.MarkAttemptSucceeded(ctx, attempt.ID, id, link); err != nil {
		return err
	}
	return p.publications.MarkPostTargetPublished(ctx, target.ID)
}

func (p *PublishProcessor) instagramMediaURL(ctx context.Context, user, id uuid.UUID, format string) (*model.Media, string, error) {
	signer, ok := p.mediaStore.(instagramMediaSigner)
	if !ok || p.mediaRepo == nil {
		return nil, "", newPublishError(connectors.ErrorPermanent, errors.New("Instagram publishing requires R2 media storage"))
	}
	if id == uuid.Nil {
		return nil, "", newPublishError(connectors.ErrorPermanent, connectors.ErrMediaNotFound)
	}
	record, err := p.mediaRepo.GetMediaByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if record == nil || record.UserID != user || record.UploadStatus != "complete" || record.StorageProvider != "r2" {
		return nil, "", newPublishError(connectors.ErrorPermanent, connectors.ErrMediaIncomplete)
	}
	if record.ContentType == "image/jpeg" {
		body, err := p.mediaStore.Get(ctx, record.StorageKey)
		if err != nil {
			return nil, "", err
		}
		check := instagram.ValidateImage(body, record.FileSize, format)
		_ = body.Close()
		if check != nil {
			return nil, "", newPublishError(connectors.ErrorPermanent, check)
		}
	} else if record.ContentType == "video/mp4" || record.ContentType == "video/quicktime" {
		if err := p.validateInstagramVideo(ctx, record, format); err != nil {
			return nil, "", err
		}
	} else {
		return nil, "", newPublishError(connectors.ErrorPermanent, connectors.ErrUnsupportedMedia)
	}
	signed, err := signer.PresignRead(ctx, record.StorageKey, 2*time.Hour)
	if err != nil {
		return nil, "", err
	}
	parsed, err := url.Parse(signed)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, "", newPublishError(connectors.ErrorPermanent, errors.New("Instagram media delivery requires an HTTPS R2 endpoint"))
	}
	return record, signed, nil
}
