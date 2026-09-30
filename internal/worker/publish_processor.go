package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

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
    posts        *storage.PostRepository
    publications *storage.PublicationRepository
    connectors   map[string]connectors.PlatformConnector
    encryptionKey string
}

func NewPublishProcessor(
    posts *storage.PostRepository,
    publications *storage.PublicationRepository,
    platformConnectors map[string]connectors.PlatformConnector,
    encryptionKey string,
) *PublishProcessor {
    return &PublishProcessor{
        posts:        posts,
        publications: publications,
        connectors:   platformConnectors,
        encryptionKey: encryptionKey,
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
    successCount := 0
    failureCount := 0

    for _, target := range targets {
        if err := p.publishToTarget(ctx, post, &target); err != nil {
            log.Error().
                Err(err).
                Str("post_id", job.PostID.String()).
                Str("target_id", target.ID.String()).
                Msg("Failed to publish to target")
            failureCount++
        } else {
            successCount++
        }
    }

    // Update post status based on results
    if failureCount == 0 {
        // All succeeded
        if err := p.updatePostStatus(ctx, job.PostID, "PUBLISHED"); err != nil {
            log.Error().Err(err).Msg("Failed to update post status to PUBLISHED")
        }
    } else if successCount > 0 {
        // Some succeeded
        if err := p.updatePostStatus(ctx, job.PostID, "PARTIALLY_PUBLISHED"); err != nil {
            log.Error().Err(err).Msg("Failed to update post status to PARTIALLY_PUBLISHED")
        }
    } else {
        // All failed
        if err := p.updatePostStatus(ctx, job.PostID, "FAILED"); err != nil {
            log.Error().Err(err).Msg("Failed to update post status to FAILED")
        }
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

    // Mark as processing
    if err := p.publications.MarkAttemptProcessing(ctx, attempt.ID); err != nil {
        return fmt.Errorf("mark attempt processing: %w", err)
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
        return fmt.Errorf("decrypt access token: %w", err)
    }

    // Get connector for platform
    connector, ok := p.connectors[account.Platform]
    if !ok {
        errMsg := fmt.Sprintf("no connector for platform: %s", account.Platform)
        if err := p.publications.MarkAttemptFailed(ctx, attempt.ID, "UNKNOWN_PLATFORM", errMsg); err != nil {
            log.Error().Err(err).Msg("Failed to mark attempt failed")
        }
        return fmt.Errorf("%s", errMsg)
    }

    // Publish
    caption := ""
    if post.Caption != nil {
        caption = *post.Caption
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
            return fmt.Errorf("create download request: %w", reqErr)
        }
        req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

        resp, respErr := http.DefaultClient.Do(req)
        if respErr != nil {
            return fmt.Errorf("download media: %w", respErr)
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
            return fmt.Errorf("download media failed: %d", resp.StatusCode)
        }

        mediaType := "application/octet-stream"
        if input.MediaType != nil {
            mediaType = *input.MediaType
        }

        tempFile, err := os.CreateTemp("", "upload-*.tmp")
        if err != nil {
            return fmt.Errorf("create temp file: %w", err)
        }
        defer os.Remove(tempFile.Name())
        defer tempFile.Close()

        if _, err := io.Copy(tempFile, resp.Body); err != nil {
            return fmt.Errorf("copy to temp file: %w", err)
        }
        
        if _, err := tempFile.Seek(0, 0); err != nil {
            return fmt.Errorf("seek temp file: %w", err)
        }

        mediaID, uploadErr := connector.UploadMedia(ctx, accessToken, tempFile, mediaType)
        if uploadErr != nil {
            return fmt.Errorf("upload media to platform: %w", uploadErr)
        }
        mediaIDs = append(mediaIDs, mediaID)
    }

    publicURL, platformPostID, err := connector.Publish(ctx, accessToken, input.Caption, mediaIDs...)
    if err != nil {
        errMsg := err.Error()
        if err := p.publications.MarkAttemptFailed(ctx, attempt.ID, "PUBLISH_FAILED", errMsg); err != nil {
            log.Error().Err(err).Msg("Failed to mark attempt failed")
        }
        return fmt.Errorf("publish to platform: %w", err)
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