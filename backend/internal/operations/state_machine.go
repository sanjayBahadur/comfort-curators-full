package operations

var validTransitions = map[string]map[string]bool{
	StateDraft: {
		StateProposed:  true,
		StateCancelled: true,
	},
	StateProposed: {
		StateApproved:  true,
		StateRejected:  true,
		StateCancelled: true,
		StateDraft:     true,
	},
	StateApproved: {
		StateScheduled: true,
		StateCancelled: true,
		StateDraft:     true,
	},
	StateScheduled: {
		StateAssigned:  true,
		StateBlocked:   true,
		StateCancelled: true,
		StateDraft:     true,
	},
	StateAssigned: {
		StateInProgress: true,
		StateBlocked:    true,
		StateCancelled:  true,
		StateDraft:      true,
	},
	StateInProgress: {
		StateEvidenceSubmitted: true,
		StateBlocked:           true,
		StateCancelled:         true,
	},
	StateEvidenceSubmitted: {
		StateVerified:   true,
		StateInProgress: true,
		StateCancelled:  true,
	},
	StateVerified: {
		StateClosed:     true,
		StateInProgress: true,
	},
	StateClosed: {},
	StateBlocked: {
		StateScheduled:  true,
		StateAssigned:   true,
		StateInProgress: true,
		StateCancelled:  true,
	},
	StateCancelled: {},
	StateRejected: {
		StateDraft:     true,
		StateCancelled: true,
	},
}

func IsValidState(state string) bool {
	if state == "" {
		return false
	}
	_, ok := validTransitions[state]
	return ok
}

func CanTransition(from, to string) bool {
	edges, ok := validTransitions[from]
	if !ok {
		return false
	}
	return edges[to]
}

func AllowedNextStates(from string) []string {
	edges, ok := validTransitions[from]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(edges))
	for to := range edges {
		result = append(result, to)
	}
	return result
}

func ApplyTransition(t *Ticket, to string) error {
	if !IsValidState(to) {
		return ErrInvalidState
	}
	if !IsValidState(t.Status) {
		return ErrInvalidState
	}
	if IsTerminalState(t.Status) {
		return ErrTicketTerminal
	}
	if !CanTransition(t.Status, to) {
		return ErrInvalidTransition
	}

	if to == StateBlocked && t.Blocker == nil {
		return ErrBlockerRequired
	}

	t.Status = to
	t.Version++
	return nil
}
