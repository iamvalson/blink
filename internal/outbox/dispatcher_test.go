package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

type fakeEvents struct {
	events    []storage.ClaimedOutboxEvent
	statuses  []string
	retryAt   time.Time
	finishErr error
}

func (f *fakeEvents) RecoverStalledPosts(context.Context) error { return nil }
func (f *fakeEvents) ClaimPendingEvents(context.Context, int, time.Duration) ([]storage.ClaimedOutboxEvent, error) {
	return f.events, nil
}
func (f *fakeEvents) FinishClaim(_ context.Context, _, _ uuid.UUID, status string, at time.Time, _ string) error {
	f.statuses = append(f.statuses, status)
	f.retryAt = at
	return f.finishErr
}

type fakeQueue struct {
	err   error
	calls int
	task  *asynq.Task
	opts  []asynq.Option
}

func (q *fakeQueue) Enqueue(task *asynq.Task, opts ...asynq.Option) (string, error) {
	q.calls++
	q.task = task
	q.opts = opts
	return "task", q.err
}

func testEvent() storage.ClaimedOutboxEvent {
	return storage.ClaimedOutboxEvent{ClaimToken: uuid.New(), Event: model.OutboxEvent{ID: uuid.New(), EventType: "POST_CREATED", Attempts: 1, Payload: map[string]interface{}{"post_id": uuid.NewString(), "target_id": uuid.NewString(), "scheduling_generation": 2}}}
}

func TestDispatchRecoversEnqueueFailuresAndDuplicateTaskIDs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		queueErr   error
		wantStatus string
		wantErr    bool
	}{
		{"success", nil, "PUBLISHED", false},
		{"redis unavailable", errors.New("Redis unavailable"), "PENDING", true},
		{"already enqueued", asynq.ErrTaskIDConflict, "PUBLISHED", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := testEvent()
			store := &fakeEvents{events: []storage.ClaimedOutboxEvent{event}}
			q := &fakeQueue{err: tc.queueErr}
			before := time.Now()
			err := NewDispatcher(store, q).ProcessPendingEvents(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("dispatch error: %v", err)
			}
			if len(store.statuses) != 1 || store.statuses[0] != tc.wantStatus {
				t.Fatalf("completion: %v", store.statuses)
			}
			if tc.wantStatus == "PENDING" && store.retryAt.Before(before.Add(5*time.Second)) {
				t.Fatal("missing backoff")
			}
			job, err := jobs.ParsePublishJob(q.task.Payload())
			if err != nil {
				t.Fatal(err)
			}
			if job.TargetID == nil || job.SchedulingGeneration != 2 {
				t.Fatalf("lost target/generation: %+v", job)
			}
			var taskID string
			for _, opt := range q.opts {
				if opt.Type() == asynq.TaskIDOpt {
					taskID = opt.Value().(string)
				}
			}
			if taskID != "outbox:"+event.Event.ID.String() {
				t.Fatalf("unstable task ID: %s", taskID)
			}
		})
	}
}

func TestPoisonEventDoesNotBlockOtherEvents(t *testing.T) {
	bad := testEvent()
	bad.DecodeError = errors.New("payload is not an object")
	good := testEvent()
	store := &fakeEvents{events: []storage.ClaimedOutboxEvent{bad, good}}
	q := &fakeQueue{}
	if err := NewDispatcher(store, q).ProcessPendingEvents(context.Background()); err == nil {
		t.Fatal("expected poison event error")
	}
	if q.calls != 1 || len(store.statuses) != 2 || store.statuses[0] != "FAILED" || store.statuses[1] != "PUBLISHED" {
		t.Fatalf("events blocked: calls=%d statuses=%v", q.calls, store.statuses)
	}
}

func TestDispatchSurfacesCompletionFailure(t *testing.T) {
	store := &fakeEvents{events: []storage.ClaimedOutboxEvent{testEvent()}, finishErr: storage.ErrOutboxClaimLost}
	if err := NewDispatcher(store, &fakeQueue{}).ProcessPendingEvents(context.Background()); !errors.Is(err, storage.ErrOutboxClaimLost) {
		t.Fatalf("lost completion error: %v", err)
	}
}
