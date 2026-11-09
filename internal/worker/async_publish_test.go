package worker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/model"
)

type asyncTestStore struct {
	recoveryStore
	op           *model.AsyncPublication
	options      model.TikTokOptions
	queued       int
	saveError    bool
	successError bool
}

func (s *asyncTestStore) GetTikTokOptions(context.Context, uuid.UUID) (model.TikTokOptions, error) {
	return s.options, nil
}
func (s *asyncTestStore) GetAsyncPublication(context.Context, uuid.UUID) (*model.AsyncPublication, error) {
	return s.op, nil
}
func (s *asyncTestStore) BeginAsyncPublication(_ context.Context, _ uuid.UUID, d time.Time) error {
	s.op = &model.AsyncPublication{Deadline: d, SendToInbox: s.options.SendToInbox}
	return nil
}
func (s *asyncTestStore) ClearRejectedAsyncPublication(context.Context, uuid.UUID) error {
	s.op = nil
	return nil
}
func (s *asyncTestStore) SaveAsyncPublication(_ context.Context, _ uuid.UUID, id string, n time.Time) error {
	if s.saveError {
		return errors.New("database unavailable")
	}
	s.op.PublishID = &id
	s.op.NextCheckAt = &n
	s.queued++
	return nil
}
func (s *asyncTestStore) QueueAsyncCheck(_ context.Context, _ uuid.UUID, status string, n time.Time) error {
	s.op.RemoteStatus = status
	s.op.NextCheckAt = &n
	s.op.PollCount++
	s.queued++
	return nil
}
func (s *asyncTestStore) MarkAttemptSucceeded(ctx context.Context, id uuid.UUID, postID, url string) error {
	if s.successError {
		return errors.New("commit failed")
	}
	return s.recoveryStore.MarkAttemptSucceeded(ctx, id, postID, url)
}

type asyncTestConnector struct {
	crashRecoveryConnector
	initializations, polls int
	initializedCaption     string
	status                 string
	initError, pollError   error
	cancel                 context.CancelFunc
}

func (c *asyncTestConnector) ValidateVideo(context.Context, string, connectors.VideoPublishInput) error {
	return nil
}
func (c *asyncTestConnector) InitializeVideo(_ context.Context, _ string, input connectors.VideoPublishInput) (string, error) {
	c.initializations++
	c.initializedCaption = input.Caption
	if c.cancel != nil {
		c.cancel()
	}
	return "operation-1", c.initError
}
func (c *asyncTestConnector) FetchPublishStatus(context.Context, string, string) (*connectors.VideoPublishStatus, error) {
	c.polls++
	return &connectors.VideoPublishStatus{Status: c.status, PublicPostIDs: []string{"public-1"}, FailureReason: "duration_check_failed"}, c.pollError
}

type asyncTestMedia struct{ media model.Media }

func (m asyncTestMedia) GetMediaByID(context.Context, uuid.UUID) (*model.Media, error) {
	return &m.media, nil
}
func (m asyncTestMedia) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("video")), nil
}
func asyncFixture(t *testing.T) (*PublishProcessor, *asyncTestStore, *asyncTestConnector) {
	t.Helper()
	key := strings.Repeat("a", 64)
	token, e := auth.EncryptToken("valid", key)
	if e != nil {
		t.Fatal(e)
	}
	user, post, target, attempt, media := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	s := &asyncTestStore{recoveryStore: recoveryStore{post: &model.Post{ID: post, UserID: user, MediaID: &media, Caption: stringPtr("video"), SchedulingGeneration: 1}, target: model.PostTarget{ID: target, PostID: post, Status: "PENDING"}, attempt: model.PublicationAttempt{ID: attempt, PostTargetID: target, Status: "PENDING"}, account: model.SocialAccount{UserID: user, Platform: "tiktok", AccessToken: token}}, options: model.TikTokOptions{PrivacyLevel: "SELF_ONLY", Consent: true, DurationSeconds: 5}}
	c := &asyncTestConnector{status: "PROCESSING_DOWNLOAD"}
	signer, e := mediastore.NewDeliverySigner("https://media.example/tiktok-media", strings.Repeat("k", 32), 2*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	m := asyncTestMedia{model.Media{ID: media, UserID: user, FileSize: 5, ContentType: "video/mp4", UploadStatus: "complete"}}
	p := NewPublishProcessor(nil, s, map[string]connectors.PlatformConnector{"tiktok": c}, key).WithMediaStore(m, m).WithAsyncPublishing(signer, 30*time.Second, 24*time.Hour)
	p.videoProbe = func(context.Context, string) (float64, error) { return 5, nil }
	return p, s, c
}
func due(s *asyncTestStore) { past := time.Now().Add(-time.Second); s.op.NextCheckAt = &past }
func TestAsyncPublishPersistsAndResumesAfterRestart(t *testing.T) {
	p, s, c := asyncFixture(t)
	s.target.CaptionOverride = stringPtr("custom TikTok caption")
	ctx := context.Background()
	if err := p.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if c.initializedCaption != *s.target.CaptionOverride {
		t.Fatalf("initialization received caption %q", c.initializedCaption)
	}
	if s.attempt.Status != "PROCESSING" || s.target.Status == "PUBLISHED" || s.op.PublishID == nil || s.queued != 1 {
		t.Fatal("upload acceptance was not durably pending")
	}
	due(s)
	// A fresh processor has no in-memory knowledge of initialization.
	restarted := NewPublishProcessor(nil, s, map[string]connectors.PlatformConnector{"tiktok": c}, p.encryptionKey)
	if err := restarted.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if c.initializations != 1 || c.polls != 1 || s.queued != 2 {
		t.Fatal("restart did not resume polling")
	}
	due(s)
	c.status = "PUBLISH_COMPLETE"
	if err := restarted.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if s.attempt.Status != "SUCCEEDED" || s.target.Status != "PUBLISHED" || *s.attempt.PlatformPostID != "public-1" || *s.attempt.PlatformURL != "" {
		t.Fatal("publication was not confirmed correctly")
	}
	if err := restarted.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if c.initializations != 1 || c.polls != 2 {
		t.Fatal("duplicate external calls")
	}
}
func TestAsyncAmbiguousInitializationNeverDuplicates(t *testing.T) {
	p, s, c := asyncFixture(t)
	c.initError = connectors.NewClassifiedError(connectors.ErrorAmbiguous, "LOST", errors.New("lost response"))
	for range 2 {
		if err := p.publishToTarget(context.Background(), s.post, &s.target); err == nil {
			t.Fatal("expected review")
		}
	}
	if c.initializations != 1 || s.attempt.Status != "UNKNOWN" || s.attempt.ErrorCode == nil || *s.attempt.ErrorCode != "MANUAL_REVIEW_REQUIRED" {
		t.Fatal("unsafe initialization replay")
	}
}
func TestAsyncCrashBeforeIDSavedNeverDuplicates(t *testing.T) {
	p, s, c := asyncFixture(t)
	s.saveError = true
	if p.publishToTarget(context.Background(), s.post, &s.target) == nil {
		t.Fatal("expected save failure")
	}
	s.saveError = false
	if p.publishToTarget(context.Background(), s.post, &s.target) == nil {
		t.Fatal("expected review")
	}
	if c.initializations != 1 {
		t.Fatal("duplicated accepted initialization")
	}
}
func TestAsyncPollingFailuresAndDeadline(t *testing.T) {
	for _, state := range []string{"FAILED", "unrecognized", "rate_limit", "expired", "success_commit_failure"} {
		t.Run(state, func(t *testing.T) {
			p, s, c := asyncFixture(t)
			if e := p.publishToTarget(context.Background(), s.post, &s.target); e != nil {
				t.Fatal(e)
			}
			due(s)
			switch state {
			case "rate_limit":
				c.pollError = connectors.NewRateLimitError("tiktok", 429, time.Minute, nil, "limited")
			case "expired":
				s.op.Deadline = time.Now().Add(-time.Second)
			case "success_commit_failure":
				c.status = "PUBLISH_COMPLETE"
				s.successError = true
			default:
				c.status = state
			}
			err := p.publishToTarget(context.Background(), s.post, &s.target)
			if state == "rate_limit" {
				if err != nil || s.op.PollCount != 1 || s.op.NextCheckAt.Before(time.Now().Add(50*time.Second)) {
					t.Fatal("rate limit not durably delayed")
				}
			} else if err == nil {
				t.Fatal("expected error/review")
			}
			if state == "success_commit_failure" {
				s.successError = false
				due(s)
				if e := p.publishToTarget(context.Background(), s.post, &s.target); e != nil {
					t.Fatal(e)
				}
			}
			if c.initializations != 1 {
				t.Fatal("polling failure restarted publication")
			}
		})
	}
}
func TestAsyncSavesIDAfterCancellation(t *testing.T) {
	p, s, c := asyncFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	defer cancel()
	if err := p.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if s.op.PublishID == nil {
		t.Fatal("accepted ID lost on cancellation")
	}
}

func TestAsyncPreInitializationCrashSafelyResumes(t *testing.T) {
	p, s, c := asyncFixture(t)
	s.attempt.Status = "PROCESSING"
	if err := p.publishToTarget(context.Background(), s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if c.initializations != 1 || s.op == nil || s.op.PublishID == nil {
		t.Fatal("preflight crash did not safely resume")
	}
}

func (s *asyncTestStore) RecordAsyncTerminal(_ context.Context, _ uuid.UUID, status string) error {
	s.op.RemoteStatus = status
	s.op.NextCheckAt = nil
	return nil
}

func TestAsyncInboxDeliveryResumesWithoutSendingAgain(t *testing.T) {
	p, s, c := asyncFixture(t)
	s.options = model.TikTokOptions{SendToInbox: true, Consent: true}
	ctx := context.Background()
	if err := p.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	due(s)
	c.status = "SEND_TO_USER_INBOX"
	s.successError = true
	if err := p.publishToTarget(ctx, s.post, &s.target); err == nil {
		t.Fatal("expected failed local completion")
	}
	s.successError = false
	s.op.Deadline = time.Now().Add(-time.Second)
	if err := p.publishToTarget(ctx, s.post, &s.target); err != nil {
		t.Fatal(err)
	}
	if c.initializations != 1 || c.polls != 1 || s.op.RemoteStatus != "SEND_TO_USER_INBOX" || s.attempt.Status != "SUCCEEDED" ||
		s.attempt.PlatformPostID == nil || *s.attempt.PlatformPostID != "" {
		t.Fatal("Inbox delivery must complete without another API request or a public post ID")
	}
}
