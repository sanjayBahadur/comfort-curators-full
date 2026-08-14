package property

// validTransitions is the frozen V0 property lifecycle. Transitions are
// explicit: a property may only move between the states listed here, so an
// attempted skip (for example lead -> active) fails instead of silently
// allowing an unvetted property to become operational.
var validTransitions = map[string]map[string]bool{
	StateLead: {
		StateQualifying:  true,
		StateOffboarding: true,
		StateArchived:    true,
	},
	StateQualifying: {
		StateOnboarding:  true,
		StateLead:        true,
		StateOffboarding: true,
		StateArchived:    true,
	},
	StateOnboarding: {
		StateRemediation:   true,
		StateReadyInactive: true,
		StateOffboarding:   true,
		StateArchived:      true,
	},
	StateRemediation: {
		StateOnboarding:    true,
		StateReadyInactive: true,
		StateOffboarding:   true,
		StateArchived:      true,
	},
	StateReadyInactive: {
		StateActive:      true,
		StateOffboarding: true,
		StateArchived:    true,
	},
	StateActive: {
		StatePaused:      true,
		StateSuspended:   true,
		StateOffboarding: true,
		StateArchived:    true,
	},
	StatePaused: {
		StateActive:      true,
		StateOffboarding: true,
		StateArchived:    true,
	},
	StateSuspended: {
		StatePaused:      true,
		StateOffboarding: true,
		StateArchived:    true,
	},
	StateOffboarding: {
		StateArchived: true,
	},
	StateArchived: {},
}

// IsValidState reports whether state is one of the frozen lifecycle states.
func IsValidState(state string) bool {
	if state == "" {
		return false
	}
	_, ok := validTransitions[state]
	return ok
}

// CanTransition reports whether a transition from the current state to the
// target state is an explicit edge of the frozen lifecycle.
func CanTransition(from, to string) bool {
	edges, ok := validTransitions[from]
	if !ok {
		return false
	}
	return edges[to]
}

// ApplyTransition moves the aggregate to the target state and bumps its
// version. It enforces the state machine only; activation readiness and
// compliance holds are evaluated by the caller before calling this.
func ApplyTransition(p *Property, to string) error {
	if !IsValidState(to) {
		return ErrInvalidState
	}
	if !IsValidState(p.State) {
		return ErrInvalidState
	}
	if p.State == StateArchived {
		return ErrArchivedTerminal
	}
	if !CanTransition(p.State, to) {
		return ErrInvalidTransition
	}
	p.State = to
	p.Version++
	return nil
}
