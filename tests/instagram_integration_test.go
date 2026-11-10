package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
	"github.com/iamvalson/blink/internal/storage"
)

func TestInstagramCarouselAndSubmissionRecovery(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	user := uuid.New()
	execSQL(t, db, `INSERT INTO users(id,email,password_hash) VALUES($1,$2,'test')`, user, user.String()+"@example.test")
	accounts := storage.NewSocialAccountRepository(db)
	if err := accounts.Upsert(ctx, user.String(), "instagram", "123", "ciphertext", "", time.Now().Add(time.Hour), storage.SocialAccountMetadata{ProviderSubject: "456", Username: "blink-test"}); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Upsert(ctx, user.String(), "instagram", "999", "ciphertext", "", time.Now().Add(time.Hour), storage.SocialAccountMetadata{ProviderSubject: "999", Username: "replacement"}); !errors.Is(err, storage.ErrAccountIdentityMismatch) {
		t.Fatalf("replacement: %v", err)
	}
	linked, err := accounts.ListByUserID(ctx, user.String())
	if err != nil || len(linked) != 1 || linked[0].Username != "blink-test" {
		t.Fatalf("connected account metadata: %v", err)
	}
	account := linked[0].ID
	media := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range media {
		execSQL(t, db, `INSERT INTO media(id,user_id,storage_key,original_filename,content_type,file_size,upload_status,storage_provider) VALUES($1,$2,$3,'photo.jpg','image/jpeg',100,'complete','r2')`, id, user, id.String())
	}
	caption := "carousel"
	post, err := storage.NewPostRepository(db).CreatePost(ctx, user, uuid.NewString(), model.CreatePostInput{Caption: &caption, Targets: []uuid.UUID{account}, MediaIDs: media, InstagramFormats: map[string]string{account.String(): "CAROUSEL"}})
	if err != nil {
		t.Fatal(err)
	}
	repo := storage.NewPublicationRepository(db)
	targets, err := repo.GetPendingPostTargets(ctx, post.ID)
	if err != nil || len(targets) != 1 {
		t.Fatalf("load target: %v", err)
	}
	target := targets[0].ID
	format, ordered, err := repo.InstagramMedia(ctx, post.ID, target)
	if err != nil || format != "CAROUSEL" || len(ordered) != 2 || ordered[0] != media[0] || ordered[1] != media[1] {
		t.Fatalf("carousel order: %q %v %v", format, ordered, err)
	}
	claimed, err := repo.ClaimPostPublishing(ctx, post.ID, post.SchedulingGeneration)
	if err != nil || !claimed {
		t.Fatalf("claim: %v", err)
	}
	attempt, err := repo.GetPublicationAttempt(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.MarkAttemptProcessing(ctx, attempt.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	next := now.Add(time.Minute)
	parent := "789"
	operation := storage.InstagramPublication{Children: []string{"234", "567"}, Container: &parent, SubmittedAt: &now, Deadline: now.Add(20 * time.Minute)}
	if err = repo.SaveInstagramStep(ctx, attempt.ID, operation, &next); err != nil {
		t.Fatal(err)
	}
	recovered, err := storage.NewPublicationRepository(db).GetInstagramPublication(ctx, attempt.ID)
	if err != nil || recovered.SubmittedAt == nil || recovered.Container == nil || *recovered.Container != parent || !recovered.NextCheck.After(now) {
		t.Fatalf("recover operation: %v", err)
	}
	var jobs int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND payload->>'attempt_id'=$2`, post.ID, attempt.ID.String()).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("recovery outbox: %d %v", jobs, err)
	}
	receipt := uuid.New()
	if err = accounts.RemoveInstagramGrant(ctx, "456", time.Now(), &receipt); err != nil {
		t.Fatal(err)
	}
	completed, err := accounts.InstagramDeletionCompleted(ctx, receipt)
	if err != nil || !completed {
		t.Fatalf("deletion receipt: %v", err)
	}
	recovered, err = repo.GetInstagramPublication(ctx, attempt.ID)
	if err != nil || recovered != nil {
		t.Fatalf("publication metadata retained after deletion: %v", err)
	}
}

func TestInstagramFeedUsesPreparedCopyOnlyForInstagram(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := fixture(t, db, nil)
	instagramID, original, prepared := uuid.New(), uuid.New(), uuid.New()
	execSQL(t, db, `INSERT INTO social_accounts(id,user_id,platform,platform_user_id,access_token) VALUES($1,$2,'instagram','123','encrypted')`, instagramID, f.userID)
	for _, media := range []struct {
		id          uuid.UUID
		contentType string
	}{{original, "image/png"}, {prepared, "image/jpeg"}} {
		execSQL(t, db, `INSERT INTO media(id,user_id,storage_key,original_filename,content_type,file_size,upload_status,storage_provider)
  VALUES($1,$2,$3,'photo',$4,100,'complete','r2')`, media.id, f.userID, media.id.String(), media.contentType)
	}
	caption := "cross-post"
	scheduled := time.Now().Add(time.Hour)
	input := model.CreatePostInput{Caption: &caption, MediaID: &original, InstagramFeedMediaID: &prepared, Targets: []uuid.UUID{f.accountID, instagramID},
		InstagramFormats: map[string]string{instagramID.String(): "IMAGE"}, ScheduledAt: &scheduled}
	repo := storage.NewPostRepository(db)
	key := uuid.NewString()
	post, err := repo.CreatePost(ctx, f.userID, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if post.MediaID == nil || *post.MediaID != original {
		t.Fatal("cross-post lost the original PNG")
	}
	var target uuid.UUID
	if err = db.QueryRow(ctx, `SELECT id FROM post_targets WHERE post_id=$1 AND social_account_id=$2`, post.ID, instagramID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	format, ids, err := storage.NewPublicationRepository(db).InstagramMedia(ctx, post.ID, target)
	if err != nil || format != "IMAGE" || len(ids) != 1 || ids[0] != prepared {
		t.Fatalf("incorrect Instagram attachment: %q %v %v", format, ids, err)
	}
	inUse, err := storage.NewMediaRepository(db).IsReferencedByPost(ctx, prepared)
	if err != nil || !inUse {
		t.Fatalf("prepared JPEG is not protected as a post attachment: %v", err)
	}
	input.InstagramFeedMediaID = &original
	if _, err = repo.CreatePost(ctx, f.userID, key, input); err == nil {
		t.Fatal("changed JPEG reference did not invalidate the idempotency key")
	}
	if _, err = repo.CreatePost(ctx, f.userID, uuid.NewString(), input); err == nil {
		t.Fatal("PNG was accepted as the prepared copy")
	}
}

func TestInstagramCarouselKeepsOrderedPreparedPhotosAndOriginalVideos(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := fixture(t, db, nil)
	instagramID, photo, video, jpeg := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	execSQL(t, db, `INSERT INTO social_accounts(id,user_id,platform,platform_user_id,access_token,instagram_username) VALUES($1,$2,'instagram','123','encrypted','blink.creator')`, instagramID, f.userID)
	for _, media := range []struct {
		id   uuid.UUID
		mime string
	}{{photo, "image/webp"}, {video, "video/mp4"}, {jpeg, "image/jpeg"}} {
		execSQL(t, db, `INSERT INTO media(id,user_id,storage_key,original_filename,content_type,file_size,upload_status,storage_provider)
  VALUES($1,$2,$3,'asset',$4,100,'complete','r2')`, media.id, f.userID, media.id.String(), media.mime)
	}
	caption := "carousel with a video"
	input := model.CreatePostInput{Caption: &caption, MediaIDs: []uuid.UUID{video, photo}, InstagramMediaIDs: []uuid.UUID{video, jpeg},
		Targets: []uuid.UUID{instagramID}, InstagramFormats: map[string]string{instagramID.String(): "CAROUSEL"}}
	posts := storage.NewPostRepository(db)
	post, err := posts.CreatePost(ctx, f.userID, uuid.NewString(), input)
	if err != nil {
		t.Fatal(err)
	}
	var target uuid.UUID
	if err = db.QueryRow(ctx, `SELECT id FROM post_targets WHERE post_id=$1`, post.ID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	format, ids, err := storage.NewPublicationRepository(db).InstagramMedia(ctx, post.ID, target)
	if err != nil || format != "CAROUSEL" || len(ids) != 2 || ids[0] != video || ids[1] != jpeg {
		t.Fatalf("unexpected publishing media: %q %v %v", format, ids, err)
	}
	saved, err := posts.GetPostWithDetails(ctx, f.userID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Targets) != 1 || saved.Targets[0].InstagramFormat == nil || *saved.Targets[0].InstagramFormat != "CAROUSEL" {
		t.Fatalf("incorrect saved targets: %v", saved.Targets)
	}
	if username := saved.Targets[0].PlatformUsername; username == nil || *username != "blink.creator" {
		t.Fatalf("missing account username on saved target: %v", username)
	}
	if len(saved.InstagramMedia) != 2 {
		t.Fatalf("saved carousel has %d items", len(saved.InstagramMedia))
	}
	for i, id := range []uuid.UUID{video, jpeg} {
		item := saved.InstagramMedia[i]
		if item.Position != i || item.MediaID == nil || *item.MediaID != id || item.ContentType == nil || *item.ContentType != []string{"video/mp4", "image/jpeg"}[i] {
			t.Fatalf("incorrect preview item %d: %+v", i, item)
		}
	}
	if _, err := posts.GetPostWithDetails(ctx, uuid.New(), post.ID); !errors.Is(err, storage.ErrPostNotFound) {
		t.Fatalf("another user can access the carousel: %v", err)
	}

	input.InstagramMediaIDs = []uuid.UUID{jpeg, video}
	if _, err = posts.CreatePost(ctx, f.userID, uuid.NewString(), input); err == nil {
		t.Fatal("accepted a JPEG replacement for the original video")
	}
	input.MediaIDs = []uuid.UUID{photo}
	input.InstagramMediaIDs = []uuid.UUID{jpeg}
	input.InstagramFormats[instagramID.String()] = "STORIES"
	if _, err = posts.CreatePost(ctx, f.userID, uuid.NewString(), input); err != nil {
		t.Fatalf("could not save a prepared Story photo: %v", err)
	}
}
