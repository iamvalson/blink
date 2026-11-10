package tests

import (
	"context"
	"testing"
	"time"

	"github.com/iamvalson/blink/internal/storage"
)

func TestConnectedAccountNamesReachPostDetails(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := fixture(t, db, nil)
	accounts := storage.NewSocialAccountRepository(db)
	userID := f.userID.String()
	names := map[string]string{"twitter": "blink_on_x", "youtube": "Blink Channel", "tiktok": "Blink Creator"}
	for platform, name := range names {
		if err := accounts.Upsert(ctx, userID, platform, "test", "ciphertext", "", time.Now().Add(time.Hour),
			storage.SocialAccountMetadata{Username: name, AvatarURL: "https://cdn.example/" + platform + ".jpg"}); err != nil {
			t.Fatal(err)
		}
		if platform != "twitter" {
			execSQL(t, db, `INSERT INTO post_targets(post_id,social_account_id,status)
    SELECT $1,id,'PENDING' FROM social_accounts WHERE user_id=$2 AND platform=$3`, f.postID, f.userID, platform)
		}
		// An optional profile lookup can fail during reconnect. Keep the saved name
		// for the same provider identity.
		if err := accounts.Upsert(ctx, userID, platform, "test", "ciphertext", "", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	repo := storage.NewPostRepository(db)
	post, err := repo.GetPostWithDetails(ctx, f.userID, f.postID)
	if err != nil {
		t.Fatal(err)
	}
	if len(post.Targets) != len(names) {
		t.Fatalf("expected %d targets, got %d", len(names), len(post.Targets))
	}
	for _, target := range post.Targets {
		if target.PlatformAvatarURL == nil || *target.PlatformAvatarURL != "https://cdn.example/"+target.Platform+".jpg" {
			t.Fatalf("incorrect avatar for %s: %v", target.Platform, target.PlatformAvatarURL)
		}
		if target.PlatformUsername == nil || *target.PlatformUsername != names[target.Platform] {
			t.Fatalf("incorrect name for %s: %v", target.Platform, target.PlatformUsername)
		}
	}

	if err := accounts.Upsert(ctx, userID, "twitter", "different-account", "ciphertext", "", time.Now().Add(time.Hour), storage.SocialAccountMetadata{AvatarURL: "http://cdn.example/insecure.jpg"}); err != nil {
		t.Fatal(err)
	}
	post, err = repo.GetPostWithDetails(ctx, f.userID, f.postID)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range post.Targets {
		if target.Platform == "twitter" && (target.PlatformUsername != nil || target.PlatformAvatarURL != nil) {
			t.Fatal("replaced account inherited previous metadata or accepted an insecure avatar")
		}
	}
}
