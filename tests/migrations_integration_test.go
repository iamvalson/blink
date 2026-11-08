package tests

import (
	"context"
	"github.com/iamvalson/blink/internal/migration"
	"github.com/iamvalson/blink/migrations"
	"sync"
	"testing"
	"testing/fstest"
)

func TestTrackedMigrationUpDoesNotReplay(t *testing.T) {
	db := testDatabase(t)
	ctx := context.Background()
	f := fixture(t, db, nil)
	runner, err := migration.OpenConfig(db.Config().ConnConfig, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	for range 2 {
		if err := runner.Up(); err != nil {
			t.Fatal(err)
		}
	}
	status, err := runner.Status()
	if err != nil || status.Empty || status.Dirty || status.Version != 14 {
		t.Fatalf("invalid tracked version: %+v %v", status, err)
	}
	if postStatus(t, db, f.postID) != "QUEUED" {
		t.Fatal("migration replay altered existing post")
	}
	var users int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE id=$1`, f.userID).Scan(&users); err != nil || users != 1 {
		t.Fatal("existing data changed")
	}
}
func TestTrackedMigrationDownRevertsOnlyLatest(t *testing.T) {
	db := testDatabase(t)
	runner, err := migration.OpenConfig(db.Config().ConnConfig, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Down(); err != nil {
		t.Fatal(err)
	}
	status, err := runner.Status()
	if err != nil || status.Version != 13 || status.Dirty {
		t.Fatalf("down was not one step: %+v %v", status, err)
	}
	var postsExist bool
	if err := db.QueryRow(context.Background(), `SELECT to_regclass('posts') IS NOT NULL`).Scan(&postsExist); err != nil || !postsExist {
		t.Fatal("down removed earlier schema")
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}
func TestUntrackedExistingSchemaRequiresExplicitBaseline(t *testing.T) {
	db := testDatabase(t)
	f := fixture(t, db, nil)
	execSQL(t, db, `DROP TABLE schema_migrations`)
	runner, err := migration.OpenConfig(db.Config().ConnConfig, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Up(); err == nil {
		t.Fatal("replayed existing untracked schema")
	}
	if err := runner.Baseline(999); err == nil {
		t.Fatal("unknown baseline accepted")
	}
	if err := runner.Baseline(14); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Baseline(10); err == nil {
		t.Fatal("overwrote migration history")
	}
	if postStatus(t, db, f.postID) != "QUEUED" {
		t.Fatal("baseline changed post data")
	}
}
func TestFailedMigrationRemainsDirtyAndCannotBeBaselined(t *testing.T) {
	db := testDatabaseSetup(t, false)
	files := fstest.MapFS{
		"000001_start.up.sql": {Data: []byte("CREATE TABLE users(id INT);")}, "000001_start.down.sql": {Data: []byte("SELECT 1;")},
		"000002_failure.up.sql": {Data: []byte("CREATE TABLE should_rollback(id INT); SELECT deliberately_missing_function();")}, "000002_failure.down.sql": {Data: []byte("DROP TABLE IF EXISTS should_rollback;")},
	}
	runner, err := migration.OpenConfig(db.Config().ConnConfig, files)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Up(); err == nil {
		t.Fatal("broken migration succeeded")
	}
	status, err := runner.Status()
	if err != nil || !status.Dirty || status.Version != 2 {
		t.Fatalf("dirty failure not retained: %+v %v", status, err)
	}
	if err := runner.Baseline(1); err == nil {
		t.Fatal("dirty state overwritten by baseline")
	}
	if err := runner.Up(); err == nil {
		t.Fatal("dirty database silently retried")
	}
	var rolledBack bool
	if err := db.QueryRow(context.Background(), `SELECT to_regclass('should_rollback') IS NULL`).Scan(&rolledBack); err != nil || !rolledBack {
		t.Fatal("migration partially applied")
	}
}

func TestBaselineRefusesSchemaDrift(t *testing.T) {
	db := testDatabase(t)
	execSQL(t, db, `DROP TABLE schema_migrations`)
	execSQL(t, db, `ALTER TABLE posts ADD COLUMN unexpected_column TEXT`)
	runner, err := migration.OpenConfig(db.Config().ConnConfig, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Baseline(14); err == nil {
		t.Fatal("baseline accepted schema drift")
	}
	status, err := runner.Status()
	if err != nil || !status.Empty {
		t.Fatal("failed baseline altered history")
	}
	execSQL(t, db, `ALTER TABLE posts DROP COLUMN unexpected_column`)
	if err := runner.Baseline(10); err == nil {
		t.Fatal("baseline accepted wrong migration version")
	}
	if err := runner.Baseline(14); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentTrackedMigrationCommands(t *testing.T) {
	db := testDatabase(t)
	first, err := migration.OpenConfig(db.Config().ConnConfig, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := migration.OpenConfig(db.Config().ConnConfig, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, runner := range []*migration.Runner{first, second} {
		wg.Go(func() { results <- runner.Up() })
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("both migration commands failed")
	}
	if err := first.Up(); err != nil {
		t.Fatal(err)
	}
	status, err := second.Status()
	if err != nil || status.Version != 14 || status.Dirty {
		t.Fatalf("concurrent commands corrupted tracking: %+v %v", status, err)
	}
}
