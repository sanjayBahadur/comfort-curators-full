package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormativeRequirementsCount(t *testing.T) {
	reqs := Requirements()
	if len(reqs) != 146 {
		t.Errorf("expected 146 normative requirements, got %d", len(reqs))
	}
}

func TestAllRequirementIDsAreValid(t *testing.T) {
	reqs := Requirements()
	validPrefixes := map[string]bool{
		"OBJ-": true, "TEN-": true, "PROP-": true, "CAL-": true,
		"TKT-": true, "WFM-": true, "VEH-": true, "CAT-": true,
		"INV-": true, "DOC-": true, "FIN-": true, "HM-": true,
		"COM-": true, "SEC-": true, "CON-": true, "NFR-": true,
	}
	for _, r := range reqs {
		if r.ID == "" {
			t.Errorf("requirement with empty ID at index")
			continue
		}
		valid := false
		for prefix := range validPrefixes {
			if strings.HasPrefix(r.ID, prefix) {
				valid = true
				break
			}
		}
		if !valid {
			t.Errorf("requirement %s has invalid ID prefix", r.ID)
		}
		if r.OwnerTask == "" {
			t.Errorf("requirement %s has empty owner_task", r.ID)
		}
		if len(r.Tests) == 0 {
			t.Errorf("requirement %s has empty tests list", r.ID)
		}
		if len(r.Commands) == 0 {
			t.Errorf("requirement %s has empty commands list", r.ID)
		}
	}
}

func TestNoDuplicateRequirementIDs(t *testing.T) {
	reqs := Requirements()
	seen := map[string]bool{}
	for _, r := range reqs {
		if seen[r.ID] {
			t.Errorf("duplicate requirement ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestCoverageMatrixOwnersMatch(t *testing.T) {
	reqs := Requirements()
	expectedOwners := map[string]string{
		"OBJ-":  "p7-traceability-release",
		"TEN-":  "p2-tenancy",
		"PROP-": "p2-properties",
		"CAL-":  "p3-calendar-ingestion",
		"TKT-":  "p3-ticket-state-machine",
		"WFM-":  "p3-workforce",
		"VEH-":  "p4-fleet",
		"CAT-":  "p4-catalog-packages",
		"INV-":  "p4-inventory-ledger",
		"DOC-":  "p5-documents",
		"FIN-":  "p5-charges-invoices",
		"COM-":  "p3-communications",
		"CON-":  "p5-consumer-controls",
	}
	jarvisOwners := map[string]string{
		"HM-001": "p6-jarvis-context",
		"HM-002": "p6-jarvis-context",
		"HM-003": "p6-jarvis-context",
		"HM-004": "p6-jarvis-context",
		"HM-005": "p6-typed-tools-policy",
		"HM-006": "p6-typed-tools-policy",
		"HM-007": "p6-typed-tools-policy",
		"HM-008": "p6-typed-tools-policy",
		"HM-009": "p6-typed-tools-policy",
		"HM-010": "p6-typed-tools-policy",
	}
	secOwners := map[string]string{
		"SEC-001": "p5-privacy-rights",
		"SEC-002": "p5-privacy-rights",
		"SEC-003": "p5-privacy-rights",
		"SEC-004": "p5-privacy-rights",
		"SEC-005": "p5-privacy-rights",
		"SEC-006": "p1-security-audit",
		"SEC-007": "p1-security-audit",
		"SEC-008": "p1-security-audit",
		"SEC-009": "p1-security-audit",
		"SEC-010": "p5-privacy-rights",
		"SEC-011": "p5-privacy-rights",
		"SEC-012": "p5-privacy-rights",
		"SEC-013": "p1-security-audit",
		"SEC-014": "p1-security-audit",
	}
	nfrOwners := map[string]string{
		"NFR-001": "p7-performance-accessibility",
		"NFR-002": "p7-performance-accessibility",
		"NFR-003": "p7-performance-accessibility",
		"NFR-004": "p1-jobs",
		"NFR-005": "p1-jobs",
		"NFR-006": "p1-durability",
		"NFR-007": "p1-durability",
		"NFR-008": "p3-offline-sync",
		"NFR-009": "p3-offline-sync",
		"NFR-010": "p7-performance-accessibility",
		"NFR-011": "p3-offline-sync",
		"NFR-012": "p7-backup-recovery",
	}

	for _, r := range reqs {
		var expected string
		var ok bool
		switch {
		case strings.HasPrefix(r.ID, "HM-"):
			expected, ok = jarvisOwners[r.ID]
		case strings.HasPrefix(r.ID, "SEC-"):
			expected, ok = secOwners[r.ID]
		case strings.HasPrefix(r.ID, "NFR-"):
			expected, ok = nfrOwners[r.ID]
		default:
			for prefix, owner := range expectedOwners {
				if strings.HasPrefix(r.ID, prefix) {
					expected = owner
					ok = true
					break
				}
			}
		}
		if !ok {
			t.Errorf("requirement %s has no expected owner in coverage matrix", r.ID)
			continue
		}
		if r.OwnerTask != expected {
			t.Errorf("requirement %s: owner_task=%q, expected=%q", r.ID, r.OwnerTask, expected)
		}
	}
}

func TestNamedBehaviorsCount(t *testing.T) {
	behaviors := NamedBehaviors()
	if len(behaviors) != 55 {
		t.Errorf("expected 55 named behaviors, got %d", len(behaviors))
	}
}

func TestAllNamedBehaviorTestFuncsAreInOracle(t *testing.T) {
	behaviors := NamedBehaviors()
	expectedNames := []string{
		"TestCCFND001EmptyDatabaseMigration",
		"TestCCFND001OutboxCommitAtomicity",
		"TestCCFND001IdempotencyReplay",
		"TestCCFND001PrivateObjectSignedAccess",
		"TestCCFND001APIWorkerStart",
		"TestCCIAM001CrossTenantDenied",
		"TestCCIAM001UnassignedPropertyDenied",
		"TestCCIAM001OwnerGuestRolesDistinct",
		"TestCCIAM001ExpiredSupportAccessDenied",
		"TestCCIAM001StaffMFARequired",
		"TestCCONB001LifecycleTransitions",
		"TestCCONB001SafetyHoldBlocksActivation",
		"TestCCONB001QuoteIsDeterministic",
		"TestCCONB001AgreementVersionIsImmutable",
		"TestCCRES001StaleCalendarIsRejected",
		"TestCCRES001ConflictIsDetected",
		"TestCCRES001CancellationUpdatesTurnover",
		"TestCCRES001UnauthorizedMessageIsBlocked",
		"TestCCOPS001DispatchHonorsHardConstraints",
		"TestCCOPS001EvidenceRequiredForCompletion",
		"TestCCOPS001OfflineReplayIsIdempotent",
		"TestCCOPS001IncidentEscalates",
		"TestCCACC001CustodyLedgerIsAppendOnly",
		"TestCCACC001DisclosureIsAuditedAndExpires",
		"TestCCACC001RevocationBlocksDisclosure",
		"TestCCACC001EmergencyAccessIsAttributable",
		"TestCCINV001MovementLedgerIsAppendOnly",
		"TestCCINV001NegativeBalanceIsRejected",
		"TestCCINV001ReconciliationIsAttributable",
		"TestCCINV001ConcurrentMovementIsConsistent",
		"TestCCBIL001MoneyUsesMinorUnitsAndCurrency",
		"TestCCBIL001OwnerBillingOnly",
		"TestCCBIL001InvoiceCreationIsIdempotent",
		"TestCCBIL001DuplicateExpenseIsDetected",
		"TestCCDOC001VersionsAreImmutable",
		"TestCCDOC001ExpiryIsDetected",
		"TestCCDOC001ExtractionRetainsSourceAndConfidence",
		"TestCCDOC001SubmissionRequiresHumanReview",
		"TestCCHOU001PropertyScopeCannotCross",
		"TestCCHOU001OnlyTypedToolsAreExposed",
		"TestCCHOU001PolicyRejectsDirectMutation",
		"TestCCHOU001ModelOutageHasManualFallback",
		"TestCCHER001CommunicationAuthorityIsNarrow",
		"TestCCHER001ApprovalPolicyIsEnforced",
		"TestCCHER001OwnerAndGuestContextsAreSeparated",
		"TestCCHER001DeliveryIsIdempotent",
		"TestCCSEC001SecretsAreRedacted",
		"TestCCSEC001SecureLinkExpiresAndRejectsReplay",
		"TestCCSEC001AuditEvidenceCannotBeRewritten",
		"TestCCSEC001CrossTenantRequestsFailClosed",
		"TestCCREL001BackupRestoreRebuildsWorkflow",
		"TestCCREL001MigrationForwardRecovery",
		"TestCCREL001OutboxReplayIsIdempotent",
		"TestCCREL001DependencyDegradationIsVisible",
		"TestCCREL001CapacityTarget",
	}

	behaviorNames := map[string]bool{}
	for _, b := range behaviors {
		behaviorNames[b.TestFuncName] = true
	}

	for _, expected := range expectedNames {
		if !behaviorNames[expected] {
			t.Errorf("named behavior test function %q is missing from NamedBehaviors()", expected)
		}
	}
}

func TestNoDuplicateBehaviorTestFuncs(t *testing.T) {
	behaviors := NamedBehaviors()
	seen := map[string]bool{}
	for _, b := range behaviors {
		if seen[b.TestFuncName] {
			t.Errorf("duplicate behavior test function: %s", b.TestFuncName)
		}
		seen[b.TestFuncName] = true
	}
}

func TestLaunchAreasCount(t *testing.T) {
	areas := LaunchAreas()
	if len(areas) != 16 {
		t.Errorf("expected 16 launch acceptance areas, got %d", len(areas))
	}
}

func TestLaunchAreasHaveValidFields(t *testing.T) {
	areas := LaunchAreas()
	seenArea := map[int]bool{}
	areaNames := map[int]string{}
	testsPerArea := map[int]int{}
	for _, a := range areas {
		if seenArea[a.Area] {
			t.Errorf("duplicate launch area number: %d", a.Area)
		}
		seenArea[a.Area] = true
		if a.Area < 1 || a.Area > 16 {
			t.Errorf("launch area number out of range [1,16]: %d", a.Area)
		}
		if a.Name == "" {
			t.Errorf("launch area %d has empty name", a.Area)
		}
		if len(a.Tests) == 0 {
			t.Errorf("launch area %d has empty tests list", a.Area)
		}
		if len(a.Commands) == 0 {
			t.Errorf("launch area %d has empty commands list", a.Area)
		}
		areaNames[a.Area] = a.Name
		testsPerArea[a.Area] = len(a.Tests)
	}

	for i := 1; i <= 16; i++ {
		if !seenArea[i] {
			t.Errorf("launch area %d is missing", i)
		}
	}
}

func TestLaunchAreasUseDistinctRealTests(t *testing.T) {
	areas := LaunchAreas()
	allTests := map[string]int{}
	for _, a := range areas {
		for _, test := range a.Tests {
			allTests[test]++
		}
	}
	distinctTests := len(allTests)
	if distinctTests < 16 {
		t.Errorf("launch areas use only %d distinct tests, expected at least 16", distinctTests)
	}
}

func TestRequirementEvidenceCommandsUseRealCommands(t *testing.T) {
	reqs := Requirements()
	for _, r := range reqs {
		for _, cmd := range r.Commands {
			if !strings.Contains(cmd, "go test") && !strings.Contains(cmd, "make ") && !strings.Contains(cmd, "tests/acceptance") {
				t.Errorf("requirement %s command does not reference a real runner: %q", r.ID, cmd)
			}
		}
	}
}

func TestPilotMetricsHasEvidenceCommands(t *testing.T) {
	m := PilotMetrics()
	if m == nil {
		t.Fatal("pilot metrics is nil")
	}
	if len(m.EvidenceCommands) == 0 {
		t.Error("pilot metrics has no evidence commands")
	}
	if m.P95LatencyMilliseconds != 500.0 {
		t.Errorf("p95 latency target = %v, expected 500.0", m.P95LatencyMilliseconds)
	}
	if m.CapacityTargetProperties != 50 {
		t.Errorf("capacity target properties = %d, expected 50", m.CapacityTargetProperties)
	}
}

func TestDeviceWorkflowHasEvidenceCommands(t *testing.T) {
	d := DeviceWorkflow()
	if d == nil {
		t.Fatal("device workflow is nil")
	}
	if len(d.EvidenceCommands) == 0 {
		t.Error("device workflow has no evidence commands")
	}
	if d.OfflineChecklistSync == "" || d.IdempotentReplay == "" || d.ConflictPreservation == "" {
		t.Error("device workflow has empty verification fields")
	}
}

func TestRecoveryRehearsalHasEvidenceCommandsAndTargets(t *testing.T) {
	r := RecoveryRehearsal()
	if r == nil {
		t.Fatal("recovery rehearsal is nil")
	}
	if len(r.EvidenceCommands) == 0 {
		t.Error("recovery rehearsal has no evidence commands")
	}
	if r.RPO != "15 minutes" {
		t.Errorf("RPO = %q, expected 15 minutes", r.RPO)
	}
	if r.RTO != "4 hours" {
		t.Errorf("RTO = %q, expected 4 hours", r.RTO)
	}
}

func TestUnresolvedLimitationsExist(t *testing.T) {
	lims := Limitations()
	if len(lims) == 0 {
		t.Error("expected non-empty unresolved limitations to stop at manual inspection")
	}
	seen := map[string]bool{}
	for _, l := range lims {
		if l.ID == "" {
			t.Error("limitation with empty ID")
		}
		if seen[l.ID] {
			t.Errorf("duplicate limitation ID: %s", l.ID)
		}
		seen[l.ID] = true
		if l.Description == "" || l.Impact == "" || l.Mitigation == "" {
			t.Errorf("limitation %s has empty fields", l.ID)
		}
	}
}

func TestProductionIsNotAutoApproved(t *testing.T) {
	pkg := BuildReleasePackage("test-sha-00e0815")
	if pkg.ProductionAutoApproved {
		t.Error("ProductionAutoApproved must be false — production is not auto-approved")
	}
	if !pkg.ManualInspectionRequired {
		t.Error("ManualInspectionRequired must be true — stop at manual inspection")
	}
}

func TestBuildReleasePackageContainsAllSections(t *testing.T) {
	pkg := BuildReleasePackage("test-sha-00e0815")
	if pkg.SchemaVersion != "comfort-curators-release-package/v1" {
		t.Errorf("unexpected schema version: %s", pkg.SchemaVersion)
	}
	if len(pkg.NormativeRequirements) != 146 {
		t.Errorf("normative requirements count = %d, expected 146", len(pkg.NormativeRequirements))
	}
	if len(pkg.NamedBehaviors) != 55 {
		t.Errorf("named behaviors count = %d, expected 55", len(pkg.NamedBehaviors))
	}
	if len(pkg.LaunchAreas) != 16 {
		t.Errorf("launch areas count = %d, expected 16", len(pkg.LaunchAreas))
	}
	if pkg.PilotMetrics == nil {
		t.Error("pilot metrics is nil")
	}
	if pkg.DeviceWorkflow == nil {
		t.Error("device workflow is nil")
	}
	if pkg.RecoveryRehearsal == nil {
		t.Error("recovery rehearsal is nil")
	}
	if len(pkg.UnresolvedLimitations) == 0 {
		t.Error("unresolved limitations are empty")
	}
}

func TestReleasePackageWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "release-package.json")

	pkg := BuildReleasePackage("test-sha-00e0815")
	if err := pkg.WriteTo(path); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var roundTripped ReleasePackage
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if roundTripped.SchemaVersion != pkg.SchemaVersion {
		t.Errorf("schema version mismatch: %s != %s", roundTripped.SchemaVersion, pkg.SchemaVersion)
	}
	if len(roundTripped.NormativeRequirements) != len(pkg.NormativeRequirements) {
		t.Errorf("requirements length mismatch: %d != %d", len(roundTripped.NormativeRequirements), len(pkg.NormativeRequirements))
	}
	if roundTripped.ProductionAutoApproved {
		t.Error("round-tripped ProductionAutoApproved is true")
	}
	if !roundTripped.ManualInspectionRequired {
		t.Error("round-tripped ManualInspectionRequired is false")
	}
}

func TestWriteRequirementEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requirement-evidence.json")

	pkg := BuildReleasePackage("test-sha-00e0815")
	if err := pkg.WriteRequirementEvidence(path); err != nil {
		t.Fatalf("WriteRequirementEvidence failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var doc RequirementEvidenceDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if doc.Schema != SchemaRequirementEvidence {
		t.Errorf("schema = %q, expected %q", doc.Schema, SchemaRequirementEvidence)
	}
	if len(doc.Requirements) != 146 {
		t.Errorf("requirements = %d, expected 146", len(doc.Requirements))
	}
}

func TestWriteLaunchEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launch-evidence.json")

	pkg := BuildReleasePackage("test-sha-00e0815")
	if err := pkg.WriteLaunchEvidence(path); err != nil {
		t.Fatalf("WriteLaunchEvidence failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var doc LaunchEvidenceDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if doc.Schema != SchemaLaunchEvidence {
		t.Errorf("schema = %q, expected %q", doc.Schema, SchemaLaunchEvidence)
	}
	if len(doc.Areas) != 16 {
		t.Errorf("areas = %d, expected 16", len(doc.Areas))
	}
}

func TestWriteSecurityFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "security-findings.json")

	pkg := BuildReleasePackage("test-sha-00e0815")
	if err := pkg.WriteSecurityFindings(path); err != nil {
		t.Fatalf("WriteSecurityFindings failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var doc SecurityFindingsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if doc.Schema != SchemaSecurityFindings {
		t.Errorf("schema = %q, expected %q", doc.Schema, SchemaSecurityFindings)
	}
	if doc.ScanRevision != "test-sha-00e0815" {
		t.Errorf("scan revision = %q, expected test-sha-00e0815", doc.ScanRevision)
	}
}

func TestAll55OracleBehaviorsAreTraced(t *testing.T) {
	behaviors := NamedBehaviors()
	reqs := Requirements()

	behaviorNames := map[string]int{}
	for _, b := range behaviors {
		behaviorNames[b.TestFuncName]++
	}

	referencedNames := map[string]bool{}
	for _, r := range reqs {
		for _, test := range r.Tests {
			referencedNames[test] = true
		}
	}

	for testName := range behaviorNames {
		if !referencedNames[testName] {
			t.Errorf("named behavior test %q is not referenced by any normative requirement", testName)
		}
	}
}

func TestGateGroupAggregation(t *testing.T) {
	behaviors := NamedBehaviors()
	expectedCounts := map[string]int{
		"CC-FND-001": 5,
		"CC-IAM-001": 5,
		"CC-ONB-001": 4,
		"CC-RES-001": 4,
		"CC-OPS-001": 4,
		"CC-ACC-001": 4,
		"CC-INV-001": 4,
		"CC-BIL-001": 4,
		"CC-DOC-001": 4,
		"CC-HOU-001": 4,
		"CC-HER-001": 4,
		"CC-SEC-001": 4,
		"CC-REL-001": 5,
	}

	actual := map[string]int{}
	for _, b := range behaviors {
		actual[b.GateGroup]++
	}

	for group, expected := range expectedCounts {
		if actual[group] != expected {
			t.Errorf("gate group %s has %d behaviors, expected %d", group, actual[group], expected)
		}
	}
}
