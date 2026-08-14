package recovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func TestStatusReportDefaults(t *testing.T) {
	report := recoveryReport(nil)

	if report.RPO != "15 minutes" {
		t.Errorf("expected RPO '15 minutes', got %q", report.RPO)
	}
	if report.RTO != "4 hours" {
		t.Errorf("expected RTO '4 hours', got %q", report.RTO)
	}
	if !report.BackupEnabled {
		t.Error("expected BackupEnabled to be true")
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a, b"},
		{[]string{"a", "b", "c"}, "a, b, c"},
	}

	for _, tt := range tests {
		got := joinStrings(tt.input)
		if got != tt.want {
			t.Errorf("joinStrings(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHandlerRecoveryStatus(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/health/recovery", nil)
	rec := httptest.NewRecorder()

	h.RecoveryStatus()(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var report StatusReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if report.RPO != "15 minutes" {
		t.Errorf("expected RPO '15 minutes', got %q", report.RPO)
	}
	if report.RTO != "4 hours" {
		t.Errorf("expected RTO '4 hours', got %q", report.RTO)
	}
	if !report.BackupEnabled {
		t.Error("expected BackupEnabled to be true")
	}
	if report.LastBackupAt != "" {
		t.Errorf("expected empty LastBackupAt, got %q", report.LastBackupAt)
	}
}

func TestHandlerRecordBackup(t *testing.T) {
	h := NewHandler(nil)

	if !h.LastBackup().IsZero() {
		t.Error("expected zero time before recording")
	}

	h.RecordBackup(testTime)

	if h.LastBackup().IsZero() {
		t.Error("expected non-zero time after recording")
	}

	req := httptest.NewRequest(http.MethodGet, "/health/recovery", nil)
	rec := httptest.NewRecorder()
	h.RecoveryStatus()(rec, req)

	var report StatusReport
	json.Unmarshal(rec.Body.Bytes(), &report)

	if report.LastBackupAt == "" {
		t.Error("expected LastBackupAt to be set in report after recording backup")
	}
}

func TestDependencyCheckers(t *testing.T) {
	_ = DependencyChecker()
	_ = MinioChecker()
	_ = ModelChecker()

	checkers := []struct {
		c    interface{ Name() string }
		name string
	}{
		{DependencyChecker(), "dependencies"},
		{MinioChecker(), "minio"},
		{ModelChecker(), "model"},
	}

	for _, tc := range checkers {
		if tc.c.Name() != tc.name {
			t.Errorf("expected checker name %q, got %q", tc.name, tc.c.Name())
		}
	}
}

func TestMinioAvailable(t *testing.T) {
	result := minioAvailable()
	_ = result
}

func TestModelStubAvailable(t *testing.T) {
	result := modelStubAvailable()
	_ = result
}

func TestCheckMigrationHealthNoPool(t *testing.T) {
	ok := checkMigrationHealth(context.Background(), nil)
	if ok {
		t.Error("expected migration health check to fail when pool is nil")
	}
}

func TestCheckOutboxSafetyNoPool(t *testing.T) {
	ok := checkOutboxSafety(context.Background(), nil)
	if ok {
		t.Error("expected outbox safety check to fail when pool is nil")
	}
}
