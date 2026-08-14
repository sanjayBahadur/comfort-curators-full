package automation_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
)

type stubProvider struct {
	calls        int
	output       json.RawMessage
	toolCalls    json.RawMessage
	alwaysReturn *automation.ProviderResponse
}

type hangingProvider struct{}

func (p *hangingProvider) Call(ctx context.Context, req automation.ProviderRequest) (*automation.ProviderResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type errorProvider struct{}

func (p *errorProvider) Call(ctx context.Context, req automation.ProviderRequest) (*automation.ProviderResponse, error) {
	return nil, errors.New("model unavailable")
}

func (p *stubProvider) Call(ctx context.Context, req automation.ProviderRequest) (*automation.ProviderResponse, error) {
	p.calls++
	if p.alwaysReturn != nil {
		return p.alwaysReturn, nil
	}
	return &automation.ProviderResponse{
		Output:    p.output,
		Provider:  req.Provider,
		Model:     req.Model,
		ToolCalls: p.toolCalls,
		TokenUsage: automation.TokenUsage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
		UsageMinor: 0,
		UsageCurr:  "USD",
		UsageKnown: false,
	}, nil
}

type stubToolExecutor struct {
	outcome         automation.ToolLoopOutcome
	outcomeSequence []automation.ToolLoopOutcome
	seqIndex        int
}

func (e *stubToolExecutor) Evaluate(ctx context.Context, run *automation.AgentRun, toolCall json.RawMessage) (automation.ToolLoopOutcome, error) {
	if len(e.outcomeSequence) > 0 {
		i := e.seqIndex
		e.seqIndex++
		return e.outcomeSequence[i], nil
	}
	return e.outcome, nil
}

func alwaysToolCallsProvider() *stubProvider {
	toolCalls, _ := json.Marshal([]map[string]any{
		{
			"id":   "call_1",
			"type": "function",
			"function": map[string]any{
				"name":      "get_property_operating_summary",
				"arguments": json.RawMessage(`{}`),
			},
		},
	})
	return &stubProvider{
		output:    json.RawMessage(`"ok"`),
		toolCalls: toolCalls,
	}
}

func noToolCallsProvider() *stubProvider {
	return &stubProvider{
		output: json.RawMessage(`"final text response"`),
	}
}

// sequencedProvider returns a different canned response on each successive
// call, repeating its last response once the sequence is exhausted. Used to
// model a model that proposes a tool call and then, once it sees the tool
// result fed back on the next turn, returns a final text answer — as
// opposed to stubProvider/alwaysToolCallsProvider, which always proposes a
// tool call and never completes (that's the correct, intentional shape for
// exercising the 6-iteration hard cap, but the wrong shape for a test that
// wants to observe a normal multi-turn completion).
type sequencedProvider struct {
	calls     int
	responses []*automation.ProviderResponse
}

func (p *sequencedProvider) Call(ctx context.Context, req automation.ProviderRequest) (*automation.ProviderResponse, error) {
	idx := p.calls
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	p.calls++
	resp := *p.responses[idx]
	resp.Provider = req.Provider
	resp.Model = req.Model
	return &resp, nil
}

func toolCallThenCompleteProvider() *sequencedProvider {
	toolCalls, _ := json.Marshal([]map[string]any{
		{
			"id":   "call_1",
			"type": "function",
			"function": map[string]any{
				"name":      "get_property_operating_summary",
				"arguments": json.RawMessage(`{}`),
			},
		},
	})
	return &sequencedProvider{
		responses: []*automation.ProviderResponse{
			{
				Output:     json.RawMessage(`"ok"`),
				ToolCalls:  toolCalls,
				TokenUsage: automation.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
				UsageCurr:  "USD",
			},
			{
				Output:     json.RawMessage(`"final text response after tool"`),
				TokenUsage: automation.TokenUsage{InputTokens: 8, OutputTokens: 4, TotalTokens: 12},
				UsageCurr:  "USD",
			},
		},
	}
}

func allowedExecutor() *stubToolExecutor {
	return &stubToolExecutor{
		outcome: automation.ToolLoopOutcome{
			Type:          automation.ToolLoopAllowed,
			ToolName:      "get_property_operating_summary",
			Version:       "v1",
			ResultSummary: "Property summary: OK",
		},
	}
}

func approvalRequiredExecutor() *stubToolExecutor {
	return &stubToolExecutor{
		outcome: automation.ToolLoopOutcome{
			Type:              automation.ToolLoopApprovalRequired,
			ToolName:          "propose_turnover_ticket",
			Version:           "v1",
			ApprovalRequestID: "ar_test_123",
			ApprovalSummary:   "i can raise a turnover ticket. i have not done it yet. it needs your ok.",
		},
	}
}

// submitQueuedRun submits a run and leaves it in the queued state, matching
// what RunWorkLoop expects to find. An earlier version of this helper also
// claimed and manually transitioned the run to "running" before the runner
// goroutine ever started — but RunWorkLoop drives its own Claim() (which
// only matches queued runs) internally, so a run pre-claimed outside the
// runner is invisible to it: the runner just polls ErrRunNotFound forever
// and the pre-claimed run sits in "running" untouched. Submitting and
// leaving it queued is what lets the runner claim, run, and complete it
// itself, matching how production actually drives a run end to end.
func submitQueuedRun(t *testing.T, store *automation.AgentRunStore, seed string) *automation.AgentRun {
	return submitQueuedRunWithInput(t, store, seed, json.RawMessage(`"test input"`))
}

func submitQueuedRunWithInput(t *testing.T, store *automation.AgentRunStore, seed string, input json.RawMessage) *automation.AgentRun {
	t.Helper()
	ctx := context.Background()

	req := automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tenant-test",
		PropertyID:     "prop-test",
		ActorID:        "actor-test",
		TriggerType:    "manual",
		TriggerID:      "trigger-test",
		CorrelationID:  "corr-test",
		IdempotencyKey: "key-runner-test-" + seed + "-" + time.Now().Format("20060102150405.000000000"),
		Provider:       "stub",
		Model:          "test-model",
		InputData:      input,
	}
	run, _, err := store.Submit(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return run
}

// waitForState polls the store (rather than racing a fixed sleep against
// async work) until the run reaches one of the wanted terminal-for-this-test
// states, or fails the test after a generous timeout. A tool loop makes
// several sequential DB round-trips per iteration (record tool call, record
// policy decision, record event, possibly an approval insert, then a second
// model call) — a fixed short sleep before asserting state raced that work
// and produced false failures ("expected completed, got running") against a
// real Postgres even though the loop was still correctly in flight.
func waitForState(t *testing.T, store *automation.AgentRunStore, runID string, want ...string) *automation.AgentRun {
	return waitForStateFor(t, store, runID, 5*time.Second, want...)
}

func waitForStateFor(t *testing.T, store *automation.AgentRunStore, runID string, timeout time.Duration, want ...string) *automation.AgentRun {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *automation.AgentRun
	for time.Now().Before(deadline) {
		run, err := store.Get(context.Background(), runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		last = run
		for _, w := range want {
			if run.State == w {
				return run
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("timed out waiting for run %s to reach state in %v, last seen state %q (error_message=%q)",
			runID, want, last.State, last.ErrorMessage)
	}
	t.Fatalf("timed out waiting for run %s to reach state in %v", runID, want)
	return nil
}

func TestRunnerProviderTimeoutCompletesWithFallback(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRunWithInput(t, store, "worker-timeout", json.RawMessage(`{"intent":"property_status"}`))
	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return &hangingProvider{}
	}, "worker-timeout", "test system prompt", nil)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	retrieved := waitForStateFor(t, store, run.RunID, 25*time.Second, automation.StateCompleted, automation.StateFailed)
	if retrieved.State != automation.StateCompleted {
		t.Fatalf("expected completed fallback, got %s (error: %s)", retrieved.State, retrieved.ErrorMessage)
	}
	var output map[string]any
	if err := json.Unmarshal(retrieved.OutputData, &output); err != nil {
		t.Fatalf("unmarshal fallback output: %v", err)
	}
	if output["is_fallback"] != true || output["fallback_marker"] != "OFFLINE FALLBACK" {
		t.Fatalf("fallback output is not labeled: %v", output)
	}
	if retrieved.UsageKnown || retrieved.UsageMinorUnits != 0 || retrieved.UsageTotalTokens != 0 {
		t.Fatalf("fallback must not claim usage: known=%v minor=%d total=%d", retrieved.UsageKnown, retrieved.UsageMinorUnits, retrieved.UsageTotalTokens)
	}

	events, err := automation.ListEvents(context.Background(), store.Pool(), run.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.EventName == automation.EventRunFallback {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected AgentRunFallback.v1 event")
	}
}

func TestRunnerUnknownIntentUsesGenericFallback(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRunWithInput(t, store, "worker-generic", json.RawMessage(`{"intent":"unrecognized_intent"}`))
	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return &errorProvider{}
	}, "worker-generic", "test system prompt", nil)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	retrieved := waitForState(t, store, run.RunID, automation.StateCompleted, automation.StateFailed)
	if retrieved.State != automation.StateCompleted {
		t.Fatalf("expected completed generic fallback, got %s", retrieved.State)
	}
	var output struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal(retrieved.OutputData, &output); err != nil {
		t.Fatalf("unmarshal fallback output: %v", err)
	}
	if output.Intent != "general" {
		t.Fatalf("expected generic fallback intent, got %q", output.Intent)
	}
}

func TestRunnerNoToolCallsCompletes(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRun(t, store, "worker-nc")

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return noToolCallsProvider()
	}, "worker-nc", "test system prompt", nil)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	retrieved := waitForState(t, store, run.RunID, automation.StateCompleted, automation.StateFailed)

	if retrieved.State != automation.StateCompleted {
		t.Fatalf("expected completed, got %s (error: %s)", retrieved.State, retrieved.ErrorMessage)
	}

	t.Logf("run %s completed without tool calls", run.RunID)
}

func TestRunnerWithToolExecutorAllowedCompletes(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRun(t, store, "worker-tl")

	sp := toolCallThenCompleteProvider()
	exp := allowedExecutor()

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return sp
	}, "worker-tl", "test system prompt", exp)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	retrieved := waitForState(t, store, run.RunID, automation.StateCompleted, automation.StateFailed)

	if retrieved.State != automation.StateCompleted {
		t.Fatalf("expected completed, got %s. error: %s", retrieved.State, retrieved.ErrorMessage)
	}

	if sp.calls < 2 {
		t.Fatalf("expected at least 2 provider calls (initial + at least one after tool), got %d", sp.calls)
	}

	t.Logf("run %s completed after tool loop with %d provider calls", run.RunID, sp.calls)
}

func TestRunnerApprovalRequiredPausesRun(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRun(t, store, "worker-ar")

	sp := alwaysToolCallsProvider()
	exp := approvalRequiredExecutor()

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return sp
	}, "worker-ar", "test system prompt", exp)

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
		t.Fatalf("expected exactly 1 provider call before pausing for approval, got %d", sp.calls)
	}

	events, err := automation.ListEvents(context.Background(), newPool(t), run.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	foundApprovalEvent := false
	for _, e := range events {
		if e.EventName == "ApprovalRequired.v1" {
			foundApprovalEvent = true
			break
		}
	}
	if !foundApprovalEvent {
		t.Fatal("expected ApprovalRequired.v1 event")
	}

	t.Logf("run %s paused in waiting_for_approval", run.RunID)
}

func TestRunnerSixIterationHardCap(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRun(t, store, "worker-cap")

	sp := alwaysToolCallsProvider()
	exp := allowedExecutor()

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return sp
	}, "worker-cap", "test system prompt", exp)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	// store.Fail() sets 'retryable' rather than 'failed' while attempt <
	// max_attempts (see store.go's Fail, and TestFailRetries/TestRetryFailedRun
	// for the same behavior exercised directly against the store) — a fresh
	// run's first attempt lands here, not in a terminal 'failed' state.
	retrieved := waitForState(t, store, run.RunID, automation.StateRetryable, automation.StateFailed, automation.StateCompleted, automation.StateWaitingForApproval)

	if retrieved.State != automation.StateRetryable {
		t.Fatalf("expected retryable (iteration cap exceeded on first attempt), got %s. error: %s", retrieved.State, retrieved.ErrorMessage)
	}

	if retrieved.ErrorMessage == "" {
		t.Fatal("expected error message about iteration cap")
	}
	t.Logf("run %s hit 6-iteration cap and was marked retryable: %s", run.RunID, retrieved.ErrorMessage)

	if sp.calls != automation.MaxToolLoopIterations {
		t.Fatalf("expected exactly %d provider calls before cap, got %d", automation.MaxToolLoopIterations, sp.calls)
	}

	t.Logf("hard cap enforced: %d provider calls before failure", sp.calls)
}

func TestRunnerToolCallProposedEvent(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRun(t, store, "worker-ev")

	sp := toolCallThenCompleteProvider()
	exp := allowedExecutor()

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return sp
	}, "worker-ev", "test system prompt", exp)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	waitForState(t, store, run.RunID, automation.StateCompleted, automation.StateFailed)

	events, err := automation.ListEvents(context.Background(), newPool(t), run.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}

	foundProposed := false
	for _, e := range events {
		if e.EventName == "ToolCallProposed.v1" {
			foundProposed = true
			var data map[string]any
			if err := json.Unmarshal(e.EventData, &data); err != nil {
				t.Fatalf("unmarshal ToolCallProposed event_data: %v", err)
			}
			if data["tool_name"] != "get_property_operating_summary" {
				t.Fatalf("expected tool_name get_property_operating_summary, got %v", data["tool_name"])
			}
			if data["version"] != "v1" {
				t.Fatalf("expected version v1, got %v", data["version"])
			}
			if _, ok := data["arguments"]; !ok {
				t.Fatal("expected arguments in event_data")
			}
			break
		}
	}
	if !foundProposed {
		t.Fatal("expected ToolCallProposed.v1 event")
	}

	t.Logf("ToolCallProposed.v1 event verified with correct shape")
}

func TestRunnerUsageAccumulation(t *testing.T) {
	store := newStore(t)
	run := submitQueuedRun(t, store, "worker-usage")

	sp := toolCallThenCompleteProvider()
	exp := allowedExecutor()

	runner := automation.NewRunnerWithToolLoop(store, func(kind string) automation.Provider {
		return sp
	}, "worker-usage", "test system prompt", exp)

	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer wg.Wait()
	defer cancel()
	go runner.RunWorkLoop(ctx, &wg, []string{"superhost"})

	retrieved := waitForState(t, store, run.RunID, automation.StateCompleted, automation.StateFailed)

	if retrieved.State != automation.StateCompleted {
		t.Fatalf("expected completed, got %s (error: %s)", retrieved.State, retrieved.ErrorMessage)
	}

	if retrieved.UsageInputTokens <= 10 {
		t.Fatalf("expected accumulated token usage > initial call, got input=%d", retrieved.UsageInputTokens)
	}

	if retrieved.UsageTotalTokens <= 15 {
		t.Fatalf("expected accumulated total tokens > single call, got total=%d", retrieved.UsageTotalTokens)
	}

	t.Logf("run %s completed with accumulated usage: in=%d out=%d total=%d",
		run.RunID, retrieved.UsageInputTokens, retrieved.UsageOutputTokens, retrieved.UsageTotalTokens)
}
