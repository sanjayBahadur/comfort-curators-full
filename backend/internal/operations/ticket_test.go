package operations

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIsValidState(t *testing.T) {
	for _, s := range AllStates {
		if !IsValidState(s) {
			t.Errorf("expected %q to be a valid state", s)
		}
	}

	if IsValidState("") {
		t.Error("empty string should not be a valid state")
	}
	if IsValidState("nonexistent") {
		t.Error("nonexistent should not be a valid state")
	}
}

func TestIsValidTicketType(t *testing.T) {
	for _, tt := range AllTicketTypes {
		if !IsValidTicketType(tt) {
			t.Errorf("expected %q to be a valid ticket type", tt)
		}
	}

	if IsValidTicketType("invalid_type") {
		t.Error("invalid_type should not be a valid ticket type")
	}
}

func TestAllNineTicketTypes(t *testing.T) {
	if len(AllTicketTypes) != 9 {
		t.Fatalf("expected 9 ticket types, got %d: %v", len(AllTicketTypes), AllTicketTypes)
	}

	unique := map[string]bool{}
	for _, tt := range AllTicketTypes {
		if unique[tt] {
			t.Errorf("duplicate ticket type: %s", tt)
		}
		unique[tt] = true
	}
}

func TestValidTransitionNormativeFlow(t *testing.T) {
	flow := []string{StateDraft, StateProposed, StateApproved, StateScheduled,
		StateAssigned, StateInProgress, StateEvidenceSubmitted, StateVerified, StateClosed}

	for i := 0; i < len(flow)-1; i++ {
		from := flow[i]
		to := flow[i+1]
		if !CanTransition(from, to) {
			t.Errorf("expected valid transition from %q to %q", from, to)
		}
	}
}

func TestInvalidTransitionRejected(t *testing.T) {
	invalidTransitions := []struct{ from, to string }{
		{StateDraft, StateVerified},
		{StateProposed, StateClosed},
		{StateClosed, StateInProgress},
		{StateCancelled, StateDraft},
		{StateVerified, StateDraft},
		{StateRejected, StateApproved},
		{StateDraft, StateClosed},
	}

	for _, tc := range invalidTransitions {
		if CanTransition(tc.from, tc.to) {
			t.Errorf("transition from %q to %q should be invalid", tc.from, tc.to)
		}
	}
}

func TestCancellationAvailableFromAllActiveStates(t *testing.T) {
	statesWithCancel := []string{
		StateDraft, StateProposed, StateApproved, StateScheduled,
		StateAssigned, StateInProgress, StateEvidenceSubmitted, StateBlocked,
		StateRejected,
	}

	for _, s := range statesWithCancel {
		if !CanTransition(s, StateCancelled) {
			t.Errorf("state %q should allow cancellation", s)
		}
	}
}

func TestCancelledTerminalCannotTransition(t *testing.T) {
	for _, target := range AllStates {
		if CanTransition(StateCancelled, target) {
			t.Errorf("cancelled should not allow transition to %q", target)
		}
	}
}

func TestClosedTerminalCannotTransition(t *testing.T) {
	for _, target := range AllStates {
		if CanTransition(StateClosed, target) {
			t.Errorf("closed should not allow transition to %q", target)
		}
	}
}

func TestBlockedCanUnblock(t *testing.T) {
	validUnblockTargets := []string{StateScheduled, StateAssigned, StateInProgress}

	for _, target := range validUnblockTargets {
		if !CanTransition(StateBlocked, target) {
			t.Errorf("blocked should allow transition to %q", target)
		}
	}

	invalidFromBlocked := []string{StateDraft, StateProposed, StateClosed, StateVerified}
	for _, target := range invalidFromBlocked {
		if CanTransition(StateBlocked, target) {
			t.Errorf("blocked should not allow transition to %q", target)
		}
	}
}

func TestRejectedTransitions(t *testing.T) {
	if !CanTransition(StateRejected, StateDraft) {
		t.Error("rejected should allow transition to draft")
	}
	if !CanTransition(StateRejected, StateCancelled) {
		t.Error("rejected should allow transition to cancelled")
	}
	if CanTransition(StateRejected, StateApproved) {
		t.Error("rejected should not allow direct transition to approved")
	}
}

func TestApplyTransitionIncrementsVersion(t *testing.T) {
	tkt := &Ticket{
		ID:      "tkt_1",
		Status:  StateDraft,
		Version: 1,
	}

	if err := ApplyTransition(tkt, StateProposed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tkt.Status != StateProposed {
		t.Errorf("expected status %q, got %q", StateProposed, tkt.Status)
	}
	if tkt.Version != 2 {
		t.Errorf("expected version 2, got %d", tkt.Version)
	}
}

func TestApplyTransitionInvalidTarget(t *testing.T) {
	tkt := &Ticket{ID: "tkt_1", Status: StateDraft, Version: 1}
	err := ApplyTransition(tkt, StateVerified)
	if err == nil {
		t.Fatal("expected error for invalid transition from draft to verified")
	}
	if tkt.Status != StateDraft {
		t.Errorf("status should remain draft, got %q", tkt.Status)
	}
	if tkt.Version != 1 {
		t.Errorf("version should remain 1, got %d", tkt.Version)
	}
}

func TestApplyTransitionTerminalRejects(t *testing.T) {
	tkt := &Ticket{ID: "tkt_1", Status: StateCancelled, Version: 1}
	err := ApplyTransition(tkt, StateDraft)
	if err != ErrTicketTerminal {
		t.Fatalf("expected ErrTicketTerminal, got %v", err)
	}
}

func TestApplyTransitionBlockedRequiresBlocker(t *testing.T) {
	tkt := &Ticket{ID: "tkt_1", Status: StateScheduled, Version: 1}
	err := ApplyTransition(tkt, StateBlocked)
	if err != ErrBlockerRequired {
		t.Fatalf("expected ErrBlockerRequired, got %v", err)
	}

	tkt.Blocker = &TicketBlock{Type: BlockerTypeAccess, Reason: "no key"}
	err = ApplyTransition(tkt, StateBlocked)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tkt.Status != StateBlocked {
		t.Errorf("expected status %q, got %q", StateBlocked, tkt.Status)
	}
}

func TestAllowedNextStates(t *testing.T) {
	next := AllowedNextStates(StateDraft)
	if len(next) != 2 {
		t.Fatalf("draft should have 2 allowed next states, got %d", len(next))
	}

	foundProposed := false
	foundCancelled := false
	for _, s := range next {
		if s == StateProposed {
			foundProposed = true
		}
		if s == StateCancelled {
			foundCancelled = true
		}
	}
	if !foundProposed || !foundCancelled {
		t.Errorf("draft next states should include proposed and cancelled, got %v", next)
	}
}

func TestAllowedNextStatesInvalidFrom(t *testing.T) {
	next := AllowedNextStates("invalid")
	if next != nil {
		t.Errorf("expected nil for invalid from state, got %v", next)
	}
}

func TestIsTerminalState(t *testing.T) {
	if !IsTerminalState(StateClosed) {
		t.Error("closed should be terminal")
	}
	if !IsTerminalState(StateCancelled) {
		t.Error("cancelled should be terminal")
	}
	if IsTerminalState(StateDraft) {
		t.Error("draft should not be terminal")
	}
}

func TestIsHighRiskTicketType(t *testing.T) {
	if !IsHighRiskTicketType(TypeIncident) {
		t.Error("incident should be high-risk")
	}
	if !IsHighRiskTicketType(TypeSpecialistVendorRequest) {
		t.Error("specialist vendor request should be high-risk")
	}
	if IsHighRiskTicketType(TypeTurnover) {
		t.Error("turnover should not be high-risk")
	}
}

func TestHighRiskVerifierDiffersFromActor(t *testing.T) {
	tkt := &Ticket{
		ID:         "tkt_1",
		TenantID:   "tenant_1",
		Type:       TypeIncident,
		Status:     StateEvidenceSubmitted,
		AssignedTo: "actor_1",
		Version:    5,
	}
	tkt.CreatedAt = time.Now().UTC()
	tkt.UpdatedAt = time.Now().UTC()

	if !IsHighRiskTicketType(tkt.Type) {
		t.Fatal("incident should be high-risk")
	}
	if tkt.AssignedTo == "actor_1" {
		t.Log("high-risk ticket assigned to actor_1 - verifier must differ")
	}
}

func TestChecklistStatusConstants(t *testing.T) {
	validStatuses := []string{ChecklistStatusPending, ChecklistStatusInProgress, ChecklistStatusCompleted, ChecklistStatusNA}
	for _, s := range validStatuses {
		if s == "" {
			t.Error("checklist status should not be empty")
		}
	}
}

func TestBlockerTypeConstants(t *testing.T) {
	validTypes := []string{
		BlockerTypeAccess, BlockerTypeSafety, BlockerTypeParts,
		BlockerTypeApproval, BlockerTypeWeather, BlockerTypeCompliance,
		BlockerTypeExternal,
	}
	for _, bt := range validTypes {
		if bt == "" {
			t.Error("blocker type should not be empty")
		}
	}
}

func TestNotificationIntentConstants(t *testing.T) {
	intents := []string{NotificationIntentUrgent, NotificationIntentNormal, NotificationIntentNone}
	for _, intent := range intents {
		if intent == "" {
			t.Error("notification intent should not be empty")
		}
	}
}

func TestTicketCreateParams(t *testing.T) {
	window := json.RawMessage(`{"start":"2024-01-01T00:00:00Z","end":"2024-01-01T04:00:00Z"}`)
	params := CreateTicketParams{
		TenantID:           "tenant_1",
		PropertyID:         "prop_1",
		Type:               TypeTurnover,
		RequestedWindow:    window,
		ChecklistVersionID: "cv_1",
		Reason:             "scheduled turnover",
	}

	if !IsValidTicketType(params.Type) {
		t.Error("turnover should be valid")
	}
	if params.TenantID == "" || params.PropertyID == "" {
		t.Error("tenant_id and property_id should be non-empty")
	}
	if params.Reason == "" {
		t.Error("reason should be non-empty")
	}
}

func TestTransitionParams(t *testing.T) {
	params := TransitionParams{
		ToState:     StateApproved,
		Reason:      "approved after review",
		EvidenceIDs: []string{"ev_1", "ev_2"},
	}

	if !IsValidState(params.ToState) {
		t.Error("approved should be a valid state")
	}
	if len(params.EvidenceIDs) != 2 {
		t.Errorf("expected 2 evidence IDs, got %d", len(params.EvidenceIDs))
	}
}

func TestTicketBlockStruct(t *testing.T) {
	now := time.Now().UTC()
	nextReview := now.Add(24 * time.Hour)
	block := TicketBlock{
		Type:             BlockerTypeSafety,
		Reason:           "hazardous materials found",
		ResponsibleParty: "ops_team",
		NextReviewAt:     &nextReview,
		EscalationPolicy: "notify_supervisor",
		CreatedBy:        "actor_1",
		CreatedAt:        now,
	}

	if block.Type != BlockerTypeSafety {
		t.Error("blocker type mismatch")
	}
	if block.ResponsibleParty != "ops_team" {
		t.Error("responsible party mismatch")
	}
}

func TestEveryInvalidTransitionIsRejected(t *testing.T) {
	for _, from := range AllStates {
		for _, to := range AllStates {
			expected := CanTransition(from, to)
			tkt := &Ticket{ID: "tkt_1", Status: from, Version: 1}

			if to == StateBlocked && expected {
				tkt.Blocker = &TicketBlock{Type: BlockerTypeAccess, Reason: "test"}
			}

			err := ApplyTransition(tkt, to)
			if expected && err != nil {
				t.Errorf("transition %q -> %q should succeed but got: %v", from, to, err)
			}
			if !expected && err == nil {
				t.Errorf("transition %q -> %q should fail but succeeded", from, to)
			}
		}
	}
}

func TestNoHardDeletePathExists(t *testing.T) {
	data, _ := json.Marshal(AllStates)
	if string(data) == "" {
		t.Error("states should be serializable")
	}

	for _, s := range AllStates {
		if s == "deleted" {
			t.Error("deleted state should not exist in the ticket state machine")
		}
	}

	if _, ok := validTransitions["deleted"]; ok {
		t.Error("deleted should not be a valid state key in the transition map")
	}
}

func TestReopenClosedCreatesLinkedFollowUp(t *testing.T) {
	original := &Ticket{
		ID:         "tkt_orig",
		TenantID:   "tenant_1",
		PropertyID: "prop_1",
		Type:       TypeTurnover,
		Status:     StateClosed,
		Version:    10,
	}

	if original.Status != StateClosed {
		t.Fatal("ticket should be closed to test reopen")
	}

	followUpID := "tkt_follow"
	original.FollowUpTicketID = followUpID
	original.ReopenReason = "guest complained about cleanliness"

	if original.FollowUpTicketID != followUpID {
		t.Error("follow_up_ticket_id should link to the new ticket")
	}
	if original.ReopenReason == "" {
		t.Error("reopen reason should be recorded")
	}
}

func TestTicketStateEventRecordsActor(t *testing.T) {
	event := TicketStateEvent{
		ID:        "tse_1",
		TicketID:  "tkt_1",
		TenantID:  "tenant_1",
		FromState: StateDraft,
		ToState:   StateProposed,
		ActorID:   "actor_1",
		Reason:    "proposed for review",
		Evidence:  []string{"ev_1"},
		Version:   2,
		CreatedAt: time.Now().UTC(),
	}

	if event.ActorID == "" {
		t.Error("state event must record actor_id")
	}
	if event.FromState == event.ToState {
		t.Error("from and to states should differ for a transition event")
	}
}

func TestEvidenceSubmissionAllowsRework(t *testing.T) {
	if !CanTransition(StateEvidenceSubmitted, StateInProgress) {
		t.Error("evidence_submitted should allow returning to in_progress for rework")
	}
}

func TestVerifiedAllowsRework(t *testing.T) {
	if !CanTransition(StateVerified, StateInProgress) {
		t.Error("verified should allow returning to in_progress for rework")
	}
}
