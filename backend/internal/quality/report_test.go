package quality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQualityReportWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quality-report.json")

	report := &QualityReport{
		GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Capacity: CapacityResult{
			TenantID:  "tenant-capacity-test",
			Target:    DefaultCapacityTarget(),
			Counts:    map[string]int64{"properties": 50, "movements": 100000},
			Completed: true,
		},
		Latency: []LatencySummary{
			{Path: "/v1/properties", Samples: 8, P95MS: 42, TargetMS: 500, WithinTarget: true},
		},
		Accessibility: ReviewAccessibility(),
		Localization: []LocalizationItem{
			{TemplateKey: "stay_confirmation", Language: "hi", Available: true, Disposition: DispositionSupported},
		},
	}

	if err := report.WriteTo(path); err != nil {
		t.Fatalf("write report: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}

	var loaded QualityReport
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parse report: %v", err)
	}

	if loaded.GeneratedAt != report.GeneratedAt {
		t.Errorf("generated_at mismatch")
	}
	if loaded.Capacity.TenantID != report.Capacity.TenantID {
		t.Errorf("capacity tenant mismatch")
	}
	if len(loaded.Latency) != 1 || loaded.Latency[0].P95MS != 42 {
		t.Errorf("latency not recorded: %+v", loaded.Latency)
	}
	if len(loaded.Accessibility) != len(ReviewAccessibility()) {
		t.Errorf("accessibility review not recorded: %d items", len(loaded.Accessibility))
	}
	if len(loaded.Localization) != 1 {
		t.Errorf("localization not recorded: %+v", loaded.Localization)
	}
}

func TestQualityReportWriteCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "quality-report.json")
	report := &QualityReport{GeneratedAt: time.Now().UTC()}
	if err := report.WriteTo(path); err != nil {
		t.Fatalf("write report with nested dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("report file not created: %v", err)
	}
}

func TestDispositionNotesNoUndecidedLocalization(t *testing.T) {
	loc := []LocalizationItem{
		{TemplateKey: "stay_confirmation", Language: "en", Available: true, Disposition: DispositionSupported},
		{TemplateKey: "stay_confirmation", Language: "hi", Available: true, Disposition: DispositionSupported},
	}
	notes := dispositionNotes(nil, loc)
	if len(notes) != 0 {
		t.Errorf("supported localization pairs must not produce disposition notes, got %+v", notes)
	}
}
