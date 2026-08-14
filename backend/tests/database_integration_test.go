package tests

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

func postgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func dbConfig() (config.Config, bool) {
	if !postgresAvailable() {
		return config.Config{}, false
	}

	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}

	cfg := config.Config{
		DBHost: host,
		DBPort: 5432,
		DBUser: user,
		DBPass: pass,
		DBName: name,
		DBSSL:  "disable",
	}

	db, err := database.Connect(context.Background(), cfg)
	if err != nil {
		return cfg, false
	}
	defer db.Close()

	_, err = db.Pool.Exec(context.Background(), `SELECT 1`)
	if err != nil {
		return cfg, false
	}

	return cfg, true
}

func TestDatabaseEmptyDatabaseMigrates(t *testing.T) {
	cfg, ok := dbConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	_, err = db.Pool.Exec(ctx, `DROP TABLE IF EXISTS schema_migrations`)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	err = database.RunMigrations(ctx, db)
	if err != nil {
		t.Fatalf("migrate empty database: %v", err)
	}

	var count int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 migration applied to empty database, got %d", count)
	}

	t.Logf("applied %d migrations to empty database", count)

	row := db.Pool.QueryRow(ctx,
		`SELECT version, description FROM schema_migrations ORDER BY version LIMIT 1`)
	var version int
	var description string
	if err := row.Scan(&version, &description); err != nil {
		t.Fatalf("read first migration: %v", err)
	}
	t.Logf("first migration: v%d (%s)", version, description)

	err = database.RunMigrations(ctx, db)
	if err != nil {
		t.Fatalf("re-run migrations on up-to-date database: %v", err)
	}
}

func TestDatabaseMigrationChecksumDriftFails(t *testing.T) {
	cfg, ok := dbConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	_, err = db.Pool.Exec(ctx, `DROP TABLE IF EXISTS schema_migrations`)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	err = database.RunMigrations(ctx, db)
	if err != nil {
		t.Fatalf("initial migration: %v", err)
	}

	_, err = db.Pool.Exec(ctx,
		`UPDATE schema_migrations SET checksum = 'deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef' WHERE version = (SELECT MAX(version) FROM schema_migrations)`)
	if err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}

	err = database.RunMigrations(ctx, db)
	if err == nil {
		t.Fatal("expected checksum drift error, got nil")
	}
	t.Logf("checksum drift correctly detected: %v", err)
}

func TestDatabaseTransactionsRollBackOnHandlerFailure(t *testing.T) {
	cfg, ok := dbConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	_, err = db.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_test (id SERIAL PRIMARY KEY, val TEXT)`)
	if err != nil {
		t.Fatalf("create test table: %v", err)
	}
	defer func() {
		db.Pool.Exec(ctx, `DROP TABLE IF EXISTS tx_test`)
	}()

	_, err = db.Pool.Exec(ctx, `DELETE FROM tx_test`)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	err = database.WithTx(ctx, db.Pool, func(ctx context.Context, tx pgx.Tx) error {
		return fmt.Errorf("handler failed")
	})

	if err == nil {
		t.Fatal("expected error from handler, got nil")
	}

	var count int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM tx_test`).Scan(&count)
	if err != nil {
		t.Fatalf("count tx_test: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}

	err = database.WithTx(ctx, db.Pool, func(ctx context.Context, tx pgx.Tx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("successful tx failed: %v", err)
	}
}
