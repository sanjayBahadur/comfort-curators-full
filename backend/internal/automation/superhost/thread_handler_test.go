package superhost_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/superhost"
	"comfort-curators-backend/internal/communications"
	"comfort-curators-backend/internal/contracts"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/property"
	"comfort-curators-backend/internal/reservations"

	"github.com/jackc/pgx/v5/pgxpool"
)

func threadTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)

	automation.EnsureSchema(context.Background(), pool)
	superhost.EnsureToolSchema(context.Background(), pool)
	// The rest were missing from the original version of this helper:
	// ContextAssembler.Assemble (used by CreateThread) reads from all of
	// these tables, and TestThreadToRunResolutionInStreamEndpoint seeds a
	// property + reservation directly. Matches the established testPool
	// helper in context_test.go, which sets this up for the same reason --
	// this duplicates rather than reuses that helper only because this
	// file's truncate list and pool lifecycle are otherwise independent.
	property.EnsureSchema(context.Background(), pool)
	reservations.EnsureSchema(context.Background(), pool)
	operations.EnsureSchema(context.Background(), pool)
	inventory.EnsureSchema(context.Background(), pool)
	contracts.EnsureSchema(context.Background(), pool)
	communications.EnsureSchema(context.Background(), pool)

	tables := []string{
		"agent_run_events", "agent_runs",
		"ai_tool_calls", "policy_decisions", "approval_requests",
		"superhost_threads",
		"property_compliance_holds", "property_transitions", "properties", "owner_authority_grants",
		"reservations", "calendar_exceptions", "calendar_feeds", "external_calendar_events",
		"tickets", "ticket_evidence", "incident_alerts", "service_recoveries",
		"inventory_movements", "stock_locations",
		"service_contracts", "service_contract_versions",
		"communication_preferences",
	}
	for _, table := range tables {
		if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}

	return pool
}

func setupHandler(t *testing.T, pool *pgxpool.Pool) *superhost.Handler {
	t.Helper()
	runStore := automation.NewAgentRunStore(pool)
	assembler := superhost.NewContextAssembler(pool)
	threadStore := superhost.NewThreadStore(pool, runStore, assembler)
	return superhost.NewHandlerWithThreads(runStore, assembler, threadStore)
}

// authRequest builds a request carrying an authenticated Subject in its
// context. It must be driven in-process via doRequest (mux.ServeHTTP), not
// through a real network round trip: iam.WithSubject's context value never
// crosses an actual TCP connection to a server goroutine, and separately,
// httptest.NewRequest sets RequestURI, which http.Client.Do explicitly
// rejects ("Request.RequestURI can't be set in client requests") -- a real
// httptest.NewServer + http.Client pair here was never going to
// authenticate correctly even before that panic. This was the original
// bug in this file: every authenticated test used httptest.NewServer plus
// http.DefaultClient.Do(authRequest(...)), which cannot work by
// construction. doRequest below is the fix, matching the in-process
// ServeHTTP pattern context_test.go and stream_test.go already use.
func authRequest(t *testing.T, method, path, body string, tenantID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(iam.WithSubject(req.Context(), security.Subject{
		TenantID: tenantID,
		ActorID:  tenantID,
		Roles:    []string{"superhost"},
	}))
	return req
}

// doRequest drives req through mux in-process and returns the recorded
// response. See authRequest's comment for why this replaces a real
// httptest.NewServer + http.Client round trip.
func doRequest(mux *http.ServeMux, req *http.Request) *http.Response {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

// doStreamRequest is doRequest for the streaming endpoint specifically:
// the handler blocks (polling) until the run reaches a terminal state or
// the request context is cancelled, so it must run in its own goroutine
// with an explicit timeout fallback -- matching stream_test.go's (P3.2)
// established pattern for the same reason.
func doStreamRequest(t *testing.T, mux *http.ServeMux, req *http.Request, timeout time.Duration) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		cancel()
		<-done
	}
	return rec.Result()
}

func TestCreateThreadReturns201(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"idempotency_key":"thr-test-201","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Turnover check"}`
	req := authRequest(t, http.MethodPost, "/v1/superhost/threads", body, tenantA)
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("got %d, want 201", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["id"] == nil || result["id"] == "" {
		t.Error("response must have non-empty id")
	}
	v, _ := result["version"].(float64)
	if v < 1 {
		t.Errorf("version must be >= 1, got %v", v)
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("response must have data object")
	}
	if data["thread_id"] == nil || data["thread_id"] == "" {
		t.Error("data.thread_id must be non-empty")
	}
	if data["run_id"] == nil || data["run_id"] == "" {
		t.Error("data.run_id must be non-empty")
	}
	if data["created_at"] == nil || data["created_at"] == "" {
		t.Error("data.created_at must be non-empty")
	}
}

func TestCreateThreadIdempotent(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"idempotency_key":"thr-idem-01","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Idempotency check"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", body, tenantA)
	resp1 := doRequest(mux, req1)
	defer resp1.Body.Close()
	var r1 map[string]any
	json.NewDecoder(resp1.Body).Decode(&r1)

	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads", body, tenantA)
	resp2 := doRequest(mux, req2)
	defer resp2.Body.Close()
	var r2 map[string]any
	json.NewDecoder(resp2.Body).Decode(&r2)

	if resp1.StatusCode != http.StatusCreated {
		t.Errorf("first create got %d, want 201", resp1.StatusCode)
	}
	if resp2.StatusCode != http.StatusCreated {
		t.Errorf("duplicate create got %d, want 201", resp2.StatusCode)
	}

	d1 := r1["data"].(map[string]any)
	d2 := r2["data"].(map[string]any)
	if d1["thread_id"] != d2["thread_id"] {
		t.Errorf("thread_id mismatch: %q vs %q", d1["thread_id"], d2["thread_id"])
	}
	if d1["run_id"] != d2["run_id"] {
		t.Errorf("run_id mismatch: %q vs %q", d1["run_id"], d2["run_id"])
	}
}

func TestCreateThreadCrossTenantDenied(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"idempotency_key":"thr-cross-01","tenant_id":"` + tenantB + `","property_id":"` + propA + `","purpose":"Cross tenant"}`
	req := authRequest(t, http.MethodPost, "/v1/superhost/threads", body, tenantA)
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-tenant create got %d, want 403", resp.StatusCode)
	}
}

func TestCreateThreadInvalidPropertyReturns422(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"idempotency_key":"thr-invprop-01","tenant_id":"` + tenantA + `","property_id":"nonexistent-property-id","purpose":"Invalid property"}`
	req := authRequest(t, http.MethodPost, "/v1/superhost/threads", body, tenantA)
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid property got %d, want 422", resp.StatusCode)
	}
}

func TestCreateThreadRequiresAuth(t *testing.T) {
	pool := threadTestPool(t)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/superhost/threads", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated got %d, want 401", resp.StatusCode)
	}
}

func TestSendMessageReturns202(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-msg-01","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Message test"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)

	msgBody := `{"idempotency_key":"msg-202-01","content":"Check the turnover status"}`
	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantA)
	resp2 := doRequest(mux, req2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusAccepted {
		t.Errorf("got %d, want 202", resp2.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["status"] != "accepted" {
		t.Errorf("status = %q, want 'accepted'", result["status"])
	}
	if result["request_id"] == nil || result["request_id"] == "" {
		t.Error("request_id must be non-empty")
	}
	if result["resource_id"] == nil || result["resource_id"] == "" {
		t.Error("resource_id must be non-empty")
	}
}

func TestSendMessageNotFound(t *testing.T) {
	pool := threadTestPool(t)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	msgBody := `{"idempotency_key":"msg-404-01","content":"Hello"}`
	req := authRequest(t, http.MethodPost, "/v1/superhost/threads/nonexistent-thread/messages", msgBody, tenantA)
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404", resp.StatusCode)
	}
}

func TestSendMessageContentTooLong(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-msg-long","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Long content test"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)

	longContent := strings.Repeat("x", 4001)
	msgBody := `{"idempotency_key":"msg-long-01","content":"` + longContent + `"}`
	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantA)
	resp2 := doRequest(mux, req2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", resp2.StatusCode)
	}
}

func TestSendMessageEmptyContent(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-msg-empty","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Empty content test"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)

	msgBody := `{"idempotency_key":"msg-empty-01","content":""}`
	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantA)
	resp2 := doRequest(mux, req2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", resp2.StatusCode)
	}
}

func TestSendMessageWhitespaceOnlyContent(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-msg-ws","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Whitespace test"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)

	msgBody := `{"idempotency_key":"msg-ws-01","content":"   "}`
	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantA)
	resp2 := doRequest(mux, req2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", resp2.StatusCode)
	}
}

func TestSendMessageRequiresAuth(t *testing.T) {
	pool := threadTestPool(t)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/superhost/threads/some-id/messages", strings.NewReader(`{"idempotency_key":"msg-auth-01","content":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated got %d, want 401", resp.StatusCode)
	}
}

func TestSendMessageCrossTenantForbidden(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-cross-msg","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Cross tenant msg"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)

	msgBody := `{"idempotency_key":"msg-cross-01","content":"Hello from B"}`
	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantB)
	resp2 := doRequest(mux, req2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("cross-tenant message got %d, want 403", resp2.StatusCode)
	}
}

func TestSendMessageIdempotent(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-msg-idem","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Msg idempotency"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)

	msgBody := `{"idempotency_key":"msg-idem-77","content":"Check turnover"}`
	req2a := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantA)
	resp2a := doRequest(mux, req2a)
	var ra map[string]any
	json.NewDecoder(resp2a.Body).Decode(&ra)
	resp2a.Body.Close()

	req2b := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msgBody, tenantA)
	resp2b := doRequest(mux, req2b)
	var rb map[string]any
	json.NewDecoder(resp2b.Body).Decode(&rb)
	resp2b.Body.Close()

	if resp2a.StatusCode != http.StatusAccepted {
		t.Errorf("first message got %d, want 202", resp2a.StatusCode)
	}
	if resp2b.StatusCode != http.StatusAccepted {
		t.Errorf("duplicate message got %d, want 202", resp2b.StatusCode)
	}
	if ra["resource_id"] != rb["resource_id"] {
		t.Errorf("duplicate message returned different resource_id: %q vs %q", ra["resource_id"], rb["resource_id"])
	}
}

func TestMissingIdempotencyKeyOnCreateThread(t *testing.T) {
	pool := threadTestPool(t)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Missing key"}`
	req := authRequest(t, http.MethodPost, "/v1/superhost/threads", body, tenantA)
	resp := doRequest(mux, req)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestMissingIdempotencyKeyOnSendMessage(t *testing.T) {
	pool := threadTestPool(t)
	handler := setupHandler(t, pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := authRequest(t, http.MethodPost, "/v1/superhost/threads/any-thread/messages",
		`{"content":"no idem key"}`, tenantA)
	resp := doRequest(mux, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestThreadToRunResolutionInStreamEndpoint(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)

	runStore := automation.NewAgentRunStore(pool)
	assembler := superhost.NewContextAssembler(pool)
	threadStore := superhost.NewThreadStore(pool, runStore, assembler)
	superhostHandler := superhost.NewHandlerWithThreads(runStore, assembler, threadStore)
	agentRunHandler := automation.NewAgentRunHandler(runStore)

	mux := http.NewServeMux()
	superhostHandler.RegisterRoutes(mux)
	agentRunHandler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-stream-01","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Stream test"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)
	runID := cd["run_id"].(string)

	// Nothing claims/processes this run (no runner is running in this
	// test), so it would sit "queued" forever and the stream would never
	// see a terminal state -- force one directly, the same way P3.2's own
	// TestStreamEndsWithDONEOnTerminal does, so this test can assert
	// [DONE] deterministically instead of racing an indefinite hang.
	if err := runStore.Cancel(context.Background(), runID); err != nil {
		t.Fatalf("cancel run to force terminal state: %v", err)
	}

	req2 := authRequest(t, http.MethodGet, "/v1/superhost/threads/"+threadID+"/stream", "", tenantA)
	resp2 := doStreamRequest(t, mux, req2, 5*time.Second)
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	bodyStr := string(body)

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("stream got %d, want 200. Body: %s", resp2.StatusCode, bodyStr)
	}
	if !strings.Contains(bodyStr, "data: ") {
		t.Error("stream response must contain SSE data lines")
	}
	if !strings.Contains(bodyStr, "[DONE]") {
		t.Error("stream for a thread whose run was resolved and driven to a terminal state must send [DONE]")
	}
}

func TestStreamWithThreadIDReturns404ForUnknownThread(t *testing.T) {
	pool := threadTestPool(t)
	runStore := automation.NewAgentRunStore(pool)
	handler := automation.NewAgentRunHandler(runStore)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := authRequest(t, http.MethodGet, "/v1/superhost/threads/00000000-0000-0000-0000-000000000001/stream", "", tenantA)
	resp := doStreamRequest(t, mux, req, 5*time.Second)
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404", resp.StatusCode)
	}
}

func TestUpdateThreadRunAdvancesRunID(t *testing.T) {
	pool := threadTestPool(t)
	setupFixtures(t, pool)
	handler := setupHandler(t, pool)
	runStore := automation.NewAgentRunStore(pool)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	createBody := `{"idempotency_key":"thr-adv-01","tenant_id":"` + tenantA + `","property_id":"` + propA + `","purpose":"Run advance test"}`
	req1 := authRequest(t, http.MethodPost, "/v1/superhost/threads", createBody, tenantA)
	resp1 := doRequest(mux, req1)
	var cr map[string]any
	json.NewDecoder(resp1.Body).Decode(&cr)
	resp1.Body.Close()
	cd := cr["data"].(map[string]any)
	threadID := cd["thread_id"].(string)
	initialRunID := cd["run_id"].(string)

	// After creation, GetRunIDByThreadID must return the initial run.
	resolved, err := runStore.GetRunIDByThreadID(context.Background(), threadID)
	if err != nil {
		t.Fatalf("GetRunIDByThreadID after create: %v", err)
	}
	if resolved != initialRunID {
		t.Errorf("after create: GetRunIDByThreadID = %q, want %q", resolved, initialRunID)
	}

	// Send first message. handleSendMessage now calls UpdateThreadRun.
	msg1Body := `{"idempotency_key":"msg-adv-01","content":"First message"}`
	req2 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msg1Body, tenantA)
	resp2 := doRequest(mux, req2)
	var mr1 map[string]any
	json.NewDecoder(resp2.Body).Decode(&mr1)
	resp2.Body.Close()
	firstMsgRunID := mr1["resource_id"].(string)

	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("first message got %d, want 202", resp2.StatusCode)
	}

	// After first message, the thread's run_id must be the new run.
	resolved, err = runStore.GetRunIDByThreadID(context.Background(), threadID)
	if err != nil {
		t.Fatalf("GetRunIDByThreadID after first message: %v", err)
	}
	if resolved != firstMsgRunID {
		t.Errorf("after first message: GetRunIDByThreadID = %q, want %q", resolved, firstMsgRunID)
	}
	if resolved == initialRunID {
		t.Errorf("run_id did not advance: still %q after first message", resolved)
	}

	// Send second message. Must advance again.
	msg2Body := `{"idempotency_key":"msg-adv-02","content":"Second message"}`
	req3 := authRequest(t, http.MethodPost, "/v1/superhost/threads/"+threadID+"/messages", msg2Body, tenantA)
	resp3 := doRequest(mux, req3)
	var mr2 map[string]any
	json.NewDecoder(resp3.Body).Decode(&mr2)
	resp3.Body.Close()
	secondMsgRunID := mr2["resource_id"].(string)

	if resp3.StatusCode != http.StatusAccepted {
		t.Fatalf("second message got %d, want 202", resp3.StatusCode)
	}

	resolved, err = runStore.GetRunIDByThreadID(context.Background(), threadID)
	if err != nil {
		t.Fatalf("GetRunIDByThreadID after second message: %v", err)
	}
	if resolved != secondMsgRunID {
		t.Errorf("after second message: GetRunIDByThreadID = %q, want %q", resolved, secondMsgRunID)
	}
	if resolved == firstMsgRunID {
		t.Errorf("run_id did not advance on second message: still %q", resolved)
	}
}
