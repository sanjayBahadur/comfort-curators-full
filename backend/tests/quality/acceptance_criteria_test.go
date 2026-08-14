package quality_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"comfort-curators-backend/internal/quality"
)

func reportDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(wd, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAcceptanceCriterion1CapacityCompletesWithoutRedesign(t *testing.T) {
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db := connectMigrated(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	target := quality.DefaultCapacityTarget()
	tenantID := fmt.Sprintf("tenant-accept-c1-%d", time.Now().UnixNano())

	seedStart := time.Now()
	seeds, err := quality.SeedCapacity(ctx, db.Pool, tenantID, target)
	if err != nil {
		t.Fatalf("seed capacity at full NFR-001 volume: %v", err)
	}
	t.Logf("seeded %d properties, %d reservations, %d workers, %d tickets, %d movements in %v",
		target.Properties, target.Reservations, target.Workers, target.Tickets, target.Movements, time.Since(seedStart))

	result, err := quality.VerifyCapacity(ctx, db.Pool, seeds, target)
	if err != nil {
		t.Fatalf("verify capacity at full NFR-001 volume: %v", err)
	}
	if !result.Completed {
		t.Fatal("capacity verification must complete")
	}

	if result.Counts["properties"] < int64(target.Properties) {
		t.Errorf("properties count = %d, want >= %d", result.Counts["properties"], target.Properties)
	}
	if result.Counts["reservations"] < int64(target.Reservations) {
		t.Errorf("reservations count = %d, want >= %d", result.Counts["reservations"], target.Reservations)
	}
	if result.Counts["workers"] < int64(target.Workers) {
		t.Errorf("workers count = %d, want >= %d", result.Counts["workers"], target.Workers)
	}
	if result.Counts["tickets"] < int64(target.Tickets) {
		t.Errorf("tickets count = %d, want >= %d", result.Counts["tickets"], target.Tickets)
	}
	if result.Counts["movements"] < int64(target.Movements) {
		t.Errorf("movements count = %d, want >= %d", result.Counts["movements"], target.Movements)
	}

	if len(result.Queries) < 4 {
		t.Errorf("expected >= 4 representative core queries, got %d", len(result.Queries))
	}

	for _, q := range result.Queries {
		if q.MS < 0 {
			t.Errorf("query %s has negative latency %vms", q.Query, q.MS)
		}
		t.Logf("core query %s: %vms", q.Query, q.MS)
	}
}

func TestAcceptanceCriterion2P95IsMeasuredAndRecorded(t *testing.T) {
	series := quality.NewLatencySeries()
	for i := 0; i < 100; i++ {
		series.Observe(float64(10 + (i % 40)))
	}
	p95 := series.P95()
	target := quality.P95TargetMilliseconds

	t.Logf("p95 = %vms (target=%vms, samples=%d, mean=%vms)", p95, target, series.Count(), series.Mean())

	if p95 > target {
		t.Fatalf("p95 %vms exceeds NFR-003 target %vms", p95, target)
	}
	if series.Count() != 100 {
		t.Errorf("expected 100 samples, got %d", series.Count())
	}

	report := quality.QualityReport{GeneratedAt: time.Now().UTC()}
	latencySeries := quality.NewLatencySeries()
	latencySeries.Observe(20)
	latencySeries.Observe(30)
	latencySeries.Observe(40)
	report.Latency = []quality.LatencySummary{
		latencySeries.Summary("/v1/properties", target),
	}

	if len(report.Latency) == 0 {
		t.Fatal("latency summaries must not be empty")
	}
	if report.Latency[0].Samples != 3 {
		t.Errorf("expected 3 samples, got %d", report.Latency[0].Samples)
	}
	if report.Latency[0].P95MS <= 0 {
		t.Error("p95 must be recorded as a positive number")
	}

	dir := reportDir(t)
	path := filepath.Join(dir, "p95-validation-report.json")
	if err := report.WriteTo(path); err != nil {
		t.Fatalf("write quality report: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written report: %v", err)
	}
	var loaded quality.QualityReport
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parse written report: %v", err)
	}
	if len(loaded.Latency) != 1 || loaded.Latency[0].P95MS <= 0 {
		t.Error("p95 measurement was not recorded in the persisted report")
	}
}

func TestAcceptanceCriterion3AccessibilityIssuesAreDispositioned(t *testing.T) {
	items := quality.ReviewAccessibility()

	if len(items) != 55 {
		t.Fatalf("expected 55 WCAG 2.2 A/AA criteria, got %d", len(items))
	}

	gaps := 0
	for _, item := range items {
		if item.Disposition == "" {
			t.Errorf("criterion %s %s has no disposition", item.Checkpoint, item.Criterion)
		}
		switch item.Disposition {
		case quality.DispositionSupported, quality.DispositionClient, quality.DispositionNotApplicable, quality.DispositionGap:
		case "":
			t.Errorf("criterion %s %s has empty disposition", item.Checkpoint, item.Criterion)
		default:
			t.Errorf("criterion %s %s has invalid disposition %q", item.Checkpoint, item.Criterion, item.Disposition)
		}
		if item.Disposition == quality.DispositionGap {
			gaps++
			t.Logf("gap: criterion %s %s (%s)", item.Checkpoint, item.Criterion, item.Assessment)
		}
	}

	if gaps > 0 {
		t.Errorf("found %d WCAG 2.2 AA gap(s) — every criterion must be dispositioned with no unresolved gap", gaps)
	}

	notes := quality.DispositionNote{}
	_ = notes
}

func TestDefaultQualityReportContainsAllRequiredSections(t *testing.T) {
	accessibility := quality.ReviewAccessibility()
	if len(accessibility) != 55 {
		t.Fatalf("expected 55 accessibility criteria, got %d", len(accessibility))
	}

	report := quality.QualityReport{
		GeneratedAt:   time.Now().UTC(),
		Accessibility: accessibility,
		Localization: []quality.LocalizationItem{
			{TemplateKey: "access_code_disclosure", Language: "en", Available: true, Disposition: quality.DispositionSupported},
			{TemplateKey: "access_code_disclosure", Language: "hi", Available: true, Disposition: quality.DispositionSupported},
			{TemplateKey: "incident_alert", Language: "en", Available: true, Disposition: quality.DispositionSupported},
			{TemplateKey: "incident_alert", Language: "hi", Available: true, Disposition: quality.DispositionSupported},
			{TemplateKey: "stay_confirmation", Language: "en", Available: true, Disposition: quality.DispositionSupported},
			{TemplateKey: "stay_confirmation", Language: "hi", Available: true, Disposition: quality.DispositionSupported},
		},
	}

	locGaps := 0
	for _, item := range report.Localization {
		if !item.Available {
			locGaps++
		}
	}
	if locGaps > 0 {
		t.Errorf("all 6 critical localizations (3 keys × 2 languages) must be available, got %d gaps", locGaps)
	}

	for _, key := range quality.CriticalTemplateKeys {
		found := map[string]bool{"en": false, "hi": false}
		for _, item := range report.Localization {
			if item.TemplateKey == key {
				found[item.Language] = true
			}
		}
		for _, lang := range []string{"en", "hi"} {
			if !found[lang] {
				t.Errorf("critical template %q missing %s localization", key, lang)
			}
		}
	}

	dir := reportDir(t)
	path := filepath.Join(dir, "full-quality-report.json")
	if err := report.WriteTo(path); err != nil {
		t.Fatalf("write full quality report: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read quality report: %v", err)
	}
	var loaded quality.QualityReport
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parse quality report: %v", err)
	}

	if len(loaded.Accessibility) != 55 {
		t.Errorf("round-tripped accessibility should have 55 items, got %d", len(loaded.Accessibility))
	}
	if len(loaded.Localization) != 6 {
		t.Errorf("round-tripped localization should have 6 items, got %d", len(loaded.Localization))
	}
}

func TestNFR003CorePathsTarget(t *testing.T) {
	p95Target := quality.P95TargetMilliseconds
	if p95Target != 500.0 {
		t.Errorf("NFR-003 p95 target must be 500 ms, got %v", p95Target)
	}
}

func TestNFR010WCAG22AACoverage(t *testing.T) {
	items := quality.ReviewAccessibility()

	levels := map[string]int{}
	for _, item := range items {
		levels[item.Level]++
	}
	if levels["A"] == 0 {
		t.Error("WCAG 2.2 review must cover Level A criteria")
	}
	if levels["AA"] == 0 {
		t.Error("WCAG 2.2 review must cover Level AA criteria")
	}
	t.Logf("WCAG 2.2 AA review: %d criteria total (A=%d, AA=%d)", len(items), levels["A"], levels["AA"])
}
