package onboarding

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCaseActivationHoldsMissingEvidence(t *testing.T) {
	now := time.Now().UTC()
	c := &Case{}

	holds := c.ActivationHolds(now)
	if len(holds) != 2 {
		t.Fatalf("missing both legal and safety evidence must produce 2 holds, got %d", len(holds))
	}
	if holds[0].Code != HoldMissingLegalEvidence && holds[1].Code != HoldMissingLegalEvidence {
		t.Errorf("missing legal evidence hold must be present, got %+v", holds)
	}
	if holds[0].Code != HoldMissingSafetyEvidence && holds[1].Code != HoldMissingSafetyEvidence {
		t.Errorf("missing safety evidence hold must be present, got %+v", holds)
	}
}

func TestCaseActivationHoldsClearWhenEvidenceRecorded(t *testing.T) {
	now := time.Now().UTC()
	c := &Case{}
	c.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: "hash-legal"})

	holds := c.ActivationHolds(now)
	if len(holds) != 1 {
		t.Fatalf("only the safety hold must remain after legal evidence, got %+v", holds)
	}
	if holds[0].Code != HoldMissingSafetyEvidence {
		t.Errorf("remaining hold must be safety, got %q", holds[0].Code)
	}

	c.ApplyEvidence(Evidence{Kind: EvidenceKindSafety, ContentHash: "hash-safety"})
	if holds := c.ActivationHolds(now); len(holds) != 0 {
		t.Errorf("all evidence recorded must clear all holds, got %+v", holds)
	}
}

func TestCaseEvidencePresentRequiresContentHash(t *testing.T) {
	c := &Case{}
	c.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: ""})
	if c.EvidencePresent(EvidenceKindLegal) {
		t.Error("evidence without a content hash must not count as present")
	}
	c.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: "hash"})
	if !c.EvidencePresent(EvidenceKindLegal) {
		t.Error("evidence with a content hash must count as present")
	}
}

func TestCaseProgressTracksResumeSurface(t *testing.T) {
	c := &Case{
		Portfolio: &Portfolio{PropertyName: "Sea View Villa"},
		Goals:     &Goals{PrimaryGoal: "maximize occupancy"},
	}

	progress := c.Progress()
	if len(progress) != len(AllSteps) {
		t.Fatalf("progress must cover every checklist step, got %d", len(progress))
	}
	byKey := map[string]StepProgress{}
	for _, p := range progress {
		byKey[p.Key] = p
	}
	if !byKey[StepPortfolio].Complete {
		t.Error("recorded portfolio must be complete")
	}
	if !byKey[StepGoals].Complete {
		t.Error("recorded goals must be complete")
	}
	if byKey[StepLegalEvidence].Complete {
		t.Error("legal evidence must remain pending when not recorded")
	}
	if byKey[StepSafetyEvidence].Complete {
		t.Error("safety evidence must remain pending when not recorded")
	}
	if byKey[StepInspections].Complete {
		t.Error("inspections must remain pending when not recorded")
	}
}

func TestCaseAllStepsCompleteOnlyWhenEveryStepRecorded(t *testing.T) {
	c := &Case{}
	if c.AllStepsComplete() {
		t.Fatal("empty case must not be complete")
	}
	fillAllSteps(c)
	c.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: "l"})
	c.ApplyEvidence(Evidence{Kind: EvidenceKindSafety, ContentHash: "s"})
	if !c.AllStepsComplete() {
		t.Error("fully recorded case must report every step complete")
	}

	c.Portfolio = nil
	if c.AllStepsComplete() {
		t.Error("removing a section must make the case incomplete again")
	}
}

func TestCaseCanActivate(t *testing.T) {
	now := time.Now().UTC()

	incomplete := &Case{}
	if err := incomplete.CanActivate(now); !errors.Is(err, ErrActivationBlocked) {
		t.Errorf("empty case must be blocked on activation holds, got %v", err)
	}

	noEvidence := &Case{}
	fillAllSteps(noEvidence)
	if err := noEvidence.CanActivate(now); !errors.Is(err, ErrActivationBlocked) {
		t.Errorf("case without legal or safety evidence must be blocked, got %v", err)
	}

	complete := &Case{}
	fillAllSteps(complete)
	complete.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: "l"})
	complete.ApplyEvidence(Evidence{Kind: EvidenceKindSafety, ContentHash: "s"})
	if err := complete.CanActivate(now); err != nil {
		t.Errorf("complete case with all evidence must activate, got %v", err)
	}
}

func TestCaseCanActivateReportsIncompleteWhenAllEvidencePresent(t *testing.T) {
	now := time.Now().UTC()
	c := &Case{}
	c.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: "l"})
	c.ApplyEvidence(Evidence{Kind: EvidenceKindSafety, ContentHash: "s"})
	if err := c.CanActivate(now); !errors.Is(err, ErrIncomplete) {
		t.Errorf("all evidence but missing sections must report ErrIncomplete, got %v", err)
	}
}

func TestCaseRecomputeStatus(t *testing.T) {
	now := time.Now().UTC()

	c := &Case{}
	c.RecomputeStatus(now)
	if c.Status != StatusInProgress {
		t.Errorf("partial case must stay in_progress, got %q", c.Status)
	}

	full := &Case{}
	fillAllSteps(full)
	full.ApplyEvidence(Evidence{Kind: EvidenceKindLegal, ContentHash: "l"})
	full.ApplyEvidence(Evidence{Kind: EvidenceKindSafety, ContentHash: "s"})
	full.RecomputeStatus(now)
	if full.Status != StatusReady {
		t.Errorf("complete case must become ready, got %q", full.Status)
	}

	full.Status = StatusActivated
	full.Portfolio = nil
	full.RecomputeStatus(now)
	if full.Status != StatusActivated {
		t.Errorf("activated case must stay terminal, got %q", full.Status)
	}
}

func TestCaseApplySectionValidates(t *testing.T) {
	c := &Case{}

	if err := c.ApplySection("bogus", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidSection) {
		t.Errorf("unknown section must be rejected, got %v", err)
	}
	if err := c.ApplySection(StepPortfolio, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidSection) {
		t.Errorf("portfolio without property_name must be rejected, got %v", err)
	}

	valid := json.RawMessage(`{"property_name":"Sea View Villa","managed_units":1}`)
	if err := c.ApplySection(StepPortfolio, valid); err != nil {
		t.Fatalf("valid portfolio must apply: %v", err)
	}
	if c.Portfolio == nil || c.Portfolio.PropertyName != "Sea View Villa" {
		t.Errorf("portfolio section must be recorded, got %+v", c.Portfolio)
	}

	if err := c.ApplySection(StepGoals, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidSection) {
		t.Errorf("goals without primary_goal must be rejected, got %v", err)
	}
	if err := c.ApplySection(StepGoals, json.RawMessage(`{"primary_goal":"maximize occupancy"}`)); err != nil {
		t.Fatalf("valid goals must apply: %v", err)
	}
	if c.Goals == nil || c.Goals.PrimaryGoal != "maximize occupancy" {
		t.Errorf("goals section must be recorded, got %+v", c.Goals)
	}
}

func TestCaseApplySectionUpdatesExisting(t *testing.T) {
	c := &Case{}
	_ = c.ApplySection(StepFurnishing, json.RawMessage(`{"furnishing_level":"fully_furnished"}`))
	if c.Furnishing == nil || c.Furnishing.FurnishingLevel != "fully_furnished" {
		t.Fatalf("furnishing must be recorded: %+v", c.Furnishing)
	}
	if err := c.ApplySection(StepFurnishing, json.RawMessage(`{"furnishing_level":"semi_furnished"}`)); err != nil {
		t.Fatalf("resuming must allow updating a section: %v", err)
	}
	if c.Furnishing.FurnishingLevel != "semi_furnished" {
		t.Errorf("section update must replace the recorded value, got %q", c.Furnishing.FurnishingLevel)
	}
}

func TestCaseImmutableInspectionRecords(t *testing.T) {
	c := &Case{}
	insp := Inspection{
		ID:           "insp-1",
		CaseID:       "case-1",
		EvidenceHash: "sha256:abc",
		Findings:     "no issues",
	}
	c.ApplyInspection(insp)

	if len(c.Inspections) != 1 {
		t.Fatalf("inspection must be recorded, got %d", len(c.Inspections))
	}
	if c.Inspections[0].EvidenceHash != "sha256:abc" {
		t.Errorf("inspection evidence hash must be preserved unchanged, got %q", c.Inspections[0].EvidenceHash)
	}

	// A corrected inspection is a new record; the original is untouched.
	c.ApplyInspection(Inspection{
		ID:           "insp-2",
		CaseID:       "case-1",
		EvidenceHash: "sha256:def",
		Findings:     "minor fix noted",
	})
	if len(c.Inspections) != 2 {
		t.Fatalf("corrected inspection must append, got %d records", len(c.Inspections))
	}
	if c.Inspections[0].EvidenceHash != "sha256:abc" || c.Inspections[1].EvidenceHash != "sha256:def" {
		t.Errorf("original inspection evidence must remain stable, got %+v", c.Inspections)
	}
}

func fillAllSteps(c *Case) {
	c.Portfolio = &Portfolio{PropertyName: "Sea View Villa"}
	c.Goals = &Goals{PrimaryGoal: "maximize occupancy"}
	c.ServicePreferences = &ServicePreferences{CommunicationChannel: "email", Currency: "INR"}
	c.Budgets = &Budgets{Currency: "INR"}
	c.Contacts = []Contact{{Name: "Asha", Phone: "+91-9000000000"}}
	c.Photographs = []Photograph{{ObjectRef: "obj/1"}}
	c.Amenities = []Amenity{{Name: "wifi", Quantity: 1}}
	c.Safety = &Safety{SmokeDetectorsInstalled: true}
	c.Furnishing = &Furnishing{FurnishingLevel: "fully_furnished"}
	c.Remediation = &Remediation{}
	c.FitScoreInputs = &FitScoreInputs{PropertyScore: 8}
	c.Evidence = append(c.Evidence, Evidence{Kind: EvidenceKindDocument, ContentHash: "doc"})
	c.Inspections = []Inspection{{ID: "insp-1", EvidenceHash: "insp-hash"}}
}
