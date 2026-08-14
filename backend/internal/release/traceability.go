package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const SchemaRequirementEvidence = "comfort-curators-requirement-evidence/v1"
const SchemaLaunchEvidence = "comfort-curators-launch-evidence/v1"
const SchemaSecurityFindings = "comfort-curators-security-findings/v1"

type NormativeRequirement struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	OwnerTask   string   `json:"owner_task"`
	Phase       int      `json:"phase"`
	Tests       []string `json:"tests"`
	Commands    []string `json:"commands"`
}

type NamedBehaviorEvidence struct {
	BehaviorID   string   `json:"behavior_id"`
	Phase        int      `json:"phase"`
	TestFuncName string   `json:"test_func"`
	Observations []string `json:"observations"`
	GateGroup    string   `json:"gate_group"`
	OwnerTask    string   `json:"owner_task"`
}

type LaunchAreaEvidence struct {
	Area        int      `json:"area"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tests       []string `json:"tests"`
	Commands    []string `json:"commands"`
}

type PilotMetricsVerification struct {
	P95ReadinessRate         string   `json:"p95_readiness_rate"`
	FirstPassQualityRate     string   `json:"first_pass_quality_rate"`
	ContributionPerProperty  string   `json:"contribution_per_property"`
	P95LatencyMilliseconds   float64  `json:"p95_latency_milliseconds"`
	CapacityTargetProperties int      `json:"capacity_target_properties"`
	EvidenceCommands         []string `json:"evidence_commands"`
}

type DeviceWorkflowVerification struct {
	OfflineChecklistSync string   `json:"offline_checklist_sync"`
	IdempotentReplay     string   `json:"idempotent_replay"`
	ConflictPreservation string   `json:"conflict_preservation"`
	EvidenceCommands     []string `json:"evidence_commands"`
}

type RecoveryRehearsalVerification struct {
	BackupRestore         string   `json:"backup_restore"`
	MigrationRecovery     string   `json:"migration_recovery"`
	OutboxReplay          string   `json:"outbox_replay"`
	DependencyDegradation string   `json:"dependency_degradation"`
	RPO                   string   `json:"rpo"`
	RTO                   string   `json:"rto"`
	EvidenceCommands      []string `json:"evidence_commands"`
}

type UnresolvedLimitation struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Mitigation  string `json:"mitigation"`
}

type SecurityFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type ReleasePackage struct {
	SchemaVersion            string                         `json:"schema"`
	GeneratedAt              time.Time                      `json:"generated_at"`
	CanonicalCommit          string                         `json:"canonical_commit"`
	NormativeRequirements    []NormativeRequirement         `json:"requirements,omitempty"`
	NamedBehaviors           []NamedBehaviorEvidence        `json:"named_behaviors,omitempty"`
	LaunchAreas              []LaunchAreaEvidence           `json:"launch_areas,omitempty"`
	PilotMetrics             *PilotMetricsVerification      `json:"pilot_metrics,omitempty"`
	DeviceWorkflow           *DeviceWorkflowVerification    `json:"device_workflow,omitempty"`
	RecoveryRehearsal        *RecoveryRehearsalVerification `json:"recovery_rehearsal,omitempty"`
	UnresolvedLimitations    []UnresolvedLimitation         `json:"unresolved_limitations,omitempty"`
	SecurityFindings         []SecurityFinding              `json:"security_findings"`
	ProductionAutoApproved   bool                           `json:"production_auto_approved"`
	ManualInspectionRequired bool                           `json:"manual_inspection_required"`
}

func BuildReleasePackage(commit string) *ReleasePackage {
	return &ReleasePackage{
		SchemaVersion:            "comfort-curators-release-package/v1",
		GeneratedAt:              time.Now().UTC(),
		CanonicalCommit:          commit,
		NormativeRequirements:    Requirements(),
		NamedBehaviors:           NamedBehaviors(),
		LaunchAreas:              LaunchAreas(),
		PilotMetrics:             PilotMetrics(),
		DeviceWorkflow:           DeviceWorkflow(),
		RecoveryRehearsal:        RecoveryRehearsal(),
		UnresolvedLimitations:    Limitations(),
		SecurityFindings:         Findings(),
		ProductionAutoApproved:   false,
		ManualInspectionRequired: true,
	}
}

func (p *ReleasePackage) WriteTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create release directory: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal release package: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write release package: %w", err)
	}
	return nil
}

type RequirementEvidenceDoc struct {
	Schema       string                 `json:"schema"`
	Requirements []NormativeRequirement `json:"requirements"`
}

func (p *ReleasePackage) WriteRequirementEvidence(path string) error {
	doc := RequirementEvidenceDoc{
		Schema:       SchemaRequirementEvidence,
		Requirements: p.NormativeRequirements,
	}
	return writeJSON(path, doc)
}

type LaunchEvidenceDoc struct {
	Schema string               `json:"schema"`
	Areas  []LaunchAreaEvidence `json:"areas"`
}

func (p *ReleasePackage) WriteLaunchEvidence(path string) error {
	doc := LaunchEvidenceDoc{
		Schema: SchemaLaunchEvidence,
		Areas:  p.LaunchAreas,
	}
	return writeJSON(path, doc)
}

type SecurityFindingsDoc struct {
	Schema       string            `json:"schema"`
	ScanRevision string            `json:"scan_revision"`
	Findings     []SecurityFinding `json:"findings"`
}

func (p *ReleasePackage) WriteSecurityFindings(path string) error {
	doc := SecurityFindingsDoc{
		Schema:       SchemaSecurityFindings,
		ScanRevision: p.CanonicalCommit,
		Findings:     p.SecurityFindings,
	}
	return writeJSON(path, doc)
}

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
