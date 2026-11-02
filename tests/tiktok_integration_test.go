package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

type durableTikTok struct {
	*connectors.MockConnector
	status                  string
	inits, polls, refreshes int
	pollErr                 error
}

func (c *durableTikTok) ValidateVideo(context.Context, string, connectors.VideoPublishInput) error {
	return nil
}
func (c *durableTikTok) InitializeVideo(context.Context, string, connectors.VideoPublishInput) (string, error) {
	c.inits++
	return "unexpected-init", errors.New("must not reinitialize")
}
func (c *durableTikTok) FetchPublishStatus(context.Context, string, string) (*connectors.VideoPublishStatus, error) {
	c.polls++
	return &connectors.VideoPublishStatus{Status: c.status, FailureReason: "video_pull_failed"}, c.pollErr
}
func (c *durableTikTok) RefreshToken(context.Context, string) (connectors.TokenResult, error) {
	c.refreshes++
	return connectors.TokenResult{AccessToken: "refreshed", ExpiresAt: timePtr(time.Now().Add(time.Hour))}, nil
}
func timePtr(v time.Time) *time.Time { return &v }
func tiktokFixture(t *testing.T, db *pgxpool.Pool) publicationFixture {
	f := fixture(t, db, nil)
	execSQL(t, db, `UPDATE social_accounts SET platform='tiktok',expires_at=NOW()+INTERVAL '1 hour' WHERE id=$1`, f.accountID)
	execSQL(t, db, `UPDATE posts SET status='PUBLISHING' WHERE id=$1`, f.postID)
	if err := storage.NewPublicationRepository(db).MarkAttemptProcessing(context.Background(), f.attemptID); err != nil {
		t.Fatal(err)
	}
	return f
}
func TestTikTokDurablePollingRestartAndTokenRefresh(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := tiktokFixture(t, db)
	repo := storage.NewPublicationRepository(db)
	if err := repo.BeginAsyncPublication(ctx, f.attemptID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAsyncPublication(ctx, f.attemptID, "saved-operation", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	var events int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_type='TIKTOK_STATUS_CHECK' AND payload->>'attempt_id'=$1`, f.attemptID.String()).Scan(&events); err != nil || events != 1 {
		t.Fatalf("missing durable first poll: %d %v", events, err)
	}
	refresh, _ := auth.EncryptToken("refresh", testKey)
	execSQL(t, db, `UPDATE social_accounts SET expires_at=NOW()-INTERVAL '1 hour',refresh_token=$2 WHERE id=$1`, f.accountID, refresh)
	c := &durableTikTok{MockConnector: connectors.NewMockConnector(), status: "PROCESSING_DOWNLOAD"}
	makeWorker := func() *worker.PublishProcessor {
		return worker.NewPublishProcessor(storage.NewPostRepository(db), storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{"tiktok": c}, testKey).WithTokenManager(service.NewTokenManagerService(storage.NewSocialAccountRepository(db), db, testKey))
	}
	task, err := jobs.NewPublishTaskForJob(jobs.PublishJob{PostID: f.postID, TargetID: &f.targetID, AttemptID: &f.attemptID, SchedulingGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := makeWorker().ProcessTikTokStatusJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	op, err := repo.GetAsyncPublication(ctx, f.attemptID)
	if err != nil || op.PollCount != 1 || c.refreshes != 1 || c.inits != 0 {
		t.Fatalf("bad recovery: %+v %v", op, err)
	}
	detail, err := storage.NewPostRepository(db).GetPostWithDetails(ctx, f.userID, f.postID)
	if err != nil || detail.Targets[0].Status != "PROCESSING" {
		t.Fatalf("missing processing state: %+v %v", detail, err)
	}
	execSQL(t, db, `UPDATE tiktok_publications SET next_check_at=NOW()-INTERVAL '1 second' WHERE publication_attempt_id=$1`, f.attemptID)
	c.status = "PUBLISH_COMPLETE"
	if err := makeWorker().ProcessTikTokStatusJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if postStatus(t, db, f.postID) != "PUBLISHED" || c.inits != 0 || c.polls != 2 {
		t.Fatal("publication not confirmed safely")
	}
	attempt, err := repo.GetPublicationAttempt(ctx, f.targetID)
	if err != nil || attempt.PlatformPostID == nil || *attempt.PlatformPostID != "" || *attempt.PlatformURL != "" {
		t.Fatal("private post should succeed without fabricated public IDs")
	}
	if err := makeWorker().ProcessTikTokStatusJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if c.polls != 2 {
		t.Fatal("terminal job repeated API call")
	}
}
func TestTikTokUnknownInitializationRecoveryIsManual(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := tiktokFixture(t, db)
	repo := storage.NewPublicationRepository(db)
	if err := repo.BeginAsyncPublication(ctx, f.attemptID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	c := &durableTikTok{MockConnector: connectors.NewMockConnector()}
	p := worker.NewPublishProcessor(storage.NewPostRepository(db), repo, map[string]connectors.PlatformConnector{"tiktok": c}, testKey)
	task, _ := jobs.NewPublishTask(f.postID, 1)
	if err := p.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	a, err := repo.GetPublicationAttempt(ctx, f.targetID)
	if err != nil || a.Status != "UNKNOWN" || a.ErrorCode == nil || *a.ErrorCode != "MANUAL_REVIEW_REQUIRED" || c.inits != 0 || c.polls != 0 {
		t.Fatalf("unsafe recovery: %+v %v", a, err)
	}
	if err := storage.NewPostRepository(db).RetryPost(ctx, f.userID, f.postID); err == nil {
		t.Fatal("manual retry bypassed unknown state")
	}
}
func TestTikTokSaveOperationAndOutboxAreAtomic(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := tiktokFixture(t, db)
	repo := storage.NewPublicationRepository(db)
	if err := repo.BeginAsyncPublication(ctx, f.attemptID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `ALTER TABLE outbox_events ADD CONSTRAINT test_reject_status CHECK(event_type <> 'TIKTOK_STATUS_CHECK')`)
	if repo.SaveAsyncPublication(ctx, f.attemptID, "operation", time.Now()) == nil {
		t.Fatal("expected failed outbox write")
	}
	op, err := repo.GetAsyncPublication(ctx, f.attemptID)
	if err != nil || op.PublishID != nil {
		t.Fatal("operation ID committed without polling intent")
	}
	execSQL(t, db, `ALTER TABLE outbox_events DROP CONSTRAINT test_reject_status`)
	if err := repo.SaveAsyncPublication(ctx, f.attemptID, "operation", time.Now()); err != nil {
		t.Fatal(err)
	}
}
func TestTikTokOptionsSchedulingAndIdempotency(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := fixture(t, db, nil)
	execSQL(t, db, `UPDATE social_accounts SET platform='tiktok' WHERE id=$1`, f.accountID)
	mediaRepo := storage.NewMediaRepository(db)
	media, err := mediaRepo.CreatePendingMedia(ctx, f.userID, "r2", "private/video", "video.mp4", "video/mp4", 5)
	if err != nil {
		t.Fatal(err)
	}
	if err = mediaRepo.MarkComplete(ctx, media.ID, 5); err != nil {
		t.Fatal(err)
	}
	caption := "scheduled TikTok"
	o := model.TikTokOptions{PrivacyLevel: "SELF_ONLY", Consent: true, DurationSeconds: 5, DisableComment: true, DisableDuet: true, DisableStitch: true}
	input := model.CreatePostInput{Caption: &caption, Targets: []uuid.UUID{f.accountID}, MediaID: &media.ID, MediaIDs: []uuid.UUID{media.ID}, TikTokOptions: map[string]model.TikTokOptions{f.accountID.String(): o}, ScheduledAt: timePtr(time.Now().Add(time.Hour))}
	key := uuid.NewString()
	posts := storage.NewPostRepository(db)
	post, err := posts.CreatePost(ctx, f.userID, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if post.Status != "SCHEDULED" {
		t.Fatal("not scheduled")
	}
	var target uuid.UUID
	if err = db.QueryRow(ctx, `SELECT id FROM post_targets WHERE post_id=$1`, post.ID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	got, err := storage.NewPublicationRepository(db).GetTikTokOptions(ctx, target)
	if err != nil || got.PrivacyLevel != "SELF_ONLY" || !got.Consent {
		t.Fatalf("lost choices: %+v %v", got, err)
	}
	o.PrivacyLevel = "PUBLIC_TO_EVERYONE"
	input.TikTokOptions[f.accountID.String()] = o
	if _, err := posts.CreatePost(ctx, f.userID, key, input); err == nil {
		t.Fatal("idempotency ignored creator settings")
	}
	input.TikTokOptions = nil
	if _, err := posts.CreatePost(ctx, f.userID, uuid.NewString(), input); err == nil {
		t.Fatal("missing settings accepted")
	}
}
func TestTikTokRecoverySweepRecreatesLostDelivery(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := tiktokFixture(t, db)
	repo := storage.NewPublicationRepository(db)
	if err := repo.BeginAsyncPublication(ctx, f.attemptID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAsyncPublication(ctx, f.attemptID, "known", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `DELETE FROM outbox_events WHERE aggregate_id=$1`, f.postID)
	execSQL(t, db, `UPDATE posts SET updated_at=NOW()-INTERVAL '11 minutes' WHERE id=$1`, f.postID)
	if err := storage.NewOutboxRepository(db).RecoverStalledPosts(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND status='PENDING'`, f.postID).Scan(&n); err != nil || n != 1 {
		t.Fatal("lost Redis/outbox delivery not recovered")
	}
	op, err := repo.GetAsyncPublication(ctx, f.attemptID)
	if err != nil || op.PublishID == nil || *op.PublishID != "known" {
		t.Fatal("recovery lost known ID")
	}
}

func TestTikTokAndXPublishIndependently(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := tiktokFixture(t, db)
	repo := storage.NewPublicationRepository(db)
	if err := repo.BeginAsyncPublication(ctx, f.attemptID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAsyncPublication(ctx, f.attemptID, "operation", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	account, target, attempt := uuid.New(), uuid.New(), uuid.New()
	token, _ := auth.EncryptToken("x-token", testKey)
	execSQL(t, db, `INSERT INTO social_accounts(id,user_id,platform,platform_user_id,access_token) VALUES($1,$2,'twitter','x-user',$3)`, account, f.userID, token)
	execSQL(t, db, `INSERT INTO post_targets(id,post_id,social_account_id,status) VALUES($1,$2,$3,'PENDING')`, target, f.postID, account)
	execSQL(t, db, `INSERT INTO publication_attempts(id,post_target_id,status,attempt_count) VALUES($1,$2,'PENDING',0)`, attempt, target)
	c := &durableTikTok{MockConnector: connectors.NewMockConnector(), status: "PROCESSING_DOWNLOAD"}
	x := connectors.NewMockConnector()
	xCalls := 0
	x.PublishFn = func(context.Context, string, string, string, ...string) (string, string, error) {
		xCalls++
		return "https://example.test/x/1", "x-1", nil
	}
	p := worker.NewPublishProcessor(storage.NewPostRepository(db), repo, map[string]connectors.PlatformConnector{"tiktok": c, "twitter": x}, testKey).WithTokenManager(service.NewTokenManagerService(storage.NewSocialAccountRepository(db), db, testKey))
	task, _ := jobs.NewPublishTask(f.postID, 1)
	if err := p.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if xCalls != 1 || postStatus(t, db, f.postID) != "PUBLISHING" {
		t.Fatal("X success incorrectly completed TikTok")
	}
	execSQL(t, db, `UPDATE tiktok_publications SET next_check_at=NOW()-INTERVAL '1 second' WHERE publication_attempt_id=$1`, f.attemptID)
	c.status = "PUBLISH_COMPLETE"
	if err := p.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if xCalls != 1 || c.inits != 0 || postStatus(t, db, f.postID) != "PUBLISHED" {
		t.Fatal("platform retry duplicated X or TikTok")
	}
}

func TestTikTokPermanentRejectionStopsPolling(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := tiktokFixture(t, db)
	repo := storage.NewPublicationRepository(db)
	if err := repo.BeginAsyncPublication(ctx, f.attemptID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAsyncPublication(ctx, f.attemptID, "rejected-operation", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	c := &durableTikTok{MockConnector: connectors.NewMockConnector(), status: "FAILED"}
	p := worker.NewPublishProcessor(storage.NewPostRepository(db), repo, map[string]connectors.PlatformConnector{"tiktok": c}, testKey).WithTokenManager(service.NewTokenManagerService(storage.NewSocialAccountRepository(db), db, testKey))
	task, _ := jobs.NewPublishTask(f.postID, 1)
	if err := p.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if postStatus(t, db, f.postID) != "FAILED" {
		t.Fatal("terminal rejection not persisted")
	}
	if err := p.ProcessPublishJob(ctx, task); err != nil {
		t.Fatal(err)
	}
	if c.polls != 1 || c.inits != 0 {
		t.Fatal("terminal failure repeated external operation")
	}
}
