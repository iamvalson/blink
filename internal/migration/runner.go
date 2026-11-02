package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type Runner struct {
	files    fs.FS
	engine   *migrate.Migrate
	db       *sql.DB
	latest   uint
	versions map[uint]bool
}
type Status struct {
	Version        uint
	Dirty          bool
	Empty          bool
	ExistingSchema bool
	Latest         uint
}

func Open(databaseURL string, files fs.FS) (*Runner, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL for migration runner")
	}
	return OpenConfig(cfg, files)
}

// OpenConfig preserves runtime parameters such as an isolated test search_path.
func OpenConfig(config *pgx.ConnConfig, files fs.FS) (*Runner, error) {
	cfg := config.Copy()
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(3)
	source, err := iofs.New(files, ".")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("load embedded migrations: %w", err)
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		source.Close()
		db.Close()
		return nil, err
	}
	versions := map[uint]bool{}
	var latest uint
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		part, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			source.Close()
			db.Close()
			return nil, errors.New("invalid embedded migration filename")
		}
		v := uint(n)
		versions[v] = true
		if v > latest {
			latest = v
		}
	}
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{StatementTimeout: 2 * time.Minute})
	if err != nil {
		source.Close()
		db.Close()
		return nil, errors.New("connect/initialize migration database failed; check DATABASE_URL and schema permissions")
	}
	engine, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		driver.Close()
		source.Close()
		return nil, err
	}
	engine.LockTimeout = 30 * time.Second
	return &Runner{files, engine, db, latest, versions}, nil
}
func (r *Runner) Close() error {
	sourceErr, dbErr := r.engine.Close()
	return errors.Join(sourceErr, dbErr)
}
func (r *Runner) Status() (Status, error) {
	status := Status{Latest: r.latest}
	version, dirty, err := r.engine.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		status.Empty = true
	} else if err != nil {
		return status, err
	} else {
		status.Version = version
		status.Dirty = dirty
	}
	err = r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('users','posts','post_targets','social_accounts'))`).Scan(&status.ExistingSchema)
	return status, err
}

// This transaction lock serializes all Blink commands, including baseline's
// precondition checks. golang-migrate also holds its own database migration lock.
func (r *Runner) guard() (*sql.Tx, error) {
	tx, err := r.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	var locked bool
	err = tx.QueryRow(`SELECT pg_try_advisory_xact_lock(hashtextextended(current_database() || ':' || current_schema() || ':blink-migration-runner',0))`).Scan(&locked)
	if err != nil || !locked {
		tx.Rollback()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("another Blink migration operation is running; retry when it completes")
	}
	return tx, nil
}
func (r *Runner) Up() error {
	lock, err := r.guard()
	if err != nil {
		return err
	}
	defer lock.Rollback()
	status, err := r.Status()
	if err != nil {
		return err
	}
	if status.Empty && status.ExistingSchema {
		return errors.New("existing Blink schema has no migration history; verify its applied version and run migrate baseline -version N before up")
	}
	if !status.Empty && status.Version > r.latest {
		return errors.New("database schema is newer than this binary's embedded migrations")
	}
	err = r.engine.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Down reverts one migration, never the entire schema.
func (r *Runner) Down() error {
	lock, err := r.guard()
	if err != nil {
		return err
	}
	defer lock.Rollback()
	status, err := r.Status()
	if err != nil {
		return err
	}
	if status.Empty {
		return nil
	}
	if status.Version > r.latest {
		return errors.New("database schema is newer than this binary's embedded migrations")
	}
	err = r.engine.Steps(-1)
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Baseline records an existing version after rollback-only schema verification.
// It cannot repair dirty history or overwrite any existing tracked version.
func (r *Runner) Baseline(version uint) error {
	lock, err := r.guard()
	if err != nil {
		return err
	}
	defer lock.Rollback()
	if !r.versions[version] {
		return errors.New("baseline version must match an embedded migration")
	}
	status, err := r.Status()
	if err != nil {
		return err
	}
	if !status.Empty {
		return errors.New("baseline is only allowed on an untracked database; existing or dirty history cannot be overwritten")
	}
	if !status.ExistingSchema {
		return errors.New("baseline requires an existing Blink schema; use up for an empty database")
	}
	if err := r.verifyBaseline(lock, version); err != nil {
		return err
	}
	return r.engine.Force(int(version))
}
