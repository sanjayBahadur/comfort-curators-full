// Package testdb builds connection settings for the Go test suite and refuses
// to hand out a connection to anything that is not a disposable test database.
//
// Before this package existed, every test file carried its own copy of the
// connection defaults, and those defaults pointed at "comfort_curators" — the
// database the running application uses. Several tests TRUNCATE tables, and
// tests/database_integration_test.go deliberately poisons a schema_migrations
// checksum to exercise drift detection. Running the suite against a live stack
// therefore wiped data and left the API unable to boot.
//
// The rule this package enforces: the target database name must end in
// "_test". A name that does not is a hard failure, never a skip — a guard
// nobody sees fire is not a guard.
package testdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultName is the database every test connects to unless CC_DB_NAME says
// otherwise. It must never be the application database.
const DefaultName = "comfort_curators_test"

// requiredSuffix is the whole guard. Any resolved database name that does not
// end in this is rejected.
const requiredSuffix = "_test"

// ErrUnsafeName is returned by ValidateName for a database this suite must not
// be allowed to touch.
var ErrUnsafeName = errors.New("testdb: refusing to use a database whose name does not end in " + requiredSuffix)

// Settings are the resolved connection parameters.
type Settings struct {
	Host string
	Port int
	User string
	Pass string
	Name string
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// packageName derives a short identifier for the package under test from the
// test binary's name, which Go builds as "<pkg>.test". It is the only handle a
// test has on its own identity without every call site passing one in.
func packageName() string {
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".test")
	base = strings.TrimSuffix(base, ".exe")

	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return ""
	}
	// Postgres truncates identifiers at 63 bytes; leave room for the affixes.
	if len(name) > 24 {
		name = name[:24]
	}
	return name
}

// isolatedName gives each test package its own database.
//
// Every package previously shared one database, and that sharing is what makes
// the suite red: tests/database_integration_test.go deliberately poisons a
// schema_migrations checksum and does not restore it, so every package that
// subsequently runs migrations fails with "migration checksum drift at version
// 4". Packages also TRUNCATE tables out from under each other.
//
// The name still ends in _test, so the guard applies unchanged.
func isolatedName(base string) string {
	pkg := packageName()
	if pkg == "" {
		return base
	}
	trimmed := strings.TrimSuffix(base, requiredSuffix)
	return trimmed + "_" + pkg + requiredSuffix
}

// Resolve reads the environment and applies defaults. It does not validate;
// callers that are about to connect must run ValidateName on the result.
//
// The resolved name is per-package unless CC_DB_NAME_EXACT is set, which
// forces a single shared database (used by the guard's own tests).
func Resolve() Settings {
	port, err := strconv.Atoi(env("CC_DB_PORT", "5432"))
	if err != nil || port <= 0 {
		port = 5432
	}
	name := env("CC_DB_NAME", DefaultName)
	if os.Getenv("CC_DB_NAME_EXACT") == "" {
		name = isolatedName(name)
	}

	return Settings{
		Host: env("CC_DB_HOST", "localhost"),
		Port: port,
		User: env("CC_DB_USER", "ccuser"),
		Pass: env("CC_DB_PASS", "ccpass"),
		Name: name,
	}
}

// ValidateName is the guard, kept free of *testing.T so it can be asserted on
// directly. See TestGuardRejectsApplicationDatabase.
func ValidateName(name string) error {
	if len(name) <= len(requiredSuffix) || name[len(name)-len(requiredSuffix):] != requiredSuffix {
		return fmt.Errorf("%w (got %q)", ErrUnsafeName, name)
	}
	return nil
}

// MustName resolves the test database name and panics if it is not a
// disposable one. It exists because the per-file helpers it replaces are
// plain functions with no *testing.T in scope, and threading one through 36
// files would have turned a mechanical substitution into a bespoke refactor.
//
// A panic is the right failure here: it aborts the package's test binary with
// a stack trace, so the run fails loudly and visibly. The alternative that
// must never be used is returning an error the caller can turn into a skip.
func MustName() string {
	s := Resolve()
	if err := ValidateName(s.Name); err != nil {
		panic(fmt.Sprintf("%v\n\nThe test suite may only run against a disposable "+
			"database. Unset CC_DB_NAME to use the default %q, or set it to a name "+
			"ending in %q.", err, DefaultName, requiredSuffix))
	}
	// Callers build their own connection string and connect directly, so this
	// is the only place the per-package database can be created. Done once per
	// test binary; a failure here is left to surface as the caller's own
	// connection error, which carries better context.
	ensureOnce.Do(func() {
		if Available() {
			_ = ensureDatabase(s)
		}
	})
	return s.Name
}

var ensureOnce sync.Once

// EnsureExists creates the test database if it is missing. Callers that build
// their own connection string via MustName use this to get the same
// create-on-demand behaviour the *testing.T helpers provide.
func EnsureExists() error {
	s := Resolve()
	if err := ValidateName(s.Name); err != nil {
		return err
	}
	return ensureDatabase(s)
}

// ConnString renders a libpq URL for the given database name.
func (s Settings) ConnString(dbName string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		s.User, s.Pass, s.Host, s.Port, dbName)
}

// Config converts the settings into the application config shape.
func (s Settings) Config() config.Config {
	return config.Config{
		DBHost: s.Host,
		DBPort: s.Port,
		DBUser: s.User,
		DBPass: s.Pass,
		DBName: s.Name,
		DBSSL:  "disable",
	}
}

// Available reports whether a TCP connection to the configured host and port
// succeeds. Reachability is checked before the name guard so that a machine
// with no Postgres still skips rather than fails.
func Available() bool {
	s := Resolve()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(s.Host, strconv.Itoa(s.Port)), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// prepare runs the full gate in the order that matters: reachability (skip),
// then the name guard (fatal), then ensure the database exists.
func prepare(t *testing.T) Settings {
	t.Helper()

	if !Available() {
		t.Skip("PostgreSQL not available on the configured host and port; skipping")
	}

	s := Resolve()
	if err := ValidateName(s.Name); err != nil {
		t.Fatalf("%v\n\nThe test suite may only run against a disposable database. "+
			"Unset CC_DB_NAME to use the default %q, or set it to a name ending in %q.",
			err, DefaultName, requiredSuffix)
	}

	if err := ensureDatabase(s); err != nil {
		t.Fatalf("testdb: could not ensure database %q exists: %v", s.Name, err)
	}
	return s
}

// ensureDatabase creates the test database if it is missing, so a fresh
// checkout needs no manual setup. It connects to the "postgres" maintenance
// database, which is exempt from the name guard because it is never written to
// here — only used to issue CREATE DATABASE.
func ensureDatabase(s Settings) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, s.ConnString("postgres"))
	if err != nil {
		return fmt.Errorf("connect to maintenance database: %w", err)
	}
	defer admin.Close(ctx)

	var exists bool
	err = admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, s.Name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check for database: %w", err)
	}
	if exists {
		return nil
	}

	// Identifiers cannot be parameterised. s.Name has passed ValidateName, and
	// pgx.Identifier quotes it, so this is safe against a hostile CC_DB_NAME.
	stmt := fmt.Sprintf("CREATE DATABASE %s", pgx.Identifier{s.Name}.Sanitize())
	if _, err := admin.Exec(ctx, stmt); err != nil {
		// Another package's tests may have won the race. That is not a failure.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P04" {
			return nil
		}
		return fmt.Errorf("create database: %w", err)
	}
	return nil
}

// ConnString gates and returns a connection URL for the test database.
// Skips the test if Postgres is unreachable; fails it if the target is unsafe.
func ConnString(t *testing.T) string {
	t.Helper()
	s := prepare(t)
	return s.ConnString(s.Name)
}

// Config gates and returns the application config pointed at the test database.
func Config(t *testing.T) config.Config {
	t.Helper()
	return prepare(t).Config()
}

// Pool gates, connects, and returns a pgx pool closed at the end of the test.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	connStr := ConnString(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("testdb: create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("PostgreSQL not reachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Connect gates, connects, and returns the application database handle, closed
// at the end of the test.
func Connect(t *testing.T) *database.DB {
	t.Helper()
	cfg := Config(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, cfg)
	if err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
