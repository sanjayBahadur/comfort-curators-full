package onboarding

import (
	"encoding/json"
	"fmt"
	"time"
)

// ValidEvidenceKind reports whether kind is a supported evidence kind.
func ValidEvidenceKind(kind string) bool {
	for _, k := range ValidEvidenceKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// EvidencePresent reports whether the case holds at least one record of the
// given evidence kind. Evidence is append-only, so a recorded and non-withdrawn
// capture is conclusive.
func (c *Case) EvidencePresent(kind string) bool {
	for _, e := range c.Evidence {
		if e.Kind == kind && e.ContentHash != "" {
			return true
		}
	}
	return false
}

// ActivationHolds returns the holds that block activation at now. A case is
// blocked while mandatory legal or safety evidence is missing; recording the
// evidence clears the corresponding hold.
func (c *Case) ActivationHolds(now time.Time) []ActivationHold {
	var holds []ActivationHold
	if !c.EvidencePresent(EvidenceKindLegal) {
		holds = append(holds, ActivationHold{
			Code:    HoldMissingLegalEvidence,
			Message: "legal evidence has not been recorded",
		})
	}
	if !c.EvidencePresent(EvidenceKindSafety) {
		holds = append(holds, ActivationHold{
			Code:    HoldMissingSafetyEvidence,
			Message: "safety evidence has not been recorded",
		})
	}
	return holds
}

// CompletedSteps reports which checklist steps are already recorded. It is
// derived from persisted data so a case that was interrupted mid-way can be
// resumed by recording only the missing steps.
func (c *Case) CompletedSteps() map[string]bool {
	completed := map[string]bool{}
	if c.Portfolio != nil {
		completed[StepPortfolio] = true
	}
	if c.Goals != nil {
		completed[StepGoals] = true
	}
	if c.ServicePreferences != nil {
		completed[StepServicePreferences] = true
	}
	if c.Budgets != nil {
		completed[StepBudgets] = true
	}
	if len(c.Contacts) > 0 {
		completed[StepContacts] = true
	}
	if len(c.Photographs) > 0 {
		completed[StepPhotographs] = true
	}
	if len(c.Amenities) > 0 {
		completed[StepAmenities] = true
	}
	if c.Safety != nil {
		completed[StepSafety] = true
	}
	if c.Furnishing != nil {
		completed[StepFurnishing] = true
	}
	if c.Remediation != nil {
		completed[StepRemediation] = true
	}
	if c.FitScoreInputs != nil {
		completed[StepFitScoreInputs] = true
	}
	for _, e := range c.Evidence {
		switch e.Kind {
		case EvidenceKindDocument:
			completed[StepDocuments] = true
		case EvidenceKindLegal:
			completed[StepLegalEvidence] = true
		case EvidenceKindSafety:
			completed[StepSafetyEvidence] = true
		}
	}
	if len(c.Inspections) > 0 {
		completed[StepInspections] = true
	}
	return completed
}

// Progress returns the full checklist with per-step completion state so the
// resume view exposes exactly what is left.
func (c *Case) Progress() []StepProgress {
	completed := c.CompletedSteps()
	out := make([]StepProgress, 0, len(AllSteps))
	for _, key := range AllSteps {
		out = append(out, StepProgress{Key: key, Complete: completed[key]})
	}
	return out
}

// AllStepsComplete reports whether every checklist step has been recorded.
func (c *Case) AllStepsComplete() bool {
	for _, key := range AllSteps {
		if !c.CompletedSteps()[key] {
			return false
		}
	}
	return true
}

// CanActivate returns nil only when the case may be activated: no activation
// holds and every checklist step recorded. Missing legal or safety evidence
// surfaces as ErrActivationBlocked; any other missing step surfaces as
// ErrIncomplete.
func (c *Case) CanActivate(now time.Time) error {
	if holds := c.ActivationHolds(now); len(holds) > 0 {
		return fmt.Errorf("%w: %v", ErrActivationBlocked, holds[0].Code)
	}
	if !c.AllStepsComplete() {
		return ErrIncomplete
	}
	return nil
}

// ApplySection records a typed onboarding section onto the case. Every section
// persists independently so an interrupted case keeps its committed progress.
func (c *Case) ApplySection(name string, data json.RawMessage) error {
	column, ok := sectionColumn(name)
	if !ok {
		return ErrInvalidSection
	}
	decoded, err := validateSection(name, data)
	if err != nil {
		return err
	}
	switch column {
	case "portfolio":
		var v Portfolio
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Portfolio = &v
	case "goals":
		var v Goals
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Goals = &v
	case "service_preferences":
		var v ServicePreferences
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.ServicePreferences = &v
	case "budgets":
		var v Budgets
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Budgets = &v
	case "photographs":
		var v []Photograph
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Photographs = v
	case "amenities":
		var v []Amenity
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Amenities = v
	case "safety":
		var v Safety
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Safety = &v
	case "furnishing":
		var v Furnishing
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Furnishing = &v
	case "remediation":
		var v Remediation
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.Remediation = &v
	case "fit_score_inputs":
		var v FitScoreInputs
		if err := json.Unmarshal(decoded, &v); err != nil {
			return err
		}
		c.FitScoreInputs = &v
	}
	return nil
}

// ApplyContacts records the owner contact list.
func (c *Case) ApplyContacts(contacts []Contact) {
	c.Contacts = contacts
}

// ApplyEvidence appends one immutable evidence record.
func (c *Case) ApplyEvidence(e Evidence) {
	c.Evidence = append(c.Evidence, e)
}

// ApplyInspection appends one immutable inspection record.
func (c *Case) ApplyInspection(i Inspection) {
	c.Inspections = append(c.Inspections, i)
}

// RecomputeStatus derives the aggregate status from its persisted content. An
// activated case is terminal; otherwise a fully recorded case with no holds is
// ready and any gap keeps the case in progress.
func (c *Case) RecomputeStatus(now time.Time) {
	if c.Status == StatusActivated {
		return
	}
	if len(c.ActivationHolds(now)) == 0 && c.AllStepsComplete() {
		c.Status = StatusReady
		return
	}
	c.Status = StatusInProgress
}

// sectionColumn maps a section name to its persistence column.
func sectionColumn(name string) (string, bool) {
	switch name {
	case StepPortfolio:
		return "portfolio", true
	case StepGoals:
		return "goals", true
	case StepServicePreferences:
		return "service_preferences", true
	case StepBudgets:
		return "budgets", true
	case StepPhotographs:
		return "photographs", true
	case StepAmenities:
		return "amenities", true
	case StepSafety:
		return "safety", true
	case StepFurnishing:
		return "furnishing", true
	case StepRemediation:
		return "remediation", true
	case StepFitScoreInputs:
		return "fit_score_inputs", true
	}
	return "", false
}

// validateSection decodes and re-encodes a section payload into its typed
// shape, enforcing the mandatory inputs of each recordable section.
func validateSection(name string, data json.RawMessage) (json.RawMessage, error) {
	switch name {
	case StepPortfolio:
		var v Portfolio
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		if v.PropertyName == "" {
			return nil, fmt.Errorf("%w: portfolio.property_name is required", ErrInvalidSection)
		}
		if v.ManagedUnits < 0 {
			return nil, fmt.Errorf("%w: portfolio.managed_units cannot be negative", ErrInvalidSection)
		}
		return json.Marshal(v)
	case StepGoals:
		var v Goals
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		if v.PrimaryGoal == "" {
			return nil, fmt.Errorf("%w: goals.primary_goal is required", ErrInvalidSection)
		}
		return json.Marshal(v)
	case StepServicePreferences:
		var v ServicePreferences
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		if v.CommunicationChannel == "" {
			return nil, fmt.Errorf("%w: service_preferences.communication_channel is required", ErrInvalidSection)
		}
		if v.Currency == "" {
			return nil, fmt.Errorf("%w: service_preferences.currency is required", ErrInvalidSection)
		}
		return json.Marshal(v)
	case StepBudgets:
		var v Budgets
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		if v.Currency == "" {
			return nil, fmt.Errorf("%w: budgets.currency is required", ErrInvalidSection)
		}
		return json.Marshal(v)
	case StepPhotographs:
		var v []Photograph
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		for _, p := range v {
			if p.ObjectRef == "" {
				return nil, fmt.Errorf("%w: photograph.object_ref is required", ErrInvalidSection)
			}
		}
		return json.Marshal(v)
	case StepAmenities:
		var v []Amenity
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		for _, a := range v {
			if a.Name == "" {
				return nil, fmt.Errorf("%w: amenity.name is required", ErrInvalidSection)
			}
		}
		return json.Marshal(v)
	case StepSafety:
		var v Safety
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return json.Marshal(v)
	case StepFurnishing:
		var v Furnishing
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		if v.FurnishingLevel == "" {
			return nil, fmt.Errorf("%w: furnishing.furnishing_level is required", ErrInvalidSection)
		}
		return json.Marshal(v)
	case StepRemediation:
		var v Remediation
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return json.Marshal(v)
	case StepFitScoreInputs:
		var v FitScoreInputs
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return json.Marshal(v)
	}
	return nil, ErrInvalidSection
}
