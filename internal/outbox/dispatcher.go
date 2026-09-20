package outbox

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/rs/zerolog/log"
)

type Dispatcher struct {
    outboxRepo *storage.OutboxRepository
    jobsClient *jobs.Client
}

func NewDispatcher(outboxRepo *storage.OutboxRepository, jobsClient *jobs.Client) *Dispatcher {
    return &Dispatcher{
        outboxRepo: outboxRepo,
        jobsClient: jobsClient,
    }
}

// ProcessPendingEvents reads pending outbox events and enqueues corresponding jobs
func (d *Dispatcher) ProcessPendingEvents(ctx context.Context) error {
    events, err := d.outboxRepo.GetPendingEvents(ctx, 100) // Process up to 100 at a time
    if err != nil {
        return fmt.Errorf("get pending events: %w", err)
    }

    for _, event := range events {
        if err := d.dispatchEvent(ctx, event); err != nil {
            log.Error().
                Err(err).
                Str("event_id", event.ID.String()).
                Msg("Failed to dispatch event")
            continue // Continue processing other events
        }
    }

    return nil
}

func (d *Dispatcher) dispatchEvent(ctx context.Context, event *model.OutboxEvent) error {
    // Mark event as processing
    if err := d.outboxRepo.MarkEventProcessing(ctx, event.ID); err != nil {
        return fmt.Errorf("mark event processing: %w", err)
    }

    // Handle based on event type
    var task *asynq.Task
    var err error

    switch event.EventType {
    case "POST_CREATED":
        task, err = d.handlePostCreated(event)
    default:
        return fmt.Errorf("unknown event type: %s", event.EventType)
    }

    if err != nil {
        return fmt.Errorf("handle event: %w", err)
    }

    // Enqueue task with idempotency
    _, err = d.jobsClient.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(5*60*1000*1000*1000)) // 5 minutes
    if err != nil {
        // If enqueue fails, mark event as failed
        if err := d.outboxRepo.MarkEventFailed(ctx, event.ID, err.Error()); err != nil {
            log.Error().Err(err).Msg("Failed to mark event as failed")
        }
        return fmt.Errorf("enqueue task: %w", err)
    }

    // Mark event as published
    if err := d.outboxRepo.MarkEventPublished(ctx, event.ID); err != nil {
        return fmt.Errorf("mark event published: %w", err)
    }

    return nil
}

func (d *Dispatcher) handlePostCreated(event *model.OutboxEvent) (*asynq.Task, error) {
    // Extract post_id from payload
    postIDStr, ok := event.Payload["post_id"].(string)
    if !ok {
        return nil, fmt.Errorf("missing or invalid post_id in payload")
    }

    postID, err := uuid.Parse(postIDStr)
    if err != nil {
        return nil, fmt.Errorf("parse post_id: %w", err)
    }

    return jobs.NewPublishTask(postID)
}