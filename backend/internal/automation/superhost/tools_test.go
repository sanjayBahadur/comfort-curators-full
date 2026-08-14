package superhost_test

import (
	"encoding/json"
	"testing"

	"comfort-curators-backend/internal/automation/superhost"
)

func TestSuperhostToolRegistryHasAllAllowedTools(t *testing.T) {
	expected := []string{
		"get_property_operating_summary",
		"get_reservation_change",
		"propose_turnover_ticket",
		"propose_inspection_ticket",
		"propose_restock",
		"propose_maintenance_request",
		"propose_incident_report",
		"request_owner_approval",
		"request_operations_approval",
		"send_approved_notification",
		"assemble_document_packet",
		"summarize_incident",
		"escalate_exception",
		"ui_focus",
		"ui_set_value",
		"ui_click",
		"ui_scroll_to",
		"ui_open_panel",
		"list_my_tasks",
		"log_task",
		"resolve_task",
	}

	allowed := superhost.AllowedToolNames()

	if len(allowed) != len(expected) {
		t.Fatalf("expected %d tools, got %d: %v", len(expected), len(allowed), allowed)
	}

	allowedMap := make(map[string]bool)
	for _, name := range allowed {
		allowedMap[name] = true
	}

	for _, name := range expected {
		if !allowedMap[name] {
			t.Errorf("missing required tool: %s", name)
		}
	}
}

func TestSuperhostDirectMutationToolDoesNotExist(t *testing.T) {
	mutationNames := []string{
		"delete_reservation",
		"update_ticket_status",
		"write_inventory",
		"mutate_property",
		"insert_record",
		"upsert_calendar",
		"set_property_config",
		"put_agreement",
		"patch_ticket",
		"pay_invoice",
		"refund_charge",
		"transfer_funds",
		"sign_contract",
		"certify_document",
		"file_legal_report",
		"disclose_access_secret",
		"terminate_worker",
		"suspend_worker",
		"reject_worker",
		"create_order",
		"approve_order",
		"hard_delete_record",
		"purge_data",
		"wipe_records",
		"erase_evidence",
	}

	for _, name := range mutationNames {
		_, err := superhost.LookupTool(name)
		if err == nil {
			t.Errorf("mutation tool %q should not exist in registry", name)
		}
	}
}

func TestSuperhostLookupAllowedTools(t *testing.T) {
	for _, name := range superhost.AllowedToolNames() {
		def, err := superhost.LookupTool(name)
		if err != nil {
			t.Errorf("allowed tool %q lookup failed: %v", name, err)
			continue
		}
		if def.Name != name {
			t.Errorf("tool %q name mismatch: %s", name, def.Name)
		}
		if def.SchemaVersion != superhost.ToolSchemaVersionCurrent {
			t.Errorf("tool %q wrong version: %s", name, def.SchemaVersion)
		}
	}
}

func TestSuperhostUnknownToolDenied(t *testing.T) {
	_, err := superhost.LookupTool("fabricate_receipt")
	if err == nil {
		t.Fatal("unknown tool should not be found")
	}
}

func TestSuperhostToolVersionValidation(t *testing.T) {
	err := superhost.ValidateToolVersion("get_property_operating_summary", "v1")
	if err != nil {
		t.Errorf("valid version should pass: %v", err)
	}

	err = superhost.ValidateToolVersion("get_property_operating_summary", "v99")
	if err == nil {
		t.Error("wrong version should be rejected")
	}

	err = superhost.ValidateToolVersion("nonexistent_tool", "v1")
	if err == nil {
		t.Error("nonexistent tool with valid version should still be rejected")
	}
}

func TestSuperhostToolKinds(t *testing.T) {
	def, _ := superhost.LookupTool("get_property_operating_summary")
	if def.Kind != superhost.ToolKindRead {
		t.Errorf("expected read tool, got %s", def.Kind)
	}

	def, _ = superhost.LookupTool("propose_turnover_ticket")
	if def.Kind != superhost.ToolKindPropose {
		t.Errorf("expected propose tool, got %s", def.Kind)
	}

	def, _ = superhost.LookupTool("request_owner_approval")
	if def.Kind != superhost.ToolKindRequest {
		t.Errorf("expected request tool, got %s", def.Kind)
	}
}

func TestSuperhostToolIsMutation(t *testing.T) {
	readDef, _ := superhost.LookupTool("get_property_operating_summary")
	if readDef.IsMutation() {
		t.Error("read tool should not be a mutation")
	}

	proposeDef, _ := superhost.LookupTool("propose_turnover_ticket")
	if !proposeDef.IsMutation() {
		t.Error("propose tool should be recorded as mutation")
	}
}

func TestSuperhostToolScopeValidation(t *testing.T) {
	input := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-001",
		Arguments: json.RawMessage(`{"property_id": "prop-A", "tenant_id": "tenant-A", "reason": "test"}`),
	}

	err := input.ValidateScope("tenant-A", "prop-A")
	if err != nil {
		t.Errorf("matching scope should pass: %v", err)
	}

	inputCrossTenant := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-002",
		Arguments: json.RawMessage(`{"tenant_id": "tenant-B"}`),
	}
	err = inputCrossTenant.ValidateScope("tenant-A", "prop-A")
	if err == nil {
		t.Error("cross-tenant should be rejected")
	}

	inputCrossProperty := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-003",
		Arguments: json.RawMessage(`{"property_id": "prop-B"}`),
	}
	err = inputCrossProperty.ValidateScope("tenant-A", "prop-A")
	if err == nil {
		t.Error("cross-property should be rejected")
	}

	inputNoScope := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-004",
		Arguments: json.RawMessage(`{}`),
	}
	err = inputNoScope.ValidateScope("tenant-A", "prop-A")
	if err != nil {
		t.Errorf("no scope override should pass: %v", err)
	}
}

func TestPolicyEngineReadToolAllowed(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	input := superhost.ToolCallInput{
		ToolName:  "get_property_operating_summary",
		Version:   "v1",
		CallID:    "call-001",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyAllowed {
		t.Errorf("read tool should be allowed, got %s: %s", decision.Result, decision.Reason)
	}
	if decision.InputClass != string(superhost.ToolKindRead) {
		t.Errorf("input class should be read, got %s", decision.InputClass)
	}
}

func TestPolicyEngineProposeToolRequiresApproval(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	input := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-002",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyApprovalRequired {
		t.Errorf("propose tool should require approval, got %s: %s", decision.Result, decision.Reason)
	}
}

func TestPolicyEngineDeniesToolOutsideActorRole(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-guest-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-guest-001",
		ActorRoles: []string{"guest"},
	}

	// request_owner_approval is owner-only -- a guest actor must be
	// denied, not silently allowed the way every tool was before
	// ActorRoles was actually populated and enforced.
	input := superhost.ToolCallInput{
		ToolName:  "request_owner_approval",
		Version:   "v1",
		CallID:    "call-guest-001",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyDenied {
		t.Errorf("owner-only tool should be denied for a guest actor, got %s: %s", decision.Result, decision.Reason)
	}
}

func TestPolicyEngineAllowsToolSharedAcrossRoles(t *testing.T) {
	pe := superhost.NewPolicyEngine()

	// propose_restock is deliberately shared between operations and
	// guest (see tools.go) -- both a real staff actor and a real guest
	// actor should be able to propose it.
	for _, role := range []string{"staff", "guest"} {
		ctx := superhost.PolicyContext{
			RunID:      "run-shared-" + role,
			TenantID:   "tenant-A",
			PropertyID: "prop-A",
			ActorID:    "actor-" + role,
			ActorRoles: []string{role},
		}
		input := superhost.ToolCallInput{
			ToolName:  "propose_restock",
			Version:   "v1",
			CallID:    "call-shared-" + role,
			Arguments: json.RawMessage(`{}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyApprovalRequired {
			t.Errorf("propose_restock should require approval (not be denied) for role %s, got %s: %s", role, decision.Result, decision.Reason)
		}
	}
}

func TestPolicyEngineUnresolvedRoleFailsClosed(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-noroles-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-noroles-001",
		ActorRoles: nil,
	}

	input := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-noroles-001",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyDenied {
		t.Errorf("a role-scoped tool should deny when no role resolved, got %s: %s", decision.Result, decision.Reason)
	}
}

func TestPolicyEngineUnknownToolDenied(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	input := superhost.ToolCallInput{
		ToolName:  "dangerous_mutation_tool",
		Version:   "v1",
		CallID:    "call-003",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyDenied {
		t.Errorf("unknown tool should be denied, got %s: %s", decision.Result, decision.Reason)
	}
}

func TestPolicyEngineEmptyToolNameDenied(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	input := superhost.ToolCallInput{
		ToolName:  "",
		Version:   "v1",
		CallID:    "call-004",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyDenied {
		t.Errorf("empty tool name should be denied, got %s", decision.Result)
	}
}

func TestPolicyEngineWrongVersionDenied(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	input := superhost.ToolCallInput{
		ToolName:  "get_property_operating_summary",
		Version:   "v999",
		CallID:    "call-005",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyDenied {
		t.Errorf("wrong version should be denied, got %s", decision.Result)
	}
}

func TestPolicyEngineCrossTenantDenied(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	input := superhost.ToolCallInput{
		ToolName:  "get_reservation_change",
		Version:   "v1",
		CallID:    "call-006",
		Arguments: json.RawMessage(`{"tenant_id": "tenant-B"}`),
	}

	decision := pe.Evaluate(ctx, input)
	if decision.Result != superhost.PolicyDenied {
		t.Errorf("cross-tenant call should be denied, got %s", decision.Result)
	}
}

func TestPolicyEngineUncertaintyCreatesException(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	input := superhost.ToolCallInput{
		ToolName:  "get_reservation_change",
		Version:   "v1",
		CallID:    "call-007",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.EvaluateUncertainty(ctx, input, "provider timeout exceeded")
	if decision.Result != superhost.PolicyUncertainty {
		t.Errorf("uncertainty should be marked, got %s", decision.Result)
	}
	if decision.Reason != "provider timeout exceeded" {
		t.Errorf("reason should be preserved, got %s", decision.Reason)
	}
}

func TestPolicyEngineTimeoutCreatesException(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	input := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "call-008",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.EvaluateException(ctx, input, "provider call timed out after 30s")
	if decision.Result != superhost.PolicyException {
		t.Errorf("timeout should create exception, got %s", decision.Result)
	}
	if decision.PolicyVersion != superhost.PolicyVersion {
		t.Errorf("policy version should be traceable, got %s", decision.PolicyVersion)
	}
}

func TestPolicyDecisionHasTraceableVersions(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	input := superhost.ToolCallInput{
		ToolName:  "get_property_operating_summary",
		Version:   "v1",
		CallID:    "call-vtrace",
		Arguments: json.RawMessage(`{}`),
	}

	decision := pe.Evaluate(ctx, input)

	if decision.ToolVersion != "v1" {
		t.Errorf("tool version not set: %s", decision.ToolVersion)
	}
	if decision.PolicyVersion == "" {
		t.Error("policy version should be traceable")
	}
	if decision.DecisionID == "" {
		t.Error("decision ID should be set")
	}
	if decision.RunID == "" {
		t.Error("run ID should be set")
	}
	if decision.ActorID == "" {
		t.Error("actor ID should be set")
	}
	if decision.TenantID == "" {
		t.Error("tenant ID should be set")
	}
	if decision.PropertyID == "" {
		t.Error("property ID should be set")
	}
}

func TestPolicyEngineEvaluatesBatch(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	batch := superhost.ToolCallBatchInput{
		Calls: []superhost.ToolCallInput{
			{
				ToolName:  "get_property_operating_summary",
				Version:   "v1",
				CallID:    "c1",
				Arguments: json.RawMessage(`{}`),
			},
			{
				ToolName:  "propose_turnover_ticket",
				Version:   "v1",
				CallID:    "c2",
				Arguments: json.RawMessage(`{}`),
			},
		},
	}

	decisions, err := pe.EvaluateBatch(ctx, batch)
	if err != nil {
		t.Fatalf("evaluate batch: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].Result != superhost.PolicyAllowed {
		t.Errorf("first should be allowed, got %s", decisions[0].Result)
	}
	if decisions[1].Result != superhost.PolicyApprovalRequired {
		t.Errorf("second should need approval, got %s", decisions[1].Result)
	}
}

func TestPolicyEngineEmptyBatchFails(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID: "run-001",
	}

	_, err := pe.EvaluateBatch(ctx, superhost.ToolCallBatchInput{Calls: nil})
	if err == nil {
		t.Error("empty batch should fail")
	}
}

func TestPolicyEngineDuplicateCallIDFails(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	batch := superhost.ToolCallBatchInput{
		Calls: []superhost.ToolCallInput{
			{
				ToolName:  "get_property_operating_summary",
				Version:   "v1",
				CallID:    "dup",
				Arguments: json.RawMessage(`{}`),
			},
			{
				ToolName:  "get_reservation_change",
				Version:   "v1",
				CallID:    "dup",
				Arguments: json.RawMessage(`{}`),
			},
		},
	}

	_, err := pe.EvaluateBatch(ctx, batch)
	if err == nil {
		t.Error("duplicate call_id in batch should fail")
	}
}

func TestPolicyEngineMissingCallIDFails(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
	}

	batch := superhost.ToolCallBatchInput{
		Calls: []superhost.ToolCallInput{
			{
				ToolName:  "get_property_operating_summary",
				Version:   "v1",
				CallID:    "",
				Arguments: json.RawMessage(`{}`),
			},
		},
	}

	_, err := pe.EvaluateBatch(ctx, batch)
	if err == nil {
		t.Error("missing call_id should fail")
	}
}

func TestApprovalStateMachineValidTransitions(t *testing.T) {
	ar := superhost.NewApprovalRequest(
		"req-001", "run-001", "pd-001",
		"propose_turnover_ticket", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	if ar.State != superhost.ApprovalStatePending {
		t.Fatalf("new request should be pending, got %s", ar.State)
	}

	err := ar.Decide("approver-001", "operations_supervisor", superhost.ApprovalStateApproved,
		"evidence-001", "approved after review")
	if err != nil {
		t.Fatalf("valid approval should work: %v", err)
	}
	if ar.State != superhost.ApprovalStateApproved {
		t.Errorf("should be approved, got %s", ar.State)
	}
	if ar.ActorID != "approver-001" {
		t.Errorf("actor not recorded: %s", ar.ActorID)
	}
	if ar.Evidence != "evidence-001" {
		t.Errorf("evidence not recorded: %s", ar.Evidence)
	}
}

func TestApprovalCannotSelfApprove(t *testing.T) {
	ar := superhost.NewApprovalRequest(
		"req-002", "run-001", "pd-002",
		"propose_turnover_ticket", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	err := ar.Decide("requester-001", "jarvis", superhost.ApprovalStateApproved,
		"", "self approved")
	if err == nil {
		t.Error("self-approval should be rejected (maker-checker)")
	}
}

func TestApprovalCanBeRejected(t *testing.T) {
	ar := superhost.NewApprovalRequest(
		"req-003", "run-001", "pd-003",
		"propose_restock", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	err := ar.Decide("approver-002", "operations_supervisor", superhost.ApprovalStateRejected,
		"", "insufficient justification")
	if err != nil {
		t.Fatalf("rejection should work: %v", err)
	}
	if ar.State != superhost.ApprovalStateRejected {
		t.Errorf("should be rejected, got %s", ar.State)
	}
}

func TestApprovalCannotDecideTwice(t *testing.T) {
	ar := superhost.NewApprovalRequest(
		"req-004", "run-001", "pd-004",
		"propose_inspection_ticket", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	err := ar.Decide("approver-001", "supervisor", superhost.ApprovalStateApproved, "", "ok")
	if err != nil {
		t.Fatalf("first decide: %v", err)
	}

	err = ar.Decide("approver-002", "supervisor", superhost.ApprovalStateRejected, "", "changed mind")
	if err == nil {
		t.Error("should not be able to decide twice")
	}
}

func TestApprovalRecordsPolicyVersion(t *testing.T) {
	ar := superhost.NewApprovalRequest(
		"req-005", "run-001", "pd-005",
		"propose_maintenance_request", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	if ar.PolicyVersion != superhost.PolicyVersion {
		t.Errorf("policy version not recorded: %s", ar.PolicyVersion)
	}
	if ar.ToolVersion != "v1" {
		t.Errorf("tool version not recorded: %s", ar.ToolVersion)
	}
}

func TestToolCallStateMachineValidTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		valid    bool
	}{
		{superhost.ToolCallStateProposed, superhost.ToolCallStatePolicyChecking, true},
		{superhost.ToolCallStateProposed, superhost.ToolCallStateCancelled, true},
		{superhost.ToolCallStateProposed, superhost.ToolCallStateSucceeded, false},
		{superhost.ToolCallStatePolicyChecking, superhost.ToolCallStateApprovalReq, true},
		{superhost.ToolCallStatePolicyChecking, superhost.ToolCallStateExecuting, true},
		{superhost.ToolCallStatePolicyChecking, superhost.ToolCallStateDenied, true},
		{superhost.ToolCallStatePolicyChecking, superhost.ToolCallStateFailed, true},
		{superhost.ToolCallStatePolicyChecking, superhost.ToolCallStateProposed, false},
		{superhost.ToolCallStateApprovalReq, superhost.ToolCallStateExecuting, true},
		{superhost.ToolCallStateApprovalReq, superhost.ToolCallStateDenied, true},
		{superhost.ToolCallStateApprovalReq, superhost.ToolCallStateCancelled, true},
		{superhost.ToolCallStateExecuting, superhost.ToolCallStateSucceeded, true},
		{superhost.ToolCallStateExecuting, superhost.ToolCallStateRetryable, true},
		{superhost.ToolCallStateExecuting, superhost.ToolCallStateFailed, true},
		{superhost.ToolCallStateExecuting, superhost.ToolCallStateCancelled, true},
		{superhost.ToolCallStateRetryable, superhost.ToolCallStatePolicyChecking, true},
		{superhost.ToolCallStateRetryable, superhost.ToolCallStateFailed, true},
		{superhost.ToolCallStateRetryable, superhost.ToolCallStateCancelled, true},
		{superhost.ToolCallStateSucceeded, superhost.ToolCallStateProposed, false},
		{superhost.ToolCallStateDenied, superhost.ToolCallStateProposed, false},
		{superhost.ToolCallStateFailed, superhost.ToolCallStateProposed, false},
		{superhost.ToolCallStateCancelled, superhost.ToolCallStateProposed, false},
	}

	for _, tc := range tests {
		err := superhost.ValidateToolCallTransition(tc.from, tc.to)
		valid := err == nil
		if valid != tc.valid {
			if tc.valid {
				t.Errorf("transition %s -> %s should be valid: %v", tc.from, tc.to, err)
			} else {
				t.Errorf("transition %s -> %s should be invalid", tc.from, tc.to)
			}
		}
	}
}

func TestToolCallTerminalStates(t *testing.T) {
	terminals := []string{
		superhost.ToolCallStateSucceeded,
		superhost.ToolCallStateDenied,
		superhost.ToolCallStateFailed,
		superhost.ToolCallStateCancelled,
	}

	for _, state := range terminals {
		if !superhost.IsToolCallTerminal(state) {
			t.Errorf("%s should be terminal", state)
		}
	}

	nonTerminals := []string{
		superhost.ToolCallStateProposed,
		superhost.ToolCallStatePolicyChecking,
		superhost.ToolCallStateApprovalReq,
		superhost.ToolCallStateExecuting,
		superhost.ToolCallStateRetryable,
	}

	for _, state := range nonTerminals {
		if superhost.IsToolCallTerminal(state) {
			t.Errorf("%s should not be terminal", state)
		}
	}
}

func TestModelTextAloneCannotChangeState(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	mutateInput := superhost.ToolCallInput{
		ToolName:  "update_property_status",
		Version:   "v1",
		CallID:    "model-mutate",
		Arguments: json.RawMessage(`{"status": "active"}`),
	}

	dec := pe.Evaluate(ctx, mutateInput)
	if dec.Result != superhost.PolicyDenied {
		t.Errorf("model output proposing unlisted tool must be denied, got %s", dec.Result)
	}

	deleteInput := superhost.ToolCallInput{
		ToolName:  "delete_reservation",
		Version:   "v1",
		CallID:    "model-delete",
		Arguments: json.RawMessage(`{"reservation_id": "r-001"}`),
	}

	dec2 := pe.Evaluate(ctx, deleteInput)
	if dec2.Result != superhost.PolicyDenied {
		t.Errorf("model output proposing direct deletion must be denied, got %s", dec2.Result)
	}
}

func TestSuperhostProhibitedAuthorityBlocklist(t *testing.T) {
	prohibited := []string{
		"delete_reservation",
		"hard_delete_record",
		"purge_audit_log",
		"wipe_tenant_data",
		"erase_evidence",
		"pay_vendor_invoice",
		"refund_guest_charge",
		"transfer_to_bank",
		"sign_legal_contract",
		"certify_inspection_document",
		"file_legal_report",
		"disclose_access_code",
		"read_secret_key",
		"export_secret_credentials",
		"terminate_worker_contract",
		"suspend_worker_account",
		"reject_worker_application",
		"create_order_without_approval",
		"approve_order_self",
		"mutate_property_direct",
		"write_calendar_direct",
		"update_reservation_status",
		"insert_stock_record",
		"upsert_service_agreement",
		"set_property_config_direct",
		"put_guest_access",
		"patch_financial_record",
	}

	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	for _, name := range prohibited {
		input := superhost.ToolCallInput{
			ToolName:  name,
			Version:   "v1",
			CallID:    "prohibited-" + name,
			Arguments: json.RawMessage(`{}`),
		}
		dec := pe.Evaluate(ctx, input)
		if dec.Result != superhost.PolicyDenied {
			t.Errorf("prohibited tool %q should be denied, got %s", name, dec.Result)
		}
	}
}

func TestApprovalExpiryAndCancellation(t *testing.T) {
	ar := superhost.NewApprovalRequest(
		"req-expire", "run-001", "pd-001",
		"propose_restock", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	err := ar.Decide("approver-001", "supervisor", superhost.ApprovalStateExpired, "", "deadline passed")
	if err != nil {
		t.Fatalf("expiry should work: %v", err)
	}
	if ar.State != superhost.ApprovalStateExpired {
		t.Errorf("should be expired, got %s", ar.State)
	}
	if !ar.IsTerminal() {
		t.Error("expired approval should be terminal")
	}

	ar2 := superhost.NewApprovalRequest(
		"req-cancel", "run-001", "pd-002",
		"propose_inspection_ticket", "v1",
		"operations",
		"requester-001", "tenant-A", "prop-A",
		[]string{"jarvis"},
		nil,
	)

	err = ar2.Decide("approver-001", "supervisor", superhost.ApprovalStateCancelled, "", "no longer needed")
	if err != nil {
		t.Fatalf("cancellation should work: %v", err)
	}
	if ar2.State != superhost.ApprovalStateCancelled {
		t.Errorf("should be cancelled, got %s", ar2.State)
	}
	if !ar2.IsTerminal() {
		t.Error("cancelled approval should be terminal")
	}
}

func TestToolDefinitionsHaveCorrectApprovalSettings(t *testing.T) {
	readTools := map[string]bool{
		"get_property_operating_summary": true,
		"get_reservation_change":         true,
		"summarize_incident":             true,
	}

	approvalTools := map[string]bool{
		"propose_turnover_ticket":     true,
		"propose_inspection_ticket":   true,
		"propose_restock":             true,
		"propose_maintenance_request": true,
		"request_owner_approval":      true,
		"request_operations_approval": true,
		"send_approved_notification":  true,
		"assemble_document_packet":    true,
		"escalate_exception":          true,
	}

	for name := range readTools {
		def, err := superhost.LookupTool(name)
		if err != nil {
			t.Errorf("read tool %s not found: %v", name, err)
			continue
		}
		if def.RequiresApproval {
			t.Errorf("read tool %s should not require approval", name)
		}
	}

	for name := range approvalTools {
		def, err := superhost.LookupTool(name)
		if err != nil {
			t.Errorf("approval tool %s not found: %v", name, err)
			continue
		}
		if !def.RequiresApproval {
			t.Errorf("approval tool %s should require approval", name)
		}
	}
}

func TestToolDefinitionsAreIdempotent(t *testing.T) {
	// log_task is a genuine, intentional exception: it appends a new
	// entry to the account's task ledger every time, by design (each
	// call is a distinct note), so retrying it is not a no-op the way
	// every other tool's retry is.
	nonIdempotentExceptions := map[string]bool{"log_task": true}
	for _, name := range superhost.AllowedToolNames() {
		def, _ := superhost.LookupTool(name)
		if def.Kind == superhost.ToolKindUIAction || nonIdempotentExceptions[name] {
			continue
		}
		if !def.Idempotent {
			t.Errorf("tool %s should be idempotent", name)
		}
	}
}

func TestCCHOU001PropertyScopeCannotCross(t *testing.T) {
	pe := superhost.NewPolicyEngine()

	ctx := superhost.PolicyContext{
		RunID:      "run-hou-scope",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	inputCross := superhost.ToolCallInput{
		ToolName:  "get_property_operating_summary",
		Version:   "v1",
		CallID:    "cross-tenant",
		Arguments: json.RawMessage(`{"tenant_id": "tenant-B", "property_id": "prop-B"}`),
	}

	dec := pe.Evaluate(ctx, inputCross)
	if dec.Result != superhost.PolicyDenied {
		t.Errorf("cross-tenant tool call must be denied, got %s", dec.Result)
	}

	inputCrossProp := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "cross-prop",
		Arguments: json.RawMessage(`{"property_id": "prop-B"}`),
	}

	dec2 := pe.Evaluate(ctx, inputCrossProp)
	if dec2.Result != superhost.PolicyDenied {
		t.Errorf("cross-property tool call must be denied, got %s", dec2.Result)
	}

	err := inputCross.ValidateScope("tenant-A", "prop-A")
	if err == nil {
		t.Error("cross-tenant validation must fail")
	}

	err = inputCrossProp.ValidateScope("tenant-A", "prop-A")
	if err == nil {
		t.Error("cross-property validation must fail")
	}

	inputValid := superhost.ToolCallInput{
		ToolName:  "get_property_operating_summary",
		Version:   "v1",
		CallID:    "valid-scope",
		Arguments: json.RawMessage(`{}`),
	}

	err = inputValid.ValidateScope("tenant-A", "prop-A")
	if err != nil {
		t.Errorf("same-scope call must pass: %v", err)
	}

	dec3 := pe.Evaluate(ctx, inputValid)
	if dec3.Result != superhost.PolicyAllowed {
		t.Errorf("same-scope read must be allowed, got %s", dec3.Result)
	}
}

func TestCCHOU001OnlyTypedToolsAreExposed(t *testing.T) {
	allowed := superhost.AllowedToolNames()

	if len(allowed) == 0 {
		t.Fatal("no tools in catalog")
	}

	for _, name := range allowed {
		def, err := superhost.LookupTool(name)
		if err != nil {
			t.Errorf("tool %q not found: %v", name, err)
			continue
		}

		switch def.Kind {
		case superhost.ToolKindRead, superhost.ToolKindPropose, superhost.ToolKindRequest, superhost.ToolKindUIAction:
		default:
			t.Errorf("tool %q has unauthorized kind %q", name, def.Kind)
		}

		if def.SchemaVersion != superhost.ToolSchemaVersionCurrent {
			t.Errorf("tool %q has wrong schema version: %s", name, def.SchemaVersion)
		}

		if def.Name == "" {
			t.Error("tool name must not be empty")
		}
	}

	prohibitedPrefixes := []string{
		"delete_", "hard_delete_", "purge_", "wipe_", "erase_",
		"pay_", "refund_", "charge_", "transfer_", "disburse_",
		"sign_", "certify_", "file_legal_",
		"disclose_access_", "read_secret_", "export_secret_",
		"terminate_worker_", "suspend_worker_", "reject_worker_",
		"create_order_", "approve_order_", "place_order_",
		"mutate_", "write_", "update_", "insert_", "upsert_",
		"set_", "put_", "patch_",
	}

	for _, name := range allowed {
		for _, prefix := range prohibitedPrefixes {
			if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
				t.Errorf("tool %q starts with prohibited prefix %q", name, prefix)
			}
		}
	}

	namesSet := make(map[string]bool)
	for _, name := range allowed {
		if namesSet[name] {
			t.Errorf("duplicate tool name in catalog: %s", name)
		}
		namesSet[name] = true
	}
}

func TestCCHOU001PolicyRejectsDirectMutation(t *testing.T) {
	pe := superhost.NewPolicyEngine()

	ctx := superhost.PolicyContext{
		RunID:      "run-hou-mutation",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	mutationAndDangerous := []string{
		"delete_reservation",
		"update_ticket_status",
		"write_inventory",
		"mutate_property",
		"pay_invoice",
		"sign_contract",
		"certify_document",
		"terminate_worker",
		"approve_order",
		"hard_delete_record",
		"purge_audit_log",
		"wipe_tenant_data",
		"erase_evidence",
		"disclose_access_code",
		"set_property_config",
		"put_guest_access",
		"patch_financial_record",
		"create_order",
		"read_secret_key",
		"export_secret_credentials",
	}

	for _, name := range mutationAndDangerous {
		input := superhost.ToolCallInput{
			ToolName:  name,
			Version:   "v1",
			CallID:    "mut-" + name,
			Arguments: json.RawMessage(`{}`),
		}

		dec := pe.Evaluate(ctx, input)
		if dec.Result != superhost.PolicyDenied {
			t.Errorf("mutation tool %q must be denied by policy, got %s", name, dec.Result)
		}
	}

	textOnly := superhost.ToolCallInput{
		ToolName:  "",
		Version:   "v1",
		CallID:    "empty-name",
		Arguments: json.RawMessage(`{"message": "model said to update"}`),
	}

	dec := pe.Evaluate(ctx, textOnly)
	if dec.Result != superhost.PolicyDenied {
		t.Errorf("empty tool name (model text alone) must be denied, got %s", dec.Result)
	}

	unknownTool := superhost.ToolCallInput{
		ToolName:  "fabricate_evidence",
		Version:   "v1",
		CallID:    "unknown-tool",
		Arguments: json.RawMessage(`{}`),
	}

	dec2 := pe.Evaluate(ctx, unknownTool)
	if dec2.Result != superhost.PolicyDenied {
		t.Errorf("unknown tool must be denied, got %s", dec2.Result)
	}
}

func TestCCHOU001ModelOutageHasManualFallback(t *testing.T) {
	pe := superhost.NewPolicyEngine()

	ctx := superhost.PolicyContext{
		RunID:      "run-hou-outage",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	readInput := superhost.ToolCallInput{
		ToolName:  "get_property_operating_summary",
		Version:   "v1",
		CallID:    "outage-read",
		Arguments: json.RawMessage(`{}`),
	}

	uncertaintyDec := pe.EvaluateUncertainty(ctx, readInput, "provider unavailable: all models down")
	if uncertaintyDec.Result != superhost.PolicyUncertainty {
		t.Errorf("model outage must result in uncertainty, got %s", uncertaintyDec.Result)
	}
	if uncertaintyDec.Reason != "provider unavailable: all models down" {
		t.Errorf("outage reason must be preserved, got %s", uncertaintyDec.Reason)
	}
	if uncertaintyDec.PolicyVersion != superhost.PolicyVersion {
		t.Error("policy version must be traceable during outage")
	}

	timeoutInput := superhost.ToolCallInput{
		ToolName:  "propose_turnover_ticket",
		Version:   "v1",
		CallID:    "outage-timeout",
		Arguments: json.RawMessage(`{}`),
	}

	exceptionDec := pe.EvaluateException(ctx, timeoutInput, "provider call timed out after 30s")
	if exceptionDec.Result != superhost.PolicyException {
		t.Errorf("timeout must create exception, got %s", exceptionDec.Result)
	}
	if exceptionDec.ToolVersion != "v1" {
		t.Errorf("tool version must be traceable in exception, got %s", exceptionDec.ToolVersion)
	}
	if exceptionDec.PolicyVersion != superhost.PolicyVersion {
		t.Error("policy version must be traceable in exception")
	}

	allowed := superhost.AllowedToolNames()
	if len(allowed) == 0 {
		t.Fatal("tool catalog must remain available during model outage")
	}

	for _, name := range allowed {
		def, err := superhost.LookupTool(name)
		if err != nil {
			t.Errorf("tool %q must be available during outage: %v", name, err)
		}
		if def.SchemaVersion == "" {
			t.Errorf("tool %q schema version must be traceable during outage", name)
		}
	}

	pe2 := superhost.NewPolicyEngine()
	validRead := superhost.ToolCallInput{
		ToolName:  "get_reservation_change",
		Version:   "v1",
		CallID:    "outage-valid-read",
		Arguments: json.RawMessage(`{}`),
	}

	validDec := pe2.Evaluate(ctx, validRead)
	if validDec.Result != superhost.PolicyAllowed {
		t.Errorf("core read tool must be evaluable during outage, got %s", validDec.Result)
	}
}

func TestPolicyEngineUIActionToolAllowed(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-ui-001",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	t.Run("ui_click allowed", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_click",
			Version:   "v1",
			CallID:    "call-ui-click",
			Arguments: json.RawMessage(`{"surface_id": "btn-submit"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyAllowed {
			t.Errorf("ui_click should be allowed, got %s: %s", decision.Result, decision.Reason)
		}
		if decision.InputClass != string(superhost.ToolKindUIAction) {
			t.Errorf("input class should be ui_action, got %s", decision.InputClass)
		}
	})

	t.Run("ui_set_value allowed", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_set_value",
			Version:   "v1",
			CallID:    "call-ui-set",
			Arguments: json.RawMessage(`{"surface_id": "input-name", "value": "hello"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyAllowed {
			t.Errorf("ui_set_value should be allowed, got %s: %s", decision.Result, decision.Reason)
		}
	})

	t.Run("ui_focus allowed without tenant/property scope enforcement", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_focus",
			Version:   "v1",
			CallID:    "call-ui-focus",
			Arguments: json.RawMessage(`{"surface_id": "nav-help"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyAllowed {
			t.Errorf("ui_focus should be allowed without scope args, got %s: %s", decision.Result, decision.Reason)
		}
	})

	t.Run("ui_scroll_to allowed", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_scroll_to",
			Version:   "v1",
			CallID:    "call-ui-scroll",
			Arguments: json.RawMessage(`{"surface_id": "section-footer"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyAllowed {
			t.Errorf("ui_scroll_to should be allowed, got %s: %s", decision.Result, decision.Reason)
		}
	})

	t.Run("ui_open_panel allowed", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_open_panel",
			Version:   "v1",
			CallID:    "call-ui-panel",
			Arguments: json.RawMessage(`{"surface_id": "panel-property-details"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyAllowed {
			t.Errorf("ui_open_panel should be allowed, got %s: %s", decision.Result, decision.Reason)
		}
	})
}

func TestPolicyEngineUnregisteredUIPrefixToolDenied(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-ui-deny",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	t.Run("ui_delete denied by LookupTool", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_delete",
			Version:   "v1",
			CallID:    "call-ui-delete",
			Arguments: json.RawMessage(`{"surface_id": "row-3"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyDenied {
			t.Errorf("ui_delete (unregistered) should be denied, got %s: %s", decision.Result, decision.Reason)
		}
	})

	t.Run("ui_custom_action denied by LookupTool", func(t *testing.T) {
		input := superhost.ToolCallInput{
			ToolName:  "ui_custom_action",
			Version:   "v1",
			CallID:    "call-ui-custom",
			Arguments: json.RawMessage(`{"surface_id": "x"}`),
		}
		decision := pe.Evaluate(ctx, input)
		if decision.Result != superhost.PolicyDenied {
			t.Errorf("ui_custom_action (unregistered) should be denied, got %s: %s", decision.Result, decision.Reason)
		}
	})
}

func TestPolicyEngineUIActionNotDirectMutation(t *testing.T) {
	pe := superhost.NewPolicyEngine()
	ctx := superhost.PolicyContext{
		RunID:      "run-ui-dm",
		TenantID:   "tenant-A",
		PropertyID: "prop-A",
		ActorID:    "actor-001",
		ActorRoles: []string{"jarvis"},
	}

	input := superhost.ToolCallInput{
		ToolName:  "ui_click",
		Version:   "v1",
		CallID:    "call-dm-check",
		Arguments: json.RawMessage(`{"surface_id": "btn-submit"}`),
	}

	decision := pe.Evaluate(ctx, input)

	if decision.Result == superhost.PolicyDenied {
		t.Errorf("ui_click should NOT be denied as direct mutation, got reason: %s", decision.Reason)
	}
	if decision.Result != superhost.PolicyAllowed {
		t.Errorf("ui_click should be PolicyAllowed, got %s: %s", decision.Result, decision.Reason)
	}
}

func TestSuperhostUIActionToolKinds(t *testing.T) {
	def, _ := superhost.LookupTool("ui_click")
	if def.Kind != superhost.ToolKindUIAction {
		t.Errorf("ui_click kind should be ui_action, got %s", def.Kind)
	}
	if len(def.Audiences) != 1 || def.Audiences[0] != superhost.ToolAudienceUI {
		t.Errorf("ui_click audience should be [ui], got %v", def.Audiences)
	}
	if def.RequiresApproval != false {
		t.Errorf("ui_click should not require policy approval, got %v", def.RequiresApproval)
	}
	if def.Idempotent != false {
		t.Errorf("ui_click should not be idempotent, got %v", def.Idempotent)
	}

	def, _ = superhost.LookupTool("ui_set_value")
	if def.Kind != superhost.ToolKindUIAction {
		t.Errorf("ui_set_value kind should be ui_action, got %s", def.Kind)
	}
}

func TestSuperhostUIActionIsNotAMutation(t *testing.T) {
	def, err := superhost.LookupTool("ui_click")
	if err != nil {
		t.Fatalf("ui_click should be in registry: %v", err)
	}
	if def.IsMutation() {
		t.Error("ui_action tool should not be classified as a mutation")
	}
}

func TestSuperhostNoUIPrefixCollidesWithProhibitedPrefixes(t *testing.T) {
	uiTools := []string{"ui_focus", "ui_set_value", "ui_click", "ui_scroll_to", "ui_open_panel"}
	for _, name := range uiTools {
		if superhost.IsToolProhibited(name) {
			t.Errorf("ui_* tool %q should not be prohibited by any prefix", name)
		}
		def, err := superhost.LookupTool(name)
		if err != nil {
			t.Errorf("ui_* tool %q lookup failed: %v", name, err)
		}
		if def.Kind != superhost.ToolKindUIAction {
			t.Errorf("ui_* tool %q has wrong kind: %s", name, def.Kind)
		}
	}
}

func TestSuperhostUISurfaceInput(t *testing.T) {
	input := superhost.UISurfaceInput{
		ID:      "btn-submit-checkout",
		Label:   "Submit Checkout",
		Actions: []string{"ui_click", "ui_focus"},
	}

	if input.ID != "btn-submit-checkout" {
		t.Errorf("ID mismatch: %s", input.ID)
	}
	if input.Label != "Submit Checkout" {
		t.Errorf("Label mismatch: %s", input.Label)
	}
	if len(input.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(input.Actions))
	}
}
