package tests

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/api/service"
	"github.com/iamvalson/blink/internal/auth"
	"github.com/iamvalson/blink/internal/connectors"
	"github.com/iamvalson/blink/internal/jobs"
	"github.com/iamvalson/blink/internal/mediastore"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
	"github.com/iamvalson/blink/internal/worker"
)

func TestScheduledCaptionOverrideSurvivesRetry(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	ctx := context.Background()
	posts := storage.NewPostRepository(db)
	api := service.NewPostService(posts)
	token, err := auth.EncryptToken("youtube-test-token", testKey)
	if err != nil {
		t.Fatal(err)
	}
	mediaStore, err := mediastore.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mediaStore.Put(ctx, "caption-test.mp4", strings.NewReader(strings.Repeat("v", 100)), "video/mp4"); err != nil {
		t.Fatal(err)
	}
	youtubeID, mediaID := uuid.New(), uuid.New()
	execSQL(t, db, `INSERT INTO social_accounts(id,user_id,platform,platform_user_id,access_token) VALUES($1,$2,'youtube','test-channel',$3)`, youtubeID, f.userID, token)
	execSQL(t, db, `INSERT INTO media(id,user_id,storage_provider,storage_key,original_filename,content_type,file_size,upload_status) VALUES($1,$2,'local','caption-test.mp4','video.mp4','video/mp4',100,'complete')`, mediaID, f.userID)

	shared, override := "Launch\n"+strings.Repeat("a", 300), "Short launch announcement"
	scheduledAt := time.Now().Add(time.Hour)
	input := model.CreatePostInput{
		Caption: &shared, Targets: []uuid.UUID{f.accountID, youtubeID}, MediaID: &mediaID, ScheduledAt: &scheduledAt,
	}
	key := uuid.NewString()
	if _, err := api.CreatePost(ctx, f.userID, key, input); !errors.Is(err, service.ErrPostValidation) {
		t.Fatalf("long inherited X caption should be rejected: %v", err)
	}
	input.CaptionOverrides = map[string]string{uuid.NewString(): override}
	if _, err := api.CreatePost(ctx, f.userID, key, input); !errors.Is(err, service.ErrPostValidation) {
		t.Fatalf("override for unselected account should be rejected: %v", err)
	}
	input.CaptionOverrides = map[string]string{f.accountID.String(): override}
	post, err := api.CreatePost(ctx, f.userID, key, input)
	if err != nil || post.Status != model.StatusScheduled {
		t.Fatalf("create scheduled post: %+v %v", post, err)
	}
	replayed, err := api.CreatePost(ctx, f.userID, key, input)
	if err != nil || replayed.ID != post.ID {
		t.Fatalf("identical request did not replay: %+v %v", replayed, err)
	}
	input.CaptionOverrides[f.accountID.String()] = "Different override"
	if _, err := api.CreatePost(ctx, f.userID, key, input); err == nil {
		t.Fatal("reused key accepted a different override")
	}
	details, err := api.GetPost(ctx, f.userID, post.ID)
	if err != nil || len(details.Targets) != 2 {
		t.Fatalf("read post details: %+v %v", details, err)
	}
	for _, target := range details.Targets {
		if target.SocialAccountID == f.accountID {
			if target.CaptionOverride == nil || *target.CaptionOverride != override {
				t.Fatalf("missing X override: %+v", target)
			}
		} else if target.CaptionOverride != nil {
			t.Fatalf("shared target has an override: %+v", target)
		}
	}

	execSQL(t, db, `UPDATE posts SET scheduled_at=NOW()-INTERVAL '1 second' WHERE id=$1`, post.ID)
	claimed, err := posts.ClaimDueScheduledPosts(ctx, 1)
	if err != nil || len(claimed) != 1 || claimed[0].ID != post.ID {
		t.Fatalf("claim scheduled post: %+v %v", claimed, err)
	}

	x, youtube := connectors.NewMockConnector(), connectors.NewMockConnector()
	var xCaptions, youtubeCaptions []string
	x.PublishFn = func(_ context.Context, _, _, caption string, _ ...string) (string, string, error) {
		xCaptions = append(xCaptions, caption)
		if len(xCaptions) == 1 {
			return "", "", connectors.NewClassifiedError(connectors.ErrorPermanent, "REJECTED", errors.New("injected failure"))
		}
		return "https://example.test/x", "x-id", nil
	}
	youtube.PublishFn = func(_ context.Context, _, _, caption string, _ ...string) (string, string, error) {
		youtubeCaptions = append(youtubeCaptions, caption)
		return "https://example.test/youtube", "youtube-id", nil
	}
	processor := worker.NewPublishProcessor(posts, storage.NewPublicationRepository(db), map[string]connectors.PlatformConnector{
		"twitter": x, "youtube": youtube,
	}, testKey).WithMediaStore(storage.NewMediaRepository(db), mediaStore)
	publish := func(generation int) {
		t.Helper()
		task, err := jobs.NewPublishTask(post.ID, generation)
		if err != nil {
			t.Fatal(err)
		}
		if err := processor.ProcessPublishJob(ctx, task); err != nil {
			t.Fatal(err)
		}
	}

	publish(post.SchedulingGeneration)
	if status := postStatus(t, db, post.ID); status != model.StatusPartiallyPublished {
		t.Fatalf("expected a partial publication, got %s", status)
	}
	if err := posts.RetryPost(ctx, f.userID, post.ID); err != nil {
		t.Fatal(err)
	}
	publish(post.SchedulingGeneration + 1)
	if status := postStatus(t, db, post.ID); status != model.StatusPublished {
		t.Fatalf("expected retry to publish, got %s", status)
	}
	if !slices.Equal(xCaptions, []string{override, override}) || !slices.Equal(youtubeCaptions, []string{shared}) {
		t.Fatalf("unexpected connector captions, X: %q, YouTube: %q", xCaptions, youtubeCaptions)
	}
}
