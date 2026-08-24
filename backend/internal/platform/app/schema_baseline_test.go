package app

import (
	"context"
	"os"
	"testing"

	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/testdb"
)

// TestGenerateSchemaBaseline provisions an empty database exactly the way the
// application does at startup — RunMigrations, then all 24 module
// EnsureSchema functions — so the result can be dumped into a migration
// baseline (P1-04).
//
// It exists because the schema currently has no single source of truth. 147
// tables are created by CREATE TABLE IF NOT EXISTS scattered across module
// migrate.go files, which means an ALTER to an existing table is silently
// skipped and the only way to change a column is `docker compose down -v`.
//
// Reproducing the schema by hand-copying those DDL strings would be a
// transcription exercise with 147 chances to be subtly wrong. Running the real
// code path against an empty database and dumping the result cannot drift from
// what the application actually builds.
//
// Not part of the normal suite: provisioning is slow and this is a build step,
// not an assertion. Run it deliberately:
//
//	CC_GENERATE_BASELINE=1 CC_DB_NAME=comfort_curators_baseline_test \
//	  CC_DB_NAME_EXACT=1 go test ./internal/platform/app/ -run TestGenerateSchemaBaseline -v
//
// then dump the database it built:
//
//	pg_dump --schema-only --no-owner --no-privileges comfort_curators_baseline_test
func TestGenerateSchemaBaseline(t *testing.T) {
	if os.Getenv("CC_GENERATE_BASELINE") != "1" {
		t.Skip("set CC_GENERATE_BASELINE=1 to provision a baseline database")
	}

	cfg := testdb.Config(t)
	t.Logf("provisioning baseline schema in %q", cfg.DBName)

	ctx := context.Background()
	db, err := database.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to %s: %v", cfg.DBName, err)
	}
	defer db.Close()

	if err := initializeSchema(ctx, db); err != nil {
		t.Fatalf("initializeSchema: %v", err)
	}

	var tables int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		  WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&tables); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	t.Logf("provisioned %d tables in %s", tables, cfg.DBName)

	// The count is a smoke test, not the deliverable. If it drops sharply,
	// a module stopped being wired into initializeSchema.
	if tables < 100 {
		t.Fatalf("only %d tables provisioned; expected the full module schema", tables)
	}
}
