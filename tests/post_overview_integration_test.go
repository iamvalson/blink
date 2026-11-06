package tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/storage"
)

func TestFailedPostsIncludePartialPublications(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	execSQL(t, db, `UPDATE posts SET status='FAILED' WHERE id=$1`, f.postID)
	execSQL(t, db, `UPDATE post_targets SET status='FAILED' WHERE id=$1`, f.targetID)
	partialID, publishedID := uuid.New(), uuid.New()
	execSQL(t, db, `INSERT INTO posts(id,user_id,caption,status) VALUES($1,$2,'partial','PARTIALLY_PUBLISHED'),($3,$2,'published','PUBLISHED')`, partialID, f.userID, publishedID)
	execSQL(t, db, `INSERT INTO post_targets(post_id,social_account_id,status) VALUES($1,$2,'FAILED')`, partialID, f.accountID)
	other := fixture(t, db, nil)
	execSQL(t, db, `UPDATE posts SET status='FAILED' WHERE id=$1`, other.postID)

	repo := storage.NewPostRepository(db)
	ctx := context.Background()
	posts, total, err := repo.ListPosts(ctx, f.userID, storage.ListPostsFilter{Status: "FAILED", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(posts) != 1 {
		t.Fatalf("failed count/page = %d/%d, want 2/1", total, len(posts))
	}
	second, secondTotal, err := repo.ListPosts(ctx, f.userID, storage.ListPostsFilter{Status: "FAILED", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if secondTotal != 2 || len(second) != 1 || posts[0].ID == second[0].ID {
		t.Fatalf("failed pagination did not preserve the total and distinct posts: %+v", second)
	}
	statuses := map[string]bool{posts[0].Status: true, second[0].Status: true}
	if !statuses["FAILED"] || !statuses["PARTIALLY_PUBLISHED"] {
		t.Fatalf("original statuses were lost: %v", statuses)
	}
	partial, count, err := repo.ListPosts(ctx, f.userID, storage.ListPostsFilter{Status: "PARTIALLY_PUBLISHED"})
	if err != nil || count != 1 || len(partial) != 1 || partial[0].ID != partialID {
		t.Fatalf("partial-only filter regressed: posts=%+v total=%d err=%v", partial, count, err)
	}
	filtered, count, err := repo.ListPosts(ctx, f.userID, storage.ListPostsFilter{Status: "FAILED", Platform: "twitter", Search: "partial"})
	if err != nil || count != 1 || len(filtered) != 1 || filtered[0].ID != partialID {
		t.Fatalf("combined filters failed: posts=%+v total=%d err=%v", filtered, count, err)
	}
}
