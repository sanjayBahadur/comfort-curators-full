package database

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"comfort-curators-backend/internal/platform/logging"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Migration struct {
	Version     int
	Description string
	SQL         string
	Checksum    string
}

func (m Migration) computeChecksum() string {
	h := sha256.New()
	h.Write([]byte(m.SQL))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (m Migration) ValidateChecksum() error {
	expected := m.computeChecksum()
	if expected != m.Checksum {
		return fmt.Errorf("migration %d checksum mismatch: stored %s != computed %s", m.Version, m.Checksum, expected)
	}
	return nil
}

type Runner struct {
	db *DB
}

func NewRunner(db *DB) *Runner {
	return &Runner{db: db}
}

func (r *Runner) Run(ctx context.Context) error {
	migrations, err := discoverMigrations()
	if err != nil {
		return fmt.Errorf("migrations: discover: %w", err)
	}

	if len(migrations) == 0 {
		logging.Info(ctx, "no migrations found")
		return nil
	}

	if err := r.ensureHistoryTable(ctx); err != nil {
		return fmt.Errorf("migrations: ensure history table: %w", err)
	}

	applied, err := r.appliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("migrations: list applied: %w", err)
	}

	if err := r.checkDrift(ctx, migrations, applied); err != nil {
		return err
	}

	pending := pendingMigrations(migrations, applied)
	if len(pending) == 0 {
		logging.Info(ctx, "database is up to date", "applied", len(applied))
		return nil
	}

	logging.Info(ctx, "running pending migrations", "count", len(pending))
	for _, m := range pending {
		adopted, err := r.adoptBaseline(ctx, m)
		if err != nil {
			return fmt.Errorf("migrations: baseline check %d (%s): %w", m.Version, m.Description, err)
		}
		if adopted {
			continue
		}
		if err := r.apply(ctx, m); err != nil {
			return fmt.Errorf("migrations: apply %d (%s): %w", m.Version, m.Description, err)
		}
	}

	return nil
}

// baselineMarker matches the directive a baseline migration carries in its
// header comment, e.g. "-- baseline-if-exists: properties".
var baselineMarker = regexp.MustCompile(`(?m)^--\s*baseline-if-exists:\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*$`)

// adoptBaseline handles the one-off problem of introducing migrations to a
// database whose schema was built some other way.
//
// Before P1-04, 143 of these tables were created at every application start by
// module EnsureSchema functions rather than by a migration. Existing databases
// therefore already have the whole schema but no migration record for it, and
// running the baseline against them would fail on the first CREATE TABLE.
//
// A migration may declare "-- baseline-if-exists: <table>". If that table is
// already present, the database is taken to be at this version already: the
// migration is recorded as applied without being executed. A fresh database
// does not have the table, so it runs normally.
//
// This applies only to a migration that declares the marker. Ordinary
// migrations always execute — silently skipping one because something already
// exists is how a schema drifts away from its history.
func (r *Runner) adoptBaseline(ctx context.Context, m Migration) (bool, error) {
	match := baselineMarker.FindStringSubmatch(m.SQL)
	if match == nil {
		return false, nil
	}
	sentinel := match[1]

	var exists bool
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1
		)`, sentinel).Scan(&exists); err != nil {
		return false, fmt.Errorf("check sentinel table %q: %w", sentinel, err)
	}
	if !exists {
		return false, nil
	}

	if _, err := r.db.Pool.Exec(ctx,
		`INSERT INTO schema_migrations (version, description, checksum) VALUES ($1, $2, $3)`,
		m.Version, m.Description, m.Checksum,
	); err != nil {
		return false, fmt.Errorf("record baseline: %w", err)
	}

	logging.Info(ctx, "migration adopted as baseline; schema already present",
		"version", m.Version,
		"description", m.Description,
		"sentinel", sentinel,
	)
	return true, nil
}

func (r *Runner) ensureHistoryTable(ctx context.Context) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			checksum    TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`
	_, err := r.db.Pool.Exec(ctx, ddl)
	return err
}

func (r *Runner) appliedMigrations(ctx context.Context) (map[int]Migration, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT version, description, checksum, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]Migration)
	for rows.Next() {
		var m Migration
		var appliedAt time.Time
		if err := rows.Scan(&m.Version, &m.Description, &m.Checksum, &appliedAt); err != nil {
			return nil, err
		}
		applied[m.Version] = m
	}
	return applied, rows.Err()
}

func (r *Runner) checkDrift(ctx context.Context, migrations []Migration, applied map[int]Migration) error {
	for _, m := range migrations {
		appliedVersion, wasApplied := applied[m.Version]
		if !wasApplied {
			continue
		}
		if appliedVersion.Checksum != m.Checksum {
			logging.Error(ctx, "migration checksum drift detected",
				"version", m.Version,
				"description", m.Description,
				"stored_checksum", appliedVersion.Checksum,
				"current_checksum", m.Checksum,
			)
			return fmt.Errorf(
				"migration checksum drift at version %d (%s): stored=%s current=%s",
				m.Version, m.Description, appliedVersion.Checksum, m.Checksum,
			)
		}
	}
	return nil
}

func (r *Runner) apply(ctx context.Context, m Migration) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("exec sql: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, description, checksum) VALUES ($1, $2, $3)`,
		m.Version, m.Description, m.Checksum,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	logging.Info(ctx, "migration applied",
		"version", m.Version,
		"description", m.Description,
	)
	return nil
}

func discoverMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, description, err := parseMigrationName(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid migration filename %s: %w", entry.Name(), err)
		}

		sqlBytes, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", entry.Name(), err)
		}
		sql := string(sqlBytes)

		m := Migration{
			Version:     version,
			Description: description,
			SQL:         sql,
			Checksum:    computeChecksumString(sql),
		}
		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func parseMigrationName(name string) (int, string, error) {
	base := strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("expected format NNN_description.sql")
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid version number %q: %w", parts[0], err)
	}
	description := strings.ReplaceAll(parts[1], "_", " ")
	return version, description, nil
}

func computeChecksumString(sql string) string {
	h := sha256.New()
	h.Write([]byte(sql))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func pendingMigrations(migrations []Migration, applied map[int]Migration) []Migration {
	var pending []Migration
	for _, m := range migrations {
		if _, ok := applied[m.Version]; !ok {
			pending = append(pending, m)
		}
	}
	return pending
}

func RunMigrations(ctx context.Context, db *DB) error {
	runner := NewRunner(db)
	return runner.Run(ctx)
}
