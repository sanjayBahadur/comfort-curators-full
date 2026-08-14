package automation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/superhost"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/security"

	"github.com/jackc/pgx/v5/pgxpool"
)

func approveTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newPool(t)

	// EnsureToolSchema must run before truncating its tables -- on a fresh
	// database ai_tool_calls/policy_decisions/approval_requests don't exist
	// yet, and TRUNCATE on a nonexistent table fails outright (order was
	// reversed originally; every test using this helper failed immediately
	// with "relation ai_tool_calls does not exist").
	if err := superhost.EnsureToolSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure tool schema: %v", err)
	}

	tables := []string{"agent_run_events", "agent_runs", "ai_tool_calls", "policy_decisions", "approval_requests"}
	for _, table := range tables {
		if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}

	return pool
}

func setupApproveMux(t *testing.T, pool *pgxpool.Pool, runStore *automation.AgentRunStore) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()

	agentHandler := automation.NewAgentRunHandler(runStore)
	agentHandler.RegisterRoutes(mux)

	assembler := superhost.NewContextAssembler(pool)
	threadStore := superhost.NewThreadStore(pool, runStore, assembler)
	toolCallStore := superhost.NewToolCallStore(pool)
	superhostHandler := superhost.NewHandlerWithApprovals(runStore, assembler, threadStore, toolCallStore)
	superhostHandler.RegisterRoutes(mux)

	return mux
}

func doApproveRequest(t *testing.T, mux *http.ServeMux, method, path, body string, tenantID, actorID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(iam.WithSubject(req.Context(), security.Subject{
		TenantID: tenantID,
		ActorID:  actorID,
		Roles:    []string{"operations"},
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestApproveEndpointSelfApprovalDenied(t *testing.T) {
	pool := approveTestPool(t)
	store := automation.NewAgentRunStore(pool)
	mux := setupApproveMux(t, pool, store)

	ar := &superhost.ApprovalRequest{
		RequestID:   "ar-sa-test-001",
		RunID:       "run-sa-test-001",
		RequesterID: "actor-sa",
		TenantID:    "tenant-sa",
		State:       superhost.ApprovalStatePending,
	}
	toolCallStore := superhost.NewToolCallStore(pool)
	if err := toolCallStore.RecordApprovalRequest(context.Background(), *ar); err != nil {
		t.Fatalf("record approval request: %v", err)
	}

	body := `{"decision":"approved"}`
	rec := doApproveRequest(t, mux, "POST", "/v1/superhost/approvals/ar-sa-test-001/decide", body, "tenant-sa", "actor-sa")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for self-approval, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestApproveEndpointRejectionFailsRun(t *testing.T) {
	pool := approveTestPool(t)
	store := automation.NewAgentRunStore(pool)

	ctx := context.Background()
	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tenant-reject",
		PropertyID:     "prop-reject",
		ActorID:        "actor-reject",
		TriggerType:    "manual",
		TriggerID:      "t-reject",
		CorrelationID:  "c-reject",
		IdempotencyKey: "key-reject-" + time.Now().Format("20060102150405.000000000"),
		Provider:       "stub",
		Model:          "test-model",
		InputData:      json.RawMessage(`"test"`),
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	runStore := automation.NewAgentRunStore(pool)
	_, err = runStore.Pool().Exec(ctx, `UPDATE agent_runs SET state = $1, lease_owner = $2 WHERE run_id = $3`,
		automation.StateWaitingForApproval, "worker-test", run.RunID)
	if err != nil {
		t.Fatalf("set waiting_for_approval: %v", err)
	}

	ar := &superhost.ApprovalRequest{
		RequestID:   "ar-reject-001",
		RunID:       run.RunID,
		RequesterID: "actor-reject",
		TenantID:    "tenant-reject",
		State:       superhost.ApprovalStatePending,
	}
	toolCallStore := superhost.NewToolCallStore(pool)
	if err := toolCallStore.RecordApprovalRequest(context.Background(), *ar); err != nil {
		t.Fatalf("record approval request: %v", err)
	}

	mux := setupApproveMux(t, pool, runStore)

	body := `{"decision":"rejected"}`
	rec := doApproveRequest(t, mux, "POST", "/v1/superhost/approvals/ar-reject-001/decide", body, "tenant-reject", "approver-reject")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for rejection, got %d: %s", rec.Code, rec.Body.String())
	}

	retrieved, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if retrieved.State != automation.StateFailed {
		t.Fatalf("expected failed after rejection, got %s", retrieved.State)
	}
	if retrieved.ErrorMessage == "" {
		t.Fatal("expected error message about human rejection")
	}
	t.Logf("rejected run %s: %s", retrieved.RunID, retrieved.ErrorMessage)
}

func TestApproveEndpointApprovalRequeuesRun(t *testing.T) {
	pool := approveTestPool(t)
	store := automation.NewAgentRunStore(pool)
	mux := setupApproveMux(t, pool, store)

	ctx := context.Background()
	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tenant-a",
		PropertyID:     "prop-a",
		ActorID:        "actor-a",
		TriggerType:    "manual",
		TriggerID:      "t-a",
		CorrelationID:  "c-a",
		IdempotencyKey: "key-approve-" + time.Now().Format("20060102150405.000000000"),
		Provider:       "stub",
		Model:          "test-model",
		InputData:      json.RawMessage(`"test"`),
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	runStore := automation.NewAgentRunStore(pool)
	_, err = runStore.Pool().Exec(ctx, `UPDATE agent_runs SET state = $1, lease_owner = $2 WHERE run_id = $3`,
		automation.StateWaitingForApproval, "worker-test", run.RunID)
	if err != nil {
		t.Fatalf("set waiting_for_approval: %v", err)
	}

	ar := &superhost.ApprovalRequest{
		RequestID:   "ar-approve-001",
		RunID:       run.RunID,
		RequesterID: "actor-a",
		TenantID:    "tenant-a",
		State:       superhost.ApprovalStatePending,
	}
	toolCallStore := superhost.NewToolCallStore(pool)
	if err := toolCallStore.RecordApprovalRequest(context.Background(), *ar); err != nil {
		t.Fatalf("record approval request: %v", err)
	}

	body := `{"decision":"approved"}`
	rec := doApproveRequest(t, mux, "POST", "/v1/superhost/approvals/ar-approve-001/decide", body, "tenant-a", "approver-a")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for approval, got %d: %s", rec.Code, rec.Body.String())
	}

	retrieved, err := store.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if retrieved.State != automation.StateQueued {
		t.Fatalf("expected queued after approval, got %s", retrieved.State)
	}
	if retrieved.LeaseOwner != "" {
		t.Fatalf("expected lease cleared after approval, got %s", retrieved.LeaseOwner)
	}
	t.Logf("approved run %s requeued with no lease", retrieved.RunID)
}

// confirmAfterPauseProvider returns "no tool calls" on its second call, so the
// resumed run completes cleanly after the approval tool result is fed back.
type confirmAfterPauseProvider struct {
	calls int
}

func (p *confirmAfterPauseProvider) Call(ctx context.Context, req automation.ProviderRequest) (*automation.ProviderResponse, error) {
	p.calls++
	if p.calls == 1 {
		toolCalls, _ := json.Marshal([]map[string]any{
			{
				"id":   "call_pause_1",
				"type": "function",
				"function": map[string]any{
					"name":      "propose_turnover_ticket",
					"arguments": json.RawMessage(`{}`),
				},
			},
		})
		return &automation.ProviderResponse{
			Output:     json.RawMessage(`"I need approval to propose a ticket"`),
			ToolCalls:  toolCalls,
			TokenUsage: automation.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			UsageCurr:  "USD",
		}, nil
	}
	return &automation.ProviderResponse{
		Output:     json.RawMessage(`"Task complete after approval"`),
		TokenUsage: automation.TokenUsage{InputTokens: 8, OutputTokens: 4, TotalTokens: 12},
		UsageCurr:  "USD",
	}, nil
}

type approvalPauseExecutor struct{}

func (e *approvalPauseExecutor) Evaluate(ctx context.Context, run *automation.AgentRun, toolCall json.RawMessage) (automation.ToolLoopOutcome, error) {
	return automation.ToolLoopOutcome{
		Type:              automation.ToolLoopApprovalRequired,
		ToolName:          "propose_turnover_ticket",
		Version:           "v1",
		ApprovalRequestID: "ar_pause_roundtrip",
		ApprovalSummary:   "I can raise a turnover ticket. I have not done it yet. It needs approval.",
	}, nil
}

func TestFullRoundTripPauseApproveResumeComplete(t *testing.T) {
	pool := approveTestPool(t)
	store := automation.NewAgentRunStore(pool)

	sp := &confirmAfterPauseProvider{}
	exp := &approvalPauseExecutor{}

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return sp
	}, "worker-rt", "test system prompt", exp)

	run := submitQueuedRun(t, store, "worker-rt")

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	retrieved := waitForState(t, store, run.RunID, automation.StateWaitingForApproval, automation.StateFailed, automation.StateCompleted)
	if retrieved.State != automation.StateWaitingForApproval {
		t.Fatalf("expected waiting_for_approval, got %s (error: %s)", retrieved.State, retrieved.ErrorMessage)
	}
	if sp.calls != 1 {
		t.Fatalf("expected exactly 1 provider call before pause, got %d", sp.calls)
	}

	if len(retrieved.MessagesJSON) == 0 {
		t.Fatal("expected messages_json to be persisted on pause")
	}
	t.Logf("run %s paused with %d bytes of messages_json", retrieved.RunID, len(retrieved.MessagesJSON))

	ar := &superhost.ApprovalRequest{
		RequestID:   "ar_pause_roundtrip",
		RunID:       run.RunID,
		RequesterID: "actor-rt",
		TenantID:    "tenant-test",
		State:       superhost.ApprovalStatePending,
	}
	toolCallStore := superhost.NewToolCallStore(pool)
	if err := toolCallStore.RecordApprovalRequest(context.Background(), *ar); err != nil {
		t.Fatalf("record approval request: %v", err)
	}

	mux := setupApproveMux(t, pool, store)
	body := `{"decision":"approved"}`
	rec := doApproveRequest(t, mux, "POST", "/v1/superhost/approvals/ar_pause_roundtrip/decide", body, "tenant-test", "approver-rt")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for approval, got %d: %s", rec.Code, rec.Body.String())
	}

	completed := waitForStateFor(t, store, run.RunID, 15*time.Second, automation.StateCompleted, automation.StateFailed)
	if completed.State != automation.StateCompleted {
		t.Fatalf("expected completed after approval+resume, got %s (error: %s)", completed.State, completed.ErrorMessage)
	}

	if sp.calls < 2 {
		t.Fatalf("expected at least 2 provider calls (pause + resume), got %d", sp.calls)
	}

	t.Logf("run %s completed full round trip: pause -> approve -> resume -> complete with %d provider calls", run.RunID, sp.calls)
}

func TestApproveEndpointWrongTenant(t *testing.T) {
	pool := approveTestPool(t)
	store := automation.NewAgentRunStore(pool)
	mux := setupApproveMux(t, pool, store)

	ar := &superhost.ApprovalRequest{
		RequestID:   "ar-wt-test",
		RunID:       "run-wt-test",
		RequesterID: "requester-wt",
		TenantID:    "tenant-correct",
		State:       superhost.ApprovalStatePending,
	}
	toolCallStore := superhost.NewToolCallStore(pool)
	if err := toolCallStore.RecordApprovalRequest(context.Background(), *ar); err != nil {
		t.Fatalf("record approval request: %v", err)
	}

	body := `{"decision":"approved"}`
	rec := doApproveRequest(t, mux, "POST", "/v1/superhost/approvals/ar-wt-test/decide", body, "tenant-wrong", "actor-wrong")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong tenant, got %d: %s", rec.Code, rec.Body.String())
	}
}
