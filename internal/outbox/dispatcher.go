package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
)

type eventStore interface {
	RecoverStalledPosts(context.Context) error
	ClaimPendingEvents(context.Context, int, time.Duration) ([]storage.ClaimedOutboxEvent, error)
	FinishClaim(context.Context, uuid.UUID, uuid.UUID, string, time.Time, string) error
}

type taskEnqueuer interface {
	Enqueue(*asynq.Task, ...asynq.Option) (string, error)
}

type Dispatcher struct {
	outboxRepo eventStore
	postsRepo  *storage.PostRepository
	jobsClient taskEnqueuer
}

func NewDispatcher(
	outboxRepo eventStore,
	jobsClient taskEnqueuer,
) *Dispatcher {
	return &Dispatcher{
		outboxRepo: outboxRepo,
		jobsClient: jobsClient,
	}
}

func (d *Dispatcher) WithPostRepository(
	postsRepo *storage.PostRepository,
) *Dispatcher {
	d.postsRepo = postsRepo
	return d
}

func (d *Dispatcher) ProcessScheduledPosts(ctx context.Context) error {
	if d.postsRepo == nil {
		return nil
	}
	_, err := d.postsRepo.ClaimDueScheduledPosts(ctx, 50)
	return err
}

func (d *Dispatcher) ProcessPendingEvents(ctx context.Context) error {
	if err := d.outboxRepo.RecoverStalledPosts(ctx); err != nil {
		return fmt.Errorf("recover stalled posts: %w", err)
	}

	events, err := d.outboxRepo.ClaimPendingEvents(ctx, 100, time.Minute)
	if err != nil {
		return err
	}

	var failures []error
	for _, event := range events {
		if err := d.dispatchEvent(ctx, event); err != nil {
			log.Error().
				Err(err).
				Str("event_id", event.Event.ID.String()).
				Msg("Outbox dispatch failed")
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (d *Dispatcher) dispatchEvent(
	ctx context.Context,
	claimed storage.ClaimedOutboxEvent,
) error {
	event := claimed.Event

	failPermanently := func(cause error) error {
		saveErr := d.outboxRepo.FinishClaim(
			ctx, event.ID, claimed.ClaimToken,
			"FAILED", time.Time{}, cause.Error(),
		)
		return errors.Join(cause, saveErr)
	}

	if claimed.DecodeError != nil {
		return failPermanently(claimed.DecodeError)
	}

	if event.EventType != "POST_CREATED" {
		return failPermanently(fmt.Errorf(
			"unsupported event type: %s", event.EventType,
		))
	}

	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return failPermanently(err)
	}

	job, err := jobs.ParsePublishJob(payload)
	if err != nil {
		return failPermanently(err)
	}
	if job.SchedulingGeneration < 1 {
		// Recovery creates a fresh event with the authoritative generation.
		return failPermanently(errors.New("missing scheduling generation"))
	}

	task, err := jobs.NewPublishTaskForJob(*job)
	if err != nil {
		return failPermanently(err)
	}

	_, enqueueErr := d.jobsClient.Enqueue(
		task,
		asynq.TaskID("outbox:"+event.ID.String()),
		asynq.MaxRetry(10), // Infrastructure retries only.
		asynq.Timeout(5*time.Minute),
		asynq.Retention(24*time.Hour),
	)

	// A previous dispatcher may have enqueued successfully and crashed
	// before marking the event published.
	if enqueueErr != nil && !errors.Is(enqueueErr, asynq.ErrTaskIDConflict) {
		delay := 5 * time.Second
		for i := 1; i < event.Attempts && delay < 5*time.Minute; i++ {
			delay *= 2
		}
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}

		saveErr := d.outboxRepo.FinishClaim(
			ctx, event.ID, claimed.ClaimToken,
			"PENDING", time.Now().Add(delay), enqueueErr.Error(),
		)
		return errors.Join(enqueueErr, saveErr)
	}

	return d.outboxRepo.FinishClaim(
		ctx, event.ID, claimed.ClaimToken,
		"PUBLISHED", time.Time{}, "",
	)
}
