package tests

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/connectors/twitter"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type publicationFixture struct{ userID, accountID, postID, targetID, attemptID uuid.UUID }

func execSQL(t *testing.T, db *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, db *pgxpool.Pool, scheduledAt *time.Time) publicationFixture {
	t.Helper()
	f := publicationFixture{userID: uuid.New(), accountID: uuid.New()}
	token, err := auth.EncryptToken("test-token", testKey)
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO users(id,email,password_hash) VALUES($1,$2,'test')`, f.userID, f.userID.String()+"@example.test")
	execSQL(t, db, `INSERT INTO social_accounts(id,user_id,platform,platform_user_id,access_token) VALUES($1,$2,'twitter','test',$3)`, f.accountID, f.userID, token)
	caption := "test publication"
	post, err := storage.NewPostRepository(db).CreatePost(context.Background(), f.userID, uuid.NewString(), model.CreatePostInput{Caption: &caption, Targets: []uuid.UUID{f.accountID}, ScheduledAt: scheduledAt})
	if err != nil {
		t.Fatal(err)
	}
	f.postID = post.ID
	err = db.QueryRow(context.Background(), `SELECT pt.id,pa.id FROM post_targets pt JOIN publication_attempts pa ON pa.post_target_id=pt.id WHERE pt.post_id=$1`, post.ID).Scan(&f.targetID, &f.attemptID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func postStatus(t *testing.T, db *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := db.QueryRow(context.Background(), `SELECT status FROM posts WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestOutboxLeaseRecoveryAndConcurrentClaims(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	repo := storage.NewOutboxRepository(db)
	var wg sync.WaitGroup
	results := make(chan []storage.ClaimedOutboxEvent, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := repo.ClaimPendingEvents(ctx, 10, time.Minute)
			results <- e
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var claimed []storage.ClaimedOutboxEvent
	for e := range results {
		claimed = append(claimed, e...)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed event, got %d", len(claimed))
	}
	old := claimed[0]
	execSQL(t, db, `UPDATE outbox_events SET locked_until=NOW()-INTERVAL '1 second' WHERE id=$1`, old.Event.ID)
	recovered, err := repo.ClaimPendingEvents(ctx, 10, time.Minute)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("expired claim recovery: %v %v", recovered, err)
	}
	if err := repo.FinishClaim(ctx, old.Event.ID, old.ClaimToken, "PUBLISHED", time.Time{}, ""); !errors.Is(err, storage.ErrOutboxClaimLost) {
		t.Fatalf("stale owner completion: %v", err)
	}
	next := recovered[0]
	if err := repo.FinishClaim(ctx, next.Event.ID, next.ClaimToken, "PENDING", time.Now().Add(time.Hour), "Redis unavailable"); err != nil {
		t.Fatal(err)
	}
	if events, err := repo.ClaimPendingEvents(ctx, 10, time.Minute); err != nil || len(events) != 0 {
		t.Fatalf("backoff bypassed: %v %v", events, err)
	}
	execSQL(t, db, `UPDATE outbox_events SET available_at=NOW()-INTERVAL '1 second' WHERE aggregate_id=$1`, f.postID)
	events, err := repo.ClaimPendingEvents(ctx, 10, time.Minute)
	if err != nil || len(events) != 1 {
		t.Fatalf("due event: %v %v", events, err)
	}
}

func TestRetryIntentRollsBackWithOutboxFailure(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	repo := storage.NewPublicationRepository(db)
	if ok, err := repo.ClaimPostPublishing(ctx, f.postID, 1); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := repo.MarkAttemptProcessing(ctx, f.attemptID); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `CREATE FUNCTION reject_test_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected outbox failure'; END $$`)
	execSQL(t, db, `CREATE TRIGGER reject_test_event BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_test_event()`)
	err := repo.MarkAttemptRetrying(ctx, f.attemptID, "RETRY", "RETRYABLE", "temporary", time.Now().Add(time.Minute))
	if err == nil {
		t.Fatal("expected outbox insertion to fail")
	}
	assertRetryState(t, ctx, db, f.attemptID, "PROCESSING", 1)
	var failures int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM publication_attempt_failures WHERE publication_attempt_id=$1`, f.attemptID).Scan(&failures); err != nil {
		t.Fatal(err)
	}
	if failures != 0 {
		t.Fatal("failure history must roll back with retry intent")
	}
	execSQL(t, db, `DROP TRIGGER reject_test_event ON outbox_events`)
	if err := repo.MarkAttemptRetrying(ctx, f.attemptID, "RETRY", "RETRYABLE", "temporary", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertRetryState(t, ctx, db, f.attemptID, "RETRYING", 1)
	if err := repo.MarkAttemptProcessing(ctx, f.attemptID); !errors.Is(err, storage.ErrAttemptNotClaimed) {
		t.Fatalf("future retry claimed: %v", err)
	}
}

func TestSchedulingCancellationAndGenerations(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	posts := storage.NewPostRepository(db)
	repo := storage.NewPublicationRepository(db)
	future := time.Now().Add(time.Hour)
	f := fixture(t, db, &future)
	if ok, err := repo.ClaimPostPublishing(ctx, f.postID, 1); err != nil || ok {
		t.Fatalf("published before due: %v %v", ok, err)
	}
	if _, err := posts.ReschedulePost(ctx, f.userID, f.postID, future.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `UPDATE posts SET scheduled_at=NOW()-INTERVAL '1 second' WHERE id=$1`, f.postID)
	claimed, err := posts.ClaimDueScheduledPosts(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("due scheduling: %v %v", claimed, err)
	}
	if ok, err := repo.ClaimPostPublishing(ctx, f.postID, 1); err != nil || ok {
		t.Fatalf("stale generation published: %v %v", ok, err)
	}
	if _, err := posts.CancelSchedule(ctx, f.userID, f.postID); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ClaimPostPublishing(ctx, f.postID, 2); err != nil || ok {
		t.Fatalf("cancelled post published: %v %v", ok, err)
	}
	if postStatus(t, db, f.postID) != "CANCELLED" {
		t.Fatal("cancelled state overwritten")
	}
	f = fixture(t, db, nil)
	if ok, err := repo.ClaimPostPublishing(ctx, f.postID, 1); err != nil || !ok {
		t.Fatal(err)
	}
	if _, err := posts.CancelSchedule(ctx, f.userID, f.postID); !errors.Is(err, storage.ErrPostCannotBeCancelled) {
		t.Fatalf("active publication cancelled: %v", err)
	}
}

func TestCancelRacesWithPublishingClaim(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		f := fixture(t, db, nil)
		start := make(chan struct{})
		cancelled := make(chan error, 1)
		go func() {
			<-start
			_, err := storage.NewPostRepository(db).CancelSchedule(ctx, f.userID, f.postID)
			cancelled <- err
		}()
		close(start)
		claimed, err := storage.NewPublicationRepository(db).ClaimPostPublishing(ctx, f.postID, 1)
		if err != nil {
			t.Fatal(err)
		}
		cancelErr := <-cancelled
		if claimed {
			if !errors.Is(cancelErr, storage.ErrPostCannotBeCancelled) || postStatus(t, db, f.postID) != "PUBLISHING" {
				t.Fatalf("publishing winner: %v", cancelErr)
			}
		} else if cancelErr != nil || postStatus(t, db, f.postID) != "CANCELLED" {
			t.Fatalf("cancellation winner: %v", cancelErr)
		}
	}
}

type controlledConnector struct {
	connectors.PlatformConnector
	calls      atomic.Int32
	entered    chan struct{}
	release    chan struct{}
	publishErr error
	outcome    connectors.ReconciliationOutcome
}

func (c *controlledConnector) Publish(ctx context.Context, token, attempt, caption string, media ...string) (string, string, error) {
	c.calls.Add(1)
	if c.entered != nil {
		close(c.entered)
		select {
		case <-c.release:
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	if c.publishErr != nil {
		return "", "", c.publishErr
	}
	return "https://example.test/published", "published-id", nil
}
func (c *controlledConnector) ReconcilePublish(context.Context, string, string, string, string) (connectors.ReconciliationResult, error) {
	return connectors.ReconciliationResult{Outcome: c.outcome}, nil
}

func TestConcurrentWorkersAndDuplicateDelivery(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn := &controlledConnector{PlatformConnector: twitter.NewMock(), entered: make(chan struct{}), release: make(chan struct{})}
	defer func() {
		select {
		case <-conn.release:
		default:
			close(conn.release)
		}
	}()
	processor := worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{"twitter": conn}, testKey)
	task, err := jobs.NewPublishTask(f.postID, 1)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- processor.ProcessPublishJob(ctx, task) }()
	select {
	case <-conn.entered:
	case err := <-finished:
		t.Fatalf("worker ended before publication: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := processor.ProcessPublishJob(ctx, task); err == nil {
		t.Fatal("second worker should not acquire active post")
	}
	close(conn.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if conn.calls.Load() != 1 || postStatus(t, db, f.postID) != "PUBLISHED" {
		t.Fatalf("duplicate publication: calls=%d", conn.calls.Load())
	}
}

func TestAmbiguousPublicationRequiresReview(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	conn := &controlledConnector{PlatformConnector: twitter.NewMock(), publishErr: errors.New("response lost"), outcome: connectors.ReconciliationUnknown}
	processor := worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{"twitter": conn}, testKey)
	task, err := jobs.NewPublishTask(f.postID, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := processor.ProcessPublishJob(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	var status, code string
	if err := db.QueryRow(ctx, `SELECT status,error_code FROM publication_attempts WHERE id=$1`, f.attemptID).Scan(&status, &code); err != nil {
		t.Fatal(err)
	}
	if status != "UNKNOWN" || code != "MANUAL_REVIEW_REQUIRED" || conn.calls.Load() != 1 {
		t.Fatalf("ambiguous state %s/%s, calls=%d", status, code, conn.calls.Load())
	}
	if err := storage.NewPostRepository(db).RetryPost(ctx, f.userID, f.postID); err == nil {
		t.Fatal("manual retry must not reset uncertain publication")
	}
}

func TestMixedOutcomesAndManualRetry(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	token, err := auth.EncryptToken("test-token", testKey)
	if err != nil {
		t.Fatal(err)
	}
	account, target, attempt := uuid.New(), uuid.New(), uuid.New()
	execSQL(t, db, `INSERT INTO social_accounts(id,user_id,platform,platform_user_id,access_token) VALUES($1,$2,'test_failure','test',$3)`, account, f.userID, token)
	execSQL(t, db, `INSERT INTO post_targets(id,post_id,social_account_id) VALUES($1,$2,$3)`, target, f.postID, account)
	execSQL(t, db, `INSERT INTO publication_attempts(id,post_target_id) VALUES($1,$2)`, attempt, target)
	good := &controlledConnector{PlatformConnector: twitter.NewMock()}
	bad := &controlledConnector{PlatformConnector: twitter.NewMock(), publishErr: connectors.NewClassifiedError(connectors.ErrorPermanent, "REJECTED", errors.New("rejected"))}
	processor := worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{"twitter": good, "test_failure": bad}, testKey)
	task, _ := jobs.NewPublishTask(f.postID, 1)
	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if got := postStatus(t, db, f.postID); got != "PARTIALLY_PUBLISHED" {
		t.Fatalf("mixed status: %s", got)
	}
	if err := storage.NewPostRepository(db).RetryPost(ctx, f.userID, f.postID); err != nil {
		t.Fatal(err)
	}
	// The old generation must not execute the reset target.
	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if bad.calls.Load() != 1 {
		t.Fatal("stale job ran")
	}
	bad.publishErr = nil
	task, _ = jobs.NewPublishTask(f.postID, 2)
	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if postStatus(t, db, f.postID) != "PUBLISHED" || good.calls.Load() != 1 || bad.calls.Load() != 2 {
		t.Fatalf("manual retry republished successful target: %d/%d", good.calls.Load(), bad.calls.Load())
	}
}

func TestRecoverLostDeliveryOnlyOnce(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	execSQL(t, db, `UPDATE outbox_events SET status='PUBLISHED',created_at=NOW()-INTERVAL '1 hour' WHERE aggregate_id=$1`, f.postID)
	execSQL(t, db, `UPDATE posts SET updated_at=NOW()-INTERVAL '1 hour' WHERE id=$1`, f.postID)
	repo := storage.NewOutboxRepository(db)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { errs <- repo.RecoverStalledPosts(ctx) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND status='PENDING'`, f.postID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recovery created %d events", count)
	}
}

func TestPostMediaValidation(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	caption := "caption"
	id := uuid.New()
	for _, in := range []model.CreatePostInput{
		{MediaIDs: []uuid.UUID{uuid.New(), uuid.New()}},
		{MediaID: &id, MediaIDs: []uuid.UUID{uuid.New()}},
	} {
		in.Caption = &caption
		in.Targets = []uuid.UUID{f.accountID}
		if _, err := storage.NewPostRepository(db).CreatePost(ctx, f.userID, uuid.NewString(), in); err == nil {
			t.Fatal("invalid media accepted")
		}
	}
	execSQL(t, db, `INSERT INTO media(id,user_id,storage_provider,storage_key,original_filename,content_type,file_size,upload_status) VALUES($1,$2,'local',$3,'image.png','image/png',100,'complete')`, id, f.userID, fmt.Sprintf("test/%s", id))
	post, err := storage.NewPostRepository(db).CreatePost(ctx, f.userID, uuid.NewString(), model.CreatePostInput{Caption: &caption, Targets: []uuid.UUID{f.accountID}, MediaID: &id})
	if err != nil {
		t.Fatalf("expected X media post to be created successfully, got: %v", err)
	}
	if post == nil || post.MediaID == nil || *post.MediaID != id {
		t.Fatalf("unexpected created post: %+v", post)
	}

	pdfID := uuid.New()
	execSQL(t, db, `INSERT INTO media(id,user_id,storage_provider,storage_key,original_filename,content_type,file_size,upload_status) VALUES($1,$2,'local',$3,'doc.pdf','application/pdf',100,'complete')`, pdfID, f.userID, fmt.Sprintf("test/%s", pdfID))
	_, err = storage.NewPostRepository(db).CreatePost(ctx, f.userID, uuid.NewString(), model.CreatePostInput{Caption: &caption, Targets: []uuid.UUID{f.accountID}, MediaID: &pdfID})
	if err == nil || !strings.Contains(err.Error(), "unsupported media type for Twitter/X") {
		t.Fatalf("expected unsupported media type error for PDF, got: %v", err)
	}
}

func TestRetryExhaustionFinalizesParentAndDeadLetter(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	conn := &controlledConnector{PlatformConnector: twitter.NewMock(), publishErr: connectors.NewClassifiedError(connectors.ErrorRetryable, "TEMPORARY", errors.New("temporary failure"))}
	processor := worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{"twitter": conn}, testKey)
	task, err := jobs.NewPublishTask(f.postID, 1)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= worker.MaxAttempts; attempt++ {
		if err := processor.ProcessPublishJob(ctx, task); err != nil {
			t.Fatal(err)
		}
		if attempt < worker.MaxAttempts {
			execSQL(t, db, `UPDATE publication_attempts SET next_retry_at=NOW()-INTERVAL '1 second' WHERE id=$1`, f.attemptID)
		}
	}
	assertRetryState(t, ctx, db, f.attemptID, "FAILED", worker.MaxAttempts)
	if postStatus(t, db, f.postID) != "FAILED" {
		t.Fatal("parent remained in progress after exhaustion")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM dead_letter_jobs WHERE post_target_id=$1`, f.targetID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one dead letter, got %d", count)
	}
	if err := processor.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if conn.calls.Load() != int32(worker.MaxAttempts) {
		t.Fatal("published past retry limit")
	}
}
