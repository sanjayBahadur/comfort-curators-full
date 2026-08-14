package property_test

import (
	"errors"
	"testing"

	"comfort-curators-backend/internal/property"
)

func TestPropertyLifecycleStatesAreComplete(t *testing.T) {
	want := []string{
		"lead",
		"qualifying",
		"onboarding",
		"remediation",
		"ready_inactive",
		"active",
		"paused",
		"suspended",
		"offboarding",
		"archived",
	}
	if len(property.AllStates) != len(want) {
		t.Fatalf("expected %d lifecycle states, got %d", len(want), len(property.AllStates))
	}
	for i, state := range want {
		if property.AllStates[i] != state {
			t.Errorf("state %d: expected %q, got %q", i, state, property.AllStates[i])
		}
		if !property.IsValidState(state) {
			t.Errorf("IsValidState(%q) must be true", state)
		}
	}
}

func TestPropertyIsValidStateRejectsUnknown(t *testing.T) {
	for _, invalid := range []string{"", "INACTIVE", "ready", "listed", "unknown", "active "} {
		if property.IsValidState(invalid) {
			t.Errorf("IsValidState(%q) must be false", invalid)
		}
	}
}

func TestPropertyValidLifecyclePathSucceeds(t *testing.T) {
	p := &property.Property{State: property.StateLead, Version: 1}
	path := []string{
		property.StateQualifying,
		property.StateOnboarding,
		property.StateRemediation,
		property.StateReadyInactive,
		property.StateActive,
	}
	for _, to := range path {
		if err := property.ApplyTransition(p, to); err != nil {
			t.Fatalf("transition to %q failed: %v", to, err)
		}
		if p.State != to {
			t.Errorf("expected state %q, got %q", to, p.State)
		}
	}
	if p.Version != len(path)+1 {
		t.Errorf("expected version %d after %d transitions, got %d", len(path)+1, len(path), p.Version)
	}
}

func TestPropertyInvalidSkipsFail(t *testing.T) {
	cases := []struct {
		from string
		to   string
	}{
		{property.StateLead, property.StateActive},
		{property.StateLead, property.StateReadyInactive},
		{property.StateLead, property.StateOnboarding},
		{property.StateQualifying, property.StateActive},
		{property.StateOnboarding, property.StateActive},
		{property.StateRemediation, property.StateActive},
	}
	for _, tc := range cases {
		p := &property.Property{State: tc.from, Version: 1}
		err := property.ApplyTransition(p, tc.to)
		if !errors.Is(err, property.ErrInvalidTransition) {
			t.Errorf("transition %s -> %s must fail with ErrInvalidTransition, got %v", tc.from, tc.to, err)
		}
		if p.State != tc.from {
			t.Errorf("invalid transition %s -> %s must not mutate state, got %q", tc.from, tc.to, p.State)
		}
		if p.Version != 1 {
			t.Errorf("invalid transition must not bump version, got %d", p.Version)
		}
	}
}

func TestPropertySelfTransitionsFail(t *testing.T) {
	for _, state := range property.AllStates {
		p := &property.Property{State: state, Version: 1}
		if err := property.ApplyTransition(p, state); err == nil {
			t.Errorf("self-transition %s -> %s must fail", state, state)
		}
	}
}

func TestPropertyArchivedIsTerminal(t *testing.T) {
	p := &property.Property{State: property.StateArchived, Version: 1}
	for _, to := range property.AllStates {
		err := property.ApplyTransition(p, to)
		if !errors.Is(err, property.ErrArchivedTerminal) {
			t.Errorf("archived -> %s must fail with ErrArchivedTerminal, got %v", to, err)
		}
	}
}

func TestPropertyOffboardingArchives(t *testing.T) {
	for _, from := range property.AllStates {
		if from == property.StateArchived {
			continue
		}
		p := &property.Property{State: from, Version: 1}
		if err := property.ApplyTransition(p, property.StateArchived); err != nil {
			t.Errorf("%s -> archived must be allowed, got %v", from, err)
		}
	}
}

func TestPropertyInvalidStatesRejected(t *testing.T) {
	for _, to := range []string{"", "listed", "ACTIVE"} {
		p := &property.Property{State: property.StateLead, Version: 1}
		if err := property.ApplyTransition(p, to); !errors.Is(err, property.ErrInvalidState) {
			t.Errorf("transition to %q must fail with ErrInvalidState, got %v", to, err)
		}
	}
}

func TestPropertyReadyInactiveAndActiveAreDistinct(t *testing.T) {
	if property.StateReadyInactive == property.StateActive {
		t.Fatal("ready_inactive and active must be distinct states")
	}

	ready := &property.Property{State: property.StateReadyInactive, Version: 3}
	if ready.State == property.StateActive {
		t.Fatal("a ready-inactive property must not report as active")
	}

	active := &property.Property{State: property.StateActive, Version: 4}
	if err := property.ApplyTransition(ready, property.StateActive); err != nil {
		t.Fatalf("ready_inactive -> active must succeed: %v", err)
	}
	if ready.State != property.StateActive {
		t.Errorf("after activation expected active, got %q", ready.State)
	}
	if active.State == ready.State && active.State == property.StateReadyInactive {
		t.Error("active and ready_inactive must remain distinct")
	}
}

func TestPropertyLifecycleBranching(t *testing.T) {
	// Valid branch transitions that the operationally meaningful paths rely on.
	cases := []struct {
		from string
		to   string
		want bool
	}{
		{property.StateActive, property.StatePaused, true},
		{property.StateActive, property.StateSuspended, true},
		{property.StatePaused, property.StateActive, true},
		{property.StateSuspended, property.StatePaused, true},
		{property.StateActive, property.StateReadyInactive, false},
		{property.StateReadyInactive, property.StatePaused, false},
	}
	for _, tc := range cases {
		if got := property.CanTransition(tc.from, tc.to); got != tc.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}
