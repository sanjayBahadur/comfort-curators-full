package database

import (
	"testing"
)

func TestParseMigrationName(t *testing.T) {
	tests := []struct {
		filename    string
		wantVersion int
		wantDesc    string
		wantErr     bool
	}{
		{"001_initial_schema_snapshot.sql", 1, "initial schema snapshot", false},
		{"010_create_users_table.sql", 10, "create users table", false},
		{"100_add_index_on_tenant_id.sql", 100, "add index on tenant id", false},
		{"1_simple.sql", 1, "simple", false},
		{"notanumber_desc.sql", 0, "", true},
		{"001.sql", 0, "", true},
		{"abc.sql", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			version, desc, err := parseMigrationName(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMigrationName(%q) error = %v, wantErr = %v", tt.filename, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if version != tt.wantVersion {
				t.Errorf("parseMigrationName(%q) version = %d, want %d", tt.filename, version, tt.wantVersion)
			}
			if desc != tt.wantDesc {
				t.Errorf("parseMigrationName(%q) desc = %q, want %q", tt.filename, desc, tt.wantDesc)
			}
		})
	}
}

func TestComputeChecksumString(t *testing.T) {
	sql1 := "SELECT 1;"
	sql2 := "SELECT 2;"

	cs1a := computeChecksumString(sql1)
	cs1b := computeChecksumString(sql1)
	cs2 := computeChecksumString(sql2)

	if cs1a != cs1b {
		t.Error("checksum for same SQL should be identical")
	}
	if cs1a == cs2 {
		t.Error("checksum for different SQL should differ")
	}
	if len(cs1a) != 64 {
		t.Errorf("SHA-256 hex string should be 64 chars, got %d", len(cs1a))
	}
}

func TestMigrationValidateChecksum(t *testing.T) {
	m := Migration{
		Version:     1,
		Description: "test",
		SQL:         "SELECT 1;",
		Checksum:    computeChecksumString("SELECT 1;"),
	}
	if err := m.ValidateChecksum(); err != nil {
		t.Errorf("valid checksum should pass validation: %v", err)
	}

	m.Checksum = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := m.ValidateChecksum(); err == nil {
		t.Error("invalid checksum should fail validation")
	}
}

func TestPendingMigrations(t *testing.T) {
	migrations := []Migration{
		{Version: 1},
		{Version: 2},
		{Version: 3},
		{Version: 4},
		{Version: 5},
	}

	applied := map[int]Migration{
		1: {Version: 1},
		2: {Version: 2},
		3: {Version: 3},
	}

	pending := pendingMigrations(migrations, applied)
	if len(pending) != 2 {
		t.Errorf("expected 2 pending migrations, got %d", len(pending))
		return
	}
	if pending[0].Version != 4 {
		t.Errorf("expected first pending version 4, got %d", pending[0].Version)
	}
	if pending[1].Version != 5 {
		t.Errorf("expected second pending version 5, got %d", pending[1].Version)
	}
}

func TestPendingMigrationsAllApplied(t *testing.T) {
	migrations := []Migration{
		{Version: 1},
		{Version: 2},
	}

	applied := map[int]Migration{
		1: {Version: 1},
		2: {Version: 2},
	}

	pending := pendingMigrations(migrations, applied)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending migrations, got %d", len(pending))
	}
}

func TestPendingMigrationsEmptyApplied(t *testing.T) {
	migrations := []Migration{
		{Version: 1},
		{Version: 2},
		{Version: 3},
	}

	applied := map[int]Migration{}

	pending := pendingMigrations(migrations, applied)
	if len(pending) != len(migrations) {
		t.Errorf("expected %d pending migrations, got %d", len(migrations), len(pending))
	}
}
