package testdb_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"comfort-curators-backend/internal/platform/testdb"
)

// TestGuardRejectsApplicationDatabase is the reason this package exists.
// Running the suite against "comfort_curators" once wiped live tables and left
// the migration checksum poisoned so the API refused to boot. If this test ever
// stops failing the guard, the suite can destroy production data again.
func TestGuardRejectsApplicationDatabase(t *testing.T) {
	err := testdb.ValidateName("comfort_curators")
	if err == nil {
		t.Fatal("guard accepted the application database name; it must be rejected")
	}
	if !errors.Is(err, testdb.ErrUnsafeName) {
		t.Fatalf("expected ErrUnsafeName, got %v", err)
	}
}

func TestGuardRejectsUnsafeNames(t *testing.T) {
	unsafe := []string{
		"comfort_curators",
		"postgres",
		"",
		"_tes",
		"_test_backup",
		"comfort_curators_prod",
		"test",       // suffix must be preceded by something
		"_test",      // bare suffix, no database body
		"testdb",     // ends in "db", not "_test"
		"my_testing", // near miss
	}
	for _, name := range unsafe {
		if err := testdb.ValidateName(name); err == nil {
			t.Errorf("guard accepted unsafe database name %q", name)
		}
	}
}

func TestGuardAcceptsTestNames(t *testing.T) {
	safe := []string{
		testdb.DefaultName,
		"comfort_curators_test",
		"cc_test",
		"anything_test",
	}
	for _, name := range safe {
		if err := testdb.ValidateName(name); err != nil {
			t.Errorf("guard rejected safe database name %q: %v", name, err)
		}
	}
}

// TestDefaultIsNotTheApplicationDatabase pins the default, so a future edit
// cannot quietly restore the old one.
func TestDefaultIsNotTheApplicationDatabase(t *testing.T) {
	if testdb.DefaultName == "comfort_curators" {
		t.Fatal("default test database is the application database")
	}
	if err := testdb.ValidateName(testdb.DefaultName); err != nil {
		t.Fatalf("default database name does not satisfy the guard: %v", err)
	}
}

// TestEnvOverrideIsStillGuarded proves the guard applies to an explicitly
// supplied name, not only to the default.
func TestEnvOverrideIsStillGuarded(t *testing.T) {
	t.Setenv("CC_DB_NAME", "comfort_curators")
	t.Setenv("CC_DB_NAME_EXACT", "1")

	s := testdb.Resolve()
	if s.Name != "comfort_curators" {
		t.Fatalf("expected the override to be resolved, got %q", s.Name)
	}
	if err := testdb.ValidateName(s.Name); err == nil {
		t.Fatal("guard accepted an overridden application database name")
	}
}

// TestConnStringFatalsOnUnsafeName is the end-to-end falsification: it points a
// real subprocess at the application database and asserts the run FAILS rather
// than skips. ValidateName being correct is not enough — the connection path
// has to actually consult it, and it has to be fatal. A skip here would mean
// the suite silently ran against production data.
func TestConnStringFatalsOnUnsafeName(t *testing.T) {
	if os.Getenv("TESTDB_GUARD_SUBPROCESS") == "1" {
		// In the child. This must abort the test binary.
		testdb.ConnString(t)
		t.Error("ConnString returned instead of failing on an unsafe database name")
		return
	}

	if !testdb.Available() {
		t.Skip("PostgreSQL not available; the guard runs after the reachability check")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestConnStringFatalsOnUnsafeName$", "-test.v")
	cmd.Env = append(os.Environ(),
		"TESTDB_GUARD_SUBPROCESS=1",
		"CC_DB_NAME=comfort_curators",
		"CC_DB_NAME_EXACT=1",
	)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err == nil {
		t.Fatalf("guard did not fire: subprocess targeting comfort_curators exited 0.\n%s", output)
	}
	if strings.Contains(output, "--- SKIP") {
		t.Fatalf("guard skipped instead of failing; a silent skip is how this goes unnoticed.\n%s", output)
	}
	if !strings.Contains(output, "comfort_curators") {
		t.Errorf("failure message does not name the offending database.\n%s", output)
	}
	if !strings.Contains(output, testdb.DefaultName) {
		t.Errorf("failure message does not tell the reader how to fix it.\n%s", output)
	}
}

// TestConnectCreatesAndUsesTestDatabase exercises the happy path end to end:
// the default name passes the guard, the database is created if missing, and
// the connection lands on it and not on the application database.
func TestConnectCreatesAndUsesTestDatabase(t *testing.T) {
	if !testdb.Available() {
		t.Skip("PostgreSQL not available")
	}

	pool := testdb.Pool(t)

	var current string
	if err := pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&current); err != nil {
		t.Fatalf("querying current database: %v", err)
	}
	if !strings.HasPrefix(current, "comfort_curators_test") {
		t.Fatalf("connected to %q, want a per-package comfort_curators_test* database", current)
	}
	if current == "comfort_curators" {
		t.Fatal("connected to the application database")
	}
}

func TestResolveDefaults(t *testing.T) {
	t.Setenv("CC_DB_HOST", "")
	t.Setenv("CC_DB_PORT", "")
	t.Setenv("CC_DB_NAME", "")
	t.Setenv("CC_DB_NAME_EXACT", "1")

	s := testdb.Resolve()
	if s.Host != "localhost" {
		t.Errorf("host = %q, want localhost", s.Host)
	}
	if s.Port != 5432 {
		t.Errorf("port = %d, want 5432", s.Port)
	}
	if s.Name != testdb.DefaultName {
		t.Errorf("name = %q, want %q", s.Name, testdb.DefaultName)
	}
}
