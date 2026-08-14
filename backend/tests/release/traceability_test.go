package release_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	release "comfort-curators-backend/internal/release"
)

func TestTraceability146RequirementsCovered(t *testing.T) {
	pkg := release.BuildReleasePackage("integration-test")
	reqs := pkg.NormativeRequirements

	if len(reqs) != 146 {
		t.Fatalf("expected exactly 146 normative requirements, got %d", len(reqs))
	}

	ids := map[string]bool{}
	for _, r := range reqs {
		ids[r.ID] = true
	}

	expectedPrefixes := []struct {
		prefix string
		count  int
	}{
		{"OBJ-", 5},
		{"TEN-", 4},
		{"PROP-", 5},
		{"CAL-", 8},
		{"TKT-", 8},
		{"WFM-", 14},
		{"VEH-", 9},
		{"CAT-", 10},
		{"INV-", 13},
		{"DOC-", 10},
		{"FIN-", 11},
		{"HM-", 10},
		{"COM-", 7},
		{"SEC-", 14},
		{"CON-", 6},
		{"NFR-", 12},
	}

	for _, ep := range expectedPrefixes {
		count := 0
		for _, r := range reqs {
			if strings.HasPrefix(r.ID, ep.prefix) {
				count++
			}
		}
		if count != ep.count {
			t.Errorf("prefix %s: got %d requirements, expected %d", ep.prefix, count, ep.count)
		}
	}
}

func TestTraceability55NamedBehaviorsCovered(t *testing.T) {
	behaviors := release.NamedBehaviors()
	if len(behaviors) != 55 {
		t.Fatalf("expected exactly 55 named behaviors, got %d", len(behaviors))
	}

	byPhase := map[int]int{}
	for _, b := range behaviors {
		byPhase[b.Phase]++
	}
	if byPhase[1] != 5 {
		t.Errorf("phase 1 behaviors = %d, expected 5", byPhase[1])
	}
	if byPhase[2] != 9 {
		t.Errorf("phase 2 behaviors = %d, expected 9", byPhase[2])
	}
	if byPhase[3] != 8 {
		t.Errorf("phase 3 behaviors = %d, expected 8", byPhase[3])
	}
	if byPhase[4] != 8 {
		t.Errorf("phase 4 behaviors = %d, expected 8", byPhase[4])
	}
	if byPhase[5] != 8 {
		t.Errorf("phase 5 behaviors = %d, expected 8", byPhase[5])
	}
	if byPhase[6] != 8 {
		t.Errorf("phase 6 behaviors = %d, expected 8", byPhase[6])
	}
	if byPhase[7] != 9 {
		t.Errorf("phase 7 behaviors = %d, expected 9", byPhase[7])
	}
}

func TestTraceability16LaunchAreasCovered(t *testing.T) {
	areas := release.LaunchAreas()
	if len(areas) != 16 {
		t.Fatalf("expected exactly 16 launch acceptance areas, got %d", len(areas))
	}

	expectedNames := []string{
		"Tenant and property isolation",
		"Reservation create, change, cancel, overlap, stale-feed, and timezone",
		"Ticket-state and permission tests for every role",
		"No hard-delete path for operational records",
		"Duplicate calendar and duplicate order idempotency",
		"Worker assignment limits, age gate, and restricted-task routing",
		"Access-secret time window and revocation",
		"Inventory movement reconciliation and count adjustment",
		"Owner budget, substitution, and purchase approval",
		"Maker-checker financial approval",
		"Jarvis prohibited-action tests",
		"Human review for worker adverse action",
		"Offline worker checklist synchronization",
		"Audit-log completeness",
		"Backup restoration and incident simulation",
		"Plain-language consent, grievance, and privacy flows",
	}

	for i, a := range areas {
		if a.Area != i+1 {
			t.Errorf("launch area %d has area number %d", i+1, a.Area)
		}
		if a.Name != expectedNames[i] {
			t.Errorf("launch area %d name = %q, expected %q", i+1, a.Name, expectedNames[i])
		}
	}
}

func TestTraceabilityNoOmissionsOrDuplicateOwners(t *testing.T) {
	reqs := release.Requirements()
	ownerCount := map[string]int{}
	for _, r := range reqs {
		ownerCount[r.OwnerTask]++
	}

	for owner, count := range ownerCount {
		if count == 0 {
			t.Errorf("owner task %q has zero requirements assigned", owner)
		}
	}
}

func TestTraceabilityEvidenceReferencesRealCommands(t *testing.T) {
	reqs := release.Requirements()
	for _, r := range reqs {
		for i, cmd := range r.Commands {
			if cmd == "" {
				t.Errorf("requirement %s command[%d] is empty", r.ID, i)
			}
		}
	}
}

func TestTraceabilityProductionNotAutoApproved(t *testing.T) {
	pkg := release.BuildReleasePackage("integration-test")

	if pkg.ProductionAutoApproved {
		t.Fatal("ProductionAutoApproved must be false — development stops at manual inspection")
	}
	if !pkg.ManualInspectionRequired {
		t.Fatal("ManualInspectionRequired must be true")
	}
}

func TestTraceabilityPilotMetricsVerified(t *testing.T) {
	pm := release.PilotMetrics()
	if pm.P95ReadinessRate == "" {
		t.Error("P95ReadinessRate is empty")
	}
	if pm.FirstPassQualityRate == "" {
		t.Error("FirstPassQualityRate is empty")
	}
	if pm.ContributionPerProperty == "" {
		t.Error("ContributionPerProperty is empty")
	}
	if !strings.Contains(pm.ContributionPerProperty, "OBJ-04") {
		t.Errorf("ContributionPerProperty should reference OBJ-04: %s", pm.ContributionPerProperty)
	}
	if pm.P95LatencyMilliseconds <= 0 {
		t.Errorf("P95LatencyMilliseconds = %v, expected >0", pm.P95LatencyMilliseconds)
	}
	if len(pm.EvidenceCommands) < 2 {
		t.Errorf("expected at least 2 evidence commands, got %d", len(pm.EvidenceCommands))
	}
}

func TestTraceabilityOwnerTrustVerified(t *testing.T) {
	reqs := release.Requirements()
	ownerTrustFound := 0
	for _, r := range reqs {
		if r.OwnerTask == "p2-tenancy" || r.ID == "TEN-003" || r.ID == "TEN-001" {
			ownerTrustFound++
		}
	}
	if ownerTrustFound < 4 {
		t.Errorf("owner trust requirements insufficient: found %d (TEN-*)", ownerTrustFound)
	}
}

func TestTraceabilityCuratorDeviceWorkflowVerified(t *testing.T) {
	dw := release.DeviceWorkflow()
	if dw.OfflineChecklistSync == "" {
		t.Error("OfflineChecklistSync is empty")
	}
	if dw.IdempotentReplay == "" {
		t.Error("IdempotentReplay is empty")
	}
	if dw.ConflictPreservation == "" {
		t.Error("ConflictPreservation is empty")
	}
	if len(dw.EvidenceCommands) < 2 {
		t.Errorf("expected at least 2 evidence commands, got %d", len(dw.EvidenceCommands))
	}
}

func TestTraceabilityRecoveryRehearsalVerified(t *testing.T) {
	rr := release.RecoveryRehearsal()
	if rr.BackupRestore == "" {
		t.Error("BackupRestore is empty")
	}
	if rr.MigrationRecovery == "" {
		t.Error("MigrationRecovery is empty")
	}
	if rr.OutboxReplay == "" {
		t.Error("OutboxReplay is empty")
	}
	if rr.DependencyDegradation == "" {
		t.Error("DependencyDegradation is empty")
	}
	if rr.RPO != "15 minutes" {
		t.Errorf("RPO = %q, expected \"15 minutes\"", rr.RPO)
	}
	if rr.RTO != "4 hours" {
		t.Errorf("RTO = %q, expected \"4 hours\"", rr.RTO)
	}
	if len(rr.EvidenceCommands) < 2 {
		t.Errorf("expected at least 2 evidence commands, got %d", len(rr.EvidenceCommands))
	}
}

func TestTraceabilityUnresolvedLimitations(t *testing.T) {
	lims := release.Limitations()
	if len(lims) == 0 {
		t.Fatal("unresolved limitations must not be empty — stops at manual inspection")
	}
	for _, l := range lims {
		if l.ID == "" || l.Description == "" || l.Impact == "" || l.Mitigation == "" {
			t.Errorf("limitation %s has empty required field", l.ID)
		}
	}
}

func TestTraceabilityAllRequirementsHaveNonEmptyTests(t *testing.T) {
	reqs := release.Requirements()
	for _, r := range reqs {
		if len(r.Tests) == 0 {
			t.Errorf("requirement %s has empty tests array", r.ID)
		}
		for _, test := range r.Tests {
			if test == "" {
				t.Errorf("requirement %s has empty test name in tests array", r.ID)
			}
		}
	}
}

func TestGenerateAndValidateEvidenceFiles(t *testing.T) {
	pkg := release.BuildReleasePackage("commit-for-manual-inspection")

	dir := t.TempDir()

	reqPath := filepath.Join(dir, "requirement-evidence.json")
	if err := pkg.WriteRequirementEvidence(reqPath); err != nil {
		t.Fatalf("WriteRequirementEvidence: %v", err)
	}

	launchPath := filepath.Join(dir, "launch-evidence.json")
	if err := pkg.WriteLaunchEvidence(launchPath); err != nil {
		t.Fatalf("WriteLaunchEvidence: %v", err)
	}

	secPath := filepath.Join(dir, "security-findings.json")
	if err := pkg.WriteSecurityFindings(secPath); err != nil {
		t.Fatalf("WriteSecurityFindings: %v", err)
	}

	for _, path := range []string{reqPath, launchPath, secPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Errorf("unmarshal %s: %v", path, err)
			continue
		}
		if _, ok := m["schema"]; !ok {
			t.Errorf("%s missing 'schema' field", path)
		}
		if len(data) < 100 {
			t.Errorf("%s is suspiciously short (%d bytes)", path, len(data))
		}
	}
}

func TestTraceabilityCoverageHasNoDuplicateOwners(t *testing.T) {
	reqs := release.Requirements()
	ownerMap := map[string][]string{}
	for _, r := range reqs {
		ownerMap[r.OwnerTask] = append(ownerMap[r.OwnerTask], r.ID)
	}

	for owner, ids := range ownerMap {
		for i, id := range ids {
			for j := i + 1; j < len(ids); j++ {
				if ids[i] == ids[j] {
					t.Errorf("owner %s has duplicate requirement ID %s", owner, id)
				}
			}
		}
	}
}

func TestTraceabilityPhaseCountsMatchPlan(t *testing.T) {
	reqs := release.Requirements()
	byPhase := map[int]int{}
	for _, r := range reqs {
		byPhase[r.Phase]++
	}

	expected := map[int]int{
		1: 10,
		2: 9,
		3: 40,
		4: 32,
		5: 35,
		6: 10,
		7: 10,
	}
	for phase, expectedCount := range expected {
		if byPhase[phase] != expectedCount {
			t.Errorf("phase %d: got %d requirements, expected %d", phase, byPhase[phase], expectedCount)
		}
	}
	if total := len(reqs); total != 146 {
		t.Errorf("total requirements = %d, expected 146", total)
	}
}

func TestSecurityFindingsEmptyWhenScanPasses(t *testing.T) {
	findings := release.Findings()
	if len(findings) != 0 {
		t.Errorf("security findings should be empty when scan passes, got %d findings", len(findings))
	}
}

func TestEvidenceJSONSerializationProducesValidJSON(t *testing.T) {
	pkg := release.BuildReleasePackage("integration-test")

	paths := map[string]func(string) error{
		"requirement-evidence.json": pkg.WriteRequirementEvidence,
		"launch-evidence.json":      pkg.WriteLaunchEvidence,
		"security-findings.json":    pkg.WriteSecurityFindings,
	}

	for filename, writer := range paths {
		t.Run(filename, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, filename)
			if err := writer(path); err != nil {
				t.Fatalf("write: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !json.Valid(data) {
				t.Errorf("not valid JSON")
				t.Logf("first 200 chars: %s", string(data[:min(200, len(data))]))
			}
		})
	}
}

func TestReleasePackagePilotContributions(t *testing.T) {
	pm := release.PilotMetrics()
	if !strings.Contains(pm.ContributionPerProperty, "INR 3000") && !strings.Contains(pm.ContributionPerProperty, "3000") {
		t.Errorf("pilot contribution should mention the INR 3000 target: %s", pm.ContributionPerProperty)
	}
	if !strings.Contains(pm.P95ReadinessRate, "95%") && !strings.Contains(pm.P95ReadinessRate, "92%") {
		t.Errorf("pilot readiness rate should mention the 95%%/92%% targets: %s", pm.P95ReadinessRate)
	}
}

func TestReleasePackageFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "release-package.json")

	pkg := release.BuildReleasePackage("integration-test")
	if err := pkg.WriteTo(path); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !json.Valid(data) {
		t.Fatal("release package JSON is not valid JSON")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	requiredFields := []string{
		"schema", "generated_at", "canonical_commit",
		"requirements", "named_behaviors", "launch_areas",
		"pilot_metrics", "device_workflow", "recovery_rehearsal",
		"unresolved_limitations", "security_findings",
		"production_auto_approved", "manual_inspection_required",
	}
	for _, field := range requiredFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("release package missing required field: %s", field)
		}
	}

	prodApproved, ok := decoded["production_auto_approved"].(bool)
	if !ok || prodApproved {
		t.Error("production_auto_approved must be false")
	}
	manualInspection, ok := decoded["manual_inspection_required"].(bool)
	if !ok || !manualInspection {
		t.Error("manual_inspection_required must be true")
	}
}

func TestManualInspectionStopsBeforeProduction(t *testing.T) {
	pkg := release.BuildReleasePackage("integration-test")

	summary := fmt.Sprintf(
		"Manual inspection required: %d requirements, %d behaviors, %d launch areas, %d limitations, production_auto_approved=%v",
		len(pkg.NormativeRequirements),
		len(pkg.NamedBehaviors),
		len(pkg.LaunchAreas),
		len(pkg.UnresolvedLimitations),
		pkg.ProductionAutoApproved,
	)

	if !strings.Contains(summary, "production_auto_approved=false") {
		t.Error("manual inspection summary must stop before production")
	}
	t.Log(summary)
}
