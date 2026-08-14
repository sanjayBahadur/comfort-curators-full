package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// QualityReport is the recorded outcome of the capacity, latency, accessibility
// and localization verification. It is persisted so the p95 result and the
// accessibility dispositions are recorded, not merely computed.
type QualityReport struct {
	GeneratedAt   time.Time           `json:"generated_at"`
	Capacity      CapacityResult      `json:"capacity"`
	Latency       []LatencySummary    `json:"latency"`
	Accessibility []AccessibilityItem `json:"accessibility"`
	Localization  []LocalizationItem  `json:"localization"`
	Dispositions  []DispositionNote   `json:"dispositions"`
}

// DispositionNote is a top-level accessibility or localization issue that was
// dispositioned as part of the review.
type DispositionNote struct {
	ID          string      `json:"id"`
	Issue       string      `json:"issue"`
	Disposition Disposition `json:"disposition"`
	Owner       string      `json:"owner"`
}

// RunCapacityScenario seeds the NFR-001 volume, verifies it runs on the existing
// schema without architectural change, and measures the p95 latency of the core
// server paths. It returns the assembled report with capacity and latency
// results filled in.
func RunCapacityScenario(ctx context.Context, pool *pgxpool.Pool, baseURL, tenantID string, target CapacityTarget) (*QualityReport, error) {
	report := &QualityReport{GeneratedAt: time.Now().UTC()}

	seedStart := time.Now()
	seeds, err := SeedCapacity(ctx, pool, tenantID, target)
	if err != nil {
		return nil, err
	}
	seedSeconds := time.Since(seedStart).Seconds()

	capacity, err := VerifyCapacity(ctx, pool, seeds, target)
	if err != nil {
		return nil, err
	}
	capacity.SeedSeconds = round2(seedSeconds)
	report.Capacity = capacity

	latency, err := MeasureCorePaths(ctx, baseURL, tenantID, seeds.FirstPropertyID, 8)
	if err != nil {
		return nil, err
	}
	report.Latency = latency

	return report, nil
}

// RunAccessibilityReview fills the accessibility and localization sections of
// the report and assembles the disposition notes.
func RunAccessibilityReview(ctx context.Context, baseURL, tenantID string, report *QualityReport) error {
	report.Accessibility = ReviewAccessibility()

	loc, err := VerifyLocalization(ctx, baseURL, tenantID, CriticalTemplateKeys)
	if err != nil {
		return err
	}
	report.Localization = loc

	report.Dispositions = dispositionNotes(report.Accessibility, report.Localization)
	return nil
}

// dispositionNotes reduces the full review to the items that carry an explicit
// disposition: every accessibility gap, every client-side accessibility item
// and every localization gap. Supported items are not repeated here.
func dispositionNotes(accessibility []AccessibilityItem, localization []LocalizationItem) []DispositionNote {
	notes := []DispositionNote{}
	for _, item := range accessibility {
		switch item.Disposition {
		case DispositionGap:
			notes = append(notes, DispositionNote{
				ID:          item.ID,
				Issue:       fmt.Sprintf("%s %s is an open accessibility gap", item.Checkpoint, item.Criterion),
				Disposition: item.Disposition,
				Owner:       "api",
			})
		case DispositionClient:
			notes = append(notes, DispositionNote{
				ID:          item.ID,
				Issue:       fmt.Sprintf("%s %s is a rendering/interaction responsibility of the consuming web client", item.Checkpoint, item.Criterion),
				Disposition: item.Disposition,
				Owner:       "client",
			})
		}
	}
	for _, item := range localization {
		if !item.Available {
			notes = append(notes, DispositionNote{
				ID:          "loc-" + item.TemplateKey + "-" + item.Language,
				Issue:       fmt.Sprintf("critical template %s is not available in %s", item.TemplateKey, item.Language),
				Disposition: item.Disposition,
				Owner:       "api",
			})
		}
	}
	return notes
}

// WriteTo persists the report as JSON at path, creating parent directories.
func (r *QualityReport) WriteTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal quality report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write quality report: %w", err)
	}
	return nil
}
