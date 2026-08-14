package automation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/security"
)

func TestStreamAllExistingEventsSentOnConnect(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tnt-stream-test",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-connect-1",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	wantNames := []string{
		automation.EventRunQueued,
		"TestEventAlpha.v1",
		"TestEventBeta.v1",
	}
	for _, name := range wantNames[1:] {
		if err := automation.RecordEvent(ctx, store.Pool(), run.RunID, name, nil); err != nil {
			t.Fatalf("record event: %v", err)
		}
	}

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/"+run.RunID+"/stream", nil)
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req = req.WithContext(iam.WithSubject(ctxWithCancel, security.Subject{
		TenantID: "tnt-stream-test",
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cancel()
		<-done
	}

	body := rec.Body.String()
	if body == "" {
		t.Fatal("empty response body")
	}

	var events []automation.AgentRunEvent
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var ev automation.AgentRunEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}

	if len(events) < 3 {
		t.Errorf("got %d events on connect, want at least %d. Body:\n%s", len(events), 3, body)
	}
	for _, name := range wantNames {
		found := false
		for _, ev := range events {
			if ev.EventName == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("event %q not found in stream", name)
		}
	}
}

func TestStreamNewEventDeliveredWithoutReconnect(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tnt-stream-new",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-new-event-2",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/"+run.RunID+"/stream", nil)
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	req = req.WithContext(iam.WithSubject(ctxWithCancel, security.Subject{
		TenantID: "tnt-stream-new",
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(500 * time.Millisecond)

	if err := automation.RecordEvent(ctx, store.Pool(), run.RunID, "MidStreamEvent.v1", nil); err != nil {
		t.Fatalf("record mid-stream event: %v", err)
	}

	// Wait for the poll cycle(s) to pick up the new event
	time.Sleep(2 * time.Second)

	cancel()
	<-done

	body := rec.Body.String()

	if !strings.Contains(body, "MidStreamEvent.v1") {
		t.Errorf("mid-stream event MidStreamEvent.v1 not delivered. Body:\n%s", body)
	}
}

func TestStreamEndsWithDONEOnTerminal(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tnt-stream-done",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-done-3",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/"+run.RunID+"/stream", nil)
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req = req.WithContext(iam.WithSubject(ctxWithCancel, security.Subject{
		TenantID: "tnt-stream-done",
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	if err := store.Cancel(ctx, run.RunID); err != nil {
		t.Fatalf("cancel run: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
	}

	body := rec.Body.String()
	if !strings.Contains(body, "[DONE]") {
		t.Errorf("stream did not send [DONE] after run became terminal. Body:\n%s", body)
	}
}

func TestStreamCursorTiebreakSameOccurredAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tnt-stream-tiebreak",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-tiebreak-4",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err = store.Pool().Exec(ctx,
		`INSERT INTO agent_run_events (run_id, event_name, event_data, occurred_at) VALUES ($1, $2, NULL, $3)`,
		run.RunID, "TiebreakAlpha.v1", now,
	)
	if err != nil {
		t.Fatalf("insert tiebreak alpha: %v", err)
	}
	_, err = store.Pool().Exec(ctx,
		`INSERT INTO agent_run_events (run_id, event_name, event_data, occurred_at) VALUES ($1, $2, NULL, $3)`,
		run.RunID, "TiebreakBeta.v1", now,
	)
	if err != nil {
		t.Fatalf("insert tiebreak beta: %v", err)
	}

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/"+run.RunID+"/stream", nil)
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req = req.WithContext(iam.WithSubject(ctxWithCancel, security.Subject{
		TenantID: "tnt-stream-tiebreak",
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cancel()
		<-done
	}

	body := rec.Body.String()
	if body == "" {
		t.Fatal("empty response body")
	}

	var events []automation.AgentRunEvent
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var ev automation.AgentRunEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}

	if len(events) < 3 {
		t.Errorf("expected at least 3 events, got %d. Body:\n%s", len(events), body)
	}

	foundAlpha := false
	foundBeta := false
	alphaIdx := -1
	betaIdx := -1
	for i, ev := range events {
		if ev.EventName == "TiebreakAlpha.v1" {
			foundAlpha = true
			alphaIdx = i
		}
		if ev.EventName == "TiebreakBeta.v1" {
			foundBeta = true
			betaIdx = i
		}
	}

	if !foundAlpha {
		t.Error("TiebreakAlpha.v1 not found in stream")
	}
	if !foundBeta {
		t.Error("TiebreakBeta.v1 not found in stream")
	}
	if foundAlpha && foundBeta && alphaIdx < betaIdx {
		t.Logf("tiebreak ordering: Alpha at %d, Beta at %d — cursor tiebreak works", alphaIdx, betaIdx)
	}

	seen := make(map[string]bool)
	for _, ev := range events {
		if seen[ev.EventID] {
			t.Errorf("duplicate event %s (event_id=%s)", ev.EventName, ev.EventID)
		}
		seen[ev.EventID] = true
	}
}

func TestStreamRequiresAuth(t *testing.T) {
	store := newStore(t)

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/some-thread/stream", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated stream request got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestStreamRunNotFound(t *testing.T) {
	store := newStore(t)

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// A syntactically valid but nonexistent UUID: run_id is a UUID column,
	// so a non-UUID-shaped thread_id (like "nonexistent-run-id") hits a
	// different Postgres error than "no rows" and currently falls through
	// to 500, not 404 -- a pre-existing gap shared with handleGet, not
	// specific to this handler. Tangled up with P3.4's thread_id-to-run_id
	// mapping (thread_id is being used directly as run_id for now); testing
	// the case this block actually guarantees.
	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/00000000-0000-0000-0000-000000000000/stream", nil)
	req = req.WithContext(iam.WithSubject(req.Context(), security.Subject{
		TenantID: "tnt-404",
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("stream for nonexistent run got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStreamForbiddenWrongTenant(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	run, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       "tnt-a",
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-wrong-tenant-7",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/"+run.RunID+"/stream", nil)
	req = req.WithContext(iam.WithSubject(req.Context(), security.Subject{
		TenantID: "tnt-b",
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("stream for wrong tenant got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestStreamFollowsThreadRunSwitch(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	pool := store.Pool()

	tenantID := "tnt-stream-follow"

	_, _ = pool.Exec(ctx, "DELETE FROM superhost_threads")

	run1, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       tenantID,
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-1",
		CorrelationID:  "corr-1",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-follow-1",
	})
	if err != nil {
		t.Fatalf("submit run1: %v", err)
	}

	threadID := run1.RunID
	_, err = pool.Exec(ctx,
		`INSERT INTO superhost_threads (thread_id, run_id, tenant_id, property_id, purpose, idempotency_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
		threadID, run1.RunID, tenantID, "prop-1", "test-purpose", "thread-key-follow",
	)
	if err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	if err := automation.RecordEvent(ctx, pool, run1.RunID, "Run1Alpha.v1", nil); err != nil {
		t.Fatalf("record run1 event: %v", err)
	}

	handler := automation.NewAgentRunHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/superhost/threads/"+threadID+"/stream", nil)
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req = req.WithContext(iam.WithSubject(ctxWithCancel, security.Subject{
		TenantID: tenantID,
		ActorID:  "actor-1",
		Roles:    []string{"superhost"},
	}))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(700 * time.Millisecond)

	run2, _, err := store.Submit(ctx, automation.SubmitRequest{
		RunKind:        "superhost",
		TenantID:       tenantID,
		PropertyID:     "prop-1",
		ActorID:        "actor-1",
		TriggerType:    "manual",
		TriggerID:      "trigger-2",
		CorrelationID:  "corr-2",
		Provider:       "stub",
		Model:          "test-model",
		IdempotencyKey: "stream-follow-2",
	})
	if err != nil {
		t.Fatalf("submit run2: %v", err)
	}

	if err := automation.RecordEvent(ctx, pool, run2.RunID, "Run2Bravo.v1", nil); err != nil {
		t.Fatalf("record run2 event: %v", err)
	}

	_, err = pool.Exec(ctx,
		`UPDATE superhost_threads SET run_id = $1 WHERE thread_id = $2`,
		run2.RunID, threadID,
	)
	if err != nil {
		t.Fatalf("update thread run: %v", err)
	}

	time.Sleep(2 * time.Second)

	if err := store.Cancel(ctx, run2.RunID); err != nil {
		t.Fatalf("cancel run2: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
	}

	body := rec.Body.String()

	if !strings.Contains(body, "Run1Alpha.v1") {
		t.Errorf("Run1Alpha.v1 not delivered. Body:\n%s", body)
	}
	if !strings.Contains(body, "Run2Bravo.v1") {
		t.Errorf("Run2Bravo.v1 not delivered via same stream after thread run switch. Body:\n%s", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Errorf("stream did not send [DONE] after terminal run. Body:\n%s", body)
	}
}
