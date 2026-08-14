package app

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/superhost"

	"github.com/jackc/pgx/v5/pgxpool"
)

func executorPostgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func executorTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !executorPostgresAvailable() {
		t.Skip("PostgreSQL not available; set CC_DB_HOST, CC_DB_PORT, CC_DB_USER, CC_DB_PASS, CC_DB_NAME")
	}
	connStr := os.Getenv("CC_TEST_DB")
	if connStr == "" {
		connStr = "postgres://ccuser:ccpass@localhost:5432/comfort_curators?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)

	automation.EnsureSchema(context.Background(), pool)
	superhost.EnsureToolSchema(context.Background(), pool)

	tables := []string{
		"agent_run_events", "agent_runs",
		"ai_tool_calls", "policy_decisions", "approval_requests",
	}
	for _, table := range tables {
		if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}

	return pool
}

func TestSuperhostToolExecutorUIActionReturnsSyntheticSummary(t *testing.T) {
	pool := executorTestPool(t)

	runStore := automation.NewAgentRunStore(pool)
	executor := newSuperhostToolExecutor(pool)

	run, _, err := runStore.Submit(context.Background(), automation.SubmitRequest{
		RunKind:    superhost.AgentKindSuperhost,
		TenantID:   "tenant-ui",
		PropertyID: "prop-ui",
		ActorID:    "actor-ui",
		Provider:   "model-stub",
		Model:      "stub-v1",
	})
	if err != nil {
		t.Fatalf("submit run: %v", err)
	}

	toolCallJSON := json.RawMessage(`{
		"id": "call-ui-click-001",
		"type": "function",
		"function": {
			"name": "ui_click",
			"arguments": {"surface_id": "btn-submit-checkout"}
		}
	}`)

	outcome, err := executor.Evaluate(context.Background(), run, toolCallJSON)
	if err != nil {
		t.Fatalf("evaluate ui_click: %v", err)
	}

	if outcome.Type != automation.ToolLoopAllowed {
		t.Fatalf("expected ToolLoopAllowed, got %s", outcome.Type)
	}
	if outcome.ToolName != "ui_click" {
		t.Errorf("expected tool_name ui_click, got %s", outcome.ToolName)
	}
	if outcome.ResultSummary == "" {
		t.Fatal("expected non-empty result summary")
	}

	containsSurface := false
	expectedFrags := []string{"ui action", "ui_click", "queued", "surface", "btn-submit-checkout", "does not receive confirmation"}
	for _, frag := range expectedFrags {
		if len(outcome.ResultSummary) > 0 && containsSubstr(outcome.ResultSummary, frag) {
			containsSurface = true
		}
	}
	if !containsSurface {
		t.Errorf("result summary does not contain expected ui_action phrases: %q", outcome.ResultSummary)
	}
}

func TestSuperhostToolExecutorUIActionNoSurfaceIDReturnsFallback(t *testing.T) {
	pool := executorTestPool(t)

	runStore := automation.NewAgentRunStore(pool)
	executor := newSuperhostToolExecutor(pool)

	run, _, err := runStore.Submit(context.Background(), automation.SubmitRequest{
		RunKind:    superhost.AgentKindSuperhost,
		TenantID:   "tenant-ui2",
		PropertyID: "prop-ui2",
		ActorID:    "actor-ui2",
		Provider:   "model-stub",
		Model:      "stub-v1",
	})
	if err != nil {
		t.Fatalf("submit run: %v", err)
	}

	toolCallJSON := json.RawMessage(`{
		"id": "call-ui-focus-001",
		"type": "function",
		"function": {
			"name": "ui_focus",
			"arguments": {}
		}
	}`)

	outcome, err := executor.Evaluate(context.Background(), run, toolCallJSON)
	if err != nil {
		t.Fatalf("evaluate ui_focus: %v", err)
	}

	if outcome.Type != automation.ToolLoopAllowed {
		t.Fatalf("expected ToolLoopAllowed, got %s", outcome.Type)
	}
	if outcome.ToolName != "ui_focus" {
		t.Errorf("expected tool_name ui_focus, got %s", outcome.ToolName)
	}

	wantFrag := "(no surface_id given)"
	if !containsSubstr(outcome.ResultSummary, wantFrag) {
		t.Errorf("result summary for missing surface_id should contain %q, got %q", wantFrag, outcome.ResultSummary)
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstr(s, sub)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
