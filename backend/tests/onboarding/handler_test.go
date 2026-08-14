package onboarding_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/onboarding"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/security"

	"github.com/jackc/pgx/v5/pgxpool"
)

type apiRes struct {
	ID      string          `json:"id"`
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type apiErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiColl struct {
	Items []apiRes `json:"items"`
}

func handlerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !onboardingPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}
	pool, err := pgxpool.New(context.Background(), onboardingDBConnString())
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return pool
}

func handlerTestServer(t *testing.T, pool *pgxpool.Pool, tenantID string) (*onboarding.OnboardingHandler, http.Handler) {
	t.Helper()
	ctx := context.Background()
	if err := onboarding.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure onboarding schema: %v", err)
	}
	if err := audit.EnsureSchema(ctx, pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	auditStore := audit.NewAuditStore(pool)
	svc := onboarding.NewService(pool, auditStore).WithAuthorizer(testAuthorizer{tenant: tenantID})
	handler := onboarding.NewOnboardingHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	authMw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := iam.WithSubject(r.Context(), security.Subject{
				ActorID:  "test-actor",
				TenantID: tenantID,
				Roles:    []string{"owner"},
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	return handler, authMw(mux)
}

func doJSON(t *testing.T, method, path string, body any, handler http.Handler) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		r = strings.NewReader(string(b))
	}

	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, respBody
}

func parseResource(t *testing.T, body []byte) apiRes {
	t.Helper()
	var res apiRes
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("parse resource: %v (body: %s)", err, string(body))
	}
	return res
}

func TestOnboardingHandlerStartAndGet(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-start"
	_, handler := handlerTestServer(t, pool, tenantID)

	resp, body := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-handler-1",
		"owner_authority_id": "owner-auth-1",
	}, handler)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start case: expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	res := parseResource(t, body)
	if res.ID == "" {
		t.Fatal("start case must return an id")
	}
	if res.Version != 1 {
		t.Errorf("new case must start at version 1, got %d", res.Version)
	}

	var data struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatalf("parse case data: %v", err)
	}
	if data.Status != "in_progress" {
		t.Errorf("new case must be in_progress, got %q", data.Status)
	}

	resp2, body2 := doJSON(t, "GET", "/v1/onboarding/cases/"+res.ID, nil, handler)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("get case: expected 200, got %d: %s", resp2.StatusCode, string(body2))
	}

	getRes := parseResource(t, body2)
	if getRes.ID != res.ID {
		t.Errorf("get case id mismatch: %q vs %q", getRes.ID, res.ID)
	}
	etag := resp2.Header.Get("ETag")
	if etag == "" {
		t.Error("get case must include ETag header")
	}
}

func TestOnboardingHandlerSaveSectionAndResume(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-section"
	_, handler := handlerTestServer(t, pool, tenantID)

	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-handler-2",
		"owner_authority_id": "owner-auth-2",
	}, handler)
	started := parseResource(t, startBody)

	_, sectionBody := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/sections/portfolio", map[string]any{
		"payload": map[string]any{
			"property_name": "Sea View Villa",
			"property_type": "villa",
			"managed_units": 2,
		},
	}, handler)
	sectionRes := parseResource(t, sectionBody)
	if sectionRes.Version != 2 {
		t.Errorf("version must advance to 2 after section save, got %d", sectionRes.Version)
	}

	_, goalsBody := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/sections/goals", map[string]any{
		"payload": map[string]any{
			"primary_goal":    "maximize_occupancy",
			"rental_strategy": "fixed_price",
		},
	}, handler)
	goalsRes := parseResource(t, goalsBody)
	if goalsRes.Version != 3 {
		t.Errorf("version must advance to 3 after goals save, got %d", goalsRes.Version)
	}

	// Resume: get the case and verify sections survived
	resp, getBody := doJSON(t, "GET", "/v1/onboarding/cases/"+started.ID, nil, handler)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get case: expected 200, got %d: %s", resp.StatusCode, string(getBody))
	}

	var caseData struct {
		Status    string          `json:"status"`
		Portfolio json.RawMessage `json:"portfolio"`
		Goals     json.RawMessage `json:"goals"`
	}
	getRes := parseResource(t, getBody)
	if err := json.Unmarshal(getRes.Data, &caseData); err != nil {
		t.Fatalf("parse case data: %v", err)
	}
	if caseData.Portfolio == nil {
		t.Error("portfolio must survive and be returned")
	}
	if caseData.Goals == nil {
		t.Error("goals must survive and be returned")
	}

	// Progress shows both steps complete
	_, progBody := doJSON(t, "GET", "/v1/onboarding/cases/"+started.ID+"/progress", nil, handler)
	var progressResp struct {
		Progress []onboarding.StepProgress `json:"progress"`
	}
	if err := json.Unmarshal(progBody, &progressResp); err != nil {
		t.Fatalf("parse progress: %v", err)
	}
	byKey := map[string]bool{}
	for _, p := range progressResp.Progress {
		byKey[p.Key] = p.Complete
	}
	if !byKey[onboarding.StepPortfolio] {
		t.Error("portfolio must report complete in progress")
	}
	if !byKey[onboarding.StepGoals] {
		t.Error("goals must report complete in progress")
	}
}

func TestOnboardingHandlerListCases(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-list"
	_, handler := handlerTestServer(t, pool, tenantID)

	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-list-1",
		"owner_authority_id": "owner-list-1",
	}, handler)
	start1 := parseResource(t, startBody)

	_, startBody2 := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-list-2",
		"owner_authority_id": "owner-list-2",
	}, handler)
	parseResource(t, startBody2)

	resp, listBody := doJSON(t, "GET", "/v1/onboarding/cases", nil, handler)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list cases: expected 200, got %d: %s", resp.StatusCode, string(listBody))
	}

	var coll apiColl
	if err := json.Unmarshal(listBody, &coll); err != nil {
		t.Fatalf("parse list: %v (body: %s)", err, string(listBody))
	}
	if len(coll.Items) < 2 {
		t.Errorf("list must include at least 2 cases, got %d", len(coll.Items))
	}

	found := false
	for _, item := range coll.Items {
		if item.ID == start1.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("list must include the first created case")
	}
}

func TestOnboardingHandlerSaveContacts(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-contacts"
	_, handler := handlerTestServer(t, pool, tenantID)

	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-contacts-1",
		"owner_authority_id": "owner-contacts-1",
	}, handler)
	started := parseResource(t, startBody)

	resp, contactsBody := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/contacts", map[string]any{
		"contacts": []map[string]any{
			{"name": "Asha Kumar", "role": "property_manager", "phone": "+91-9876543210", "email": "asha@example.com"},
			{"name": "Raj Singh", "role": "emergency", "phone": "+91-9876543211"},
		},
	}, handler)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save contacts: expected 200, got %d: %s", resp.StatusCode, string(contactsBody))
	}

	contactsRes := parseResource(t, contactsBody)
	var data struct {
		Contacts []onboarding.Contact `json:"contacts"`
	}
	if err := json.Unmarshal(contactsRes.Data, &data); err != nil {
		t.Fatalf("parse contacts data: %v", err)
	}
	if len(data.Contacts) != 2 {
		t.Errorf("expected 2 contacts, got %d", len(data.Contacts))
	}

	// Missing name/phone must be rejected
	resp2, errBody := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/contacts", map[string]any{
		"contacts": []map[string]any{
			{"role": "manager"},
		},
	}, handler)
	if resp2.StatusCode == http.StatusOK {
		t.Errorf("contacts missing name/phone must be rejected, got 200: %s", string(errBody))
	}
}

func TestOnboardingHandlerMissingEvidenceBlocksActivation(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-evidence"
	_, handler := handlerTestServer(t, pool, tenantID)

	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-evidence-1",
		"owner_authority_id": "owner-evidence-1",
	}, handler)
	started := parseResource(t, startBody)

	// Save all sections except evidence
	sections := map[string]map[string]any{
		"portfolio":           {"property_name": "Test Villa", "property_type": "villa", "managed_units": 1},
		"goals":               {"primary_goal": "maximize_occupancy", "rental_strategy": "fixed_price"},
		"service_preferences": {"communication_channel": "email", "currency": "INR"},
		"budgets":             {"currency": "INR", "monthly_budget_minor_units": 5000000},
		"photographs":         {"objects": []map[string]any{{"object_ref": "obj/p1", "caption": "living room"}}},
		"amenities":           {"items": []map[string]any{{"name": "wifi", "quantity": 1}}},
		"safety":              {"smoke_detectors_installed": true, "fire_extinguisher_present": true},
		"furnishing":          {"furnishing_level": "fully_furnished", "inventory_count": 20},
		"remediation":         {"open_items": []map[string]any{}, "completed_items": []map[string]any{}},
		"fit_score_inputs":    {"property_score": 8, "market_score": 7, "operations_score": 8, "renovation_score": 6, "occupancy_score": 7},
	}
	for name, payload := range sections {
		resp, body := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/sections/"+name, map[string]any{
			"payload": payload,
		}, handler)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("save section %s: expected 200, got %d: %s", name, resp.StatusCode, string(body))
		}
	}
	doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/contacts", map[string]any{
		"contacts": []map[string]any{{"name": "Asha", "phone": "+91-9000000000"}},
	}, handler)

	// Record document evidence
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind":         "document",
		"content_hash": "sha256:doc",
		"object_ref":   "obj/doc",
	}, handler)

	// Record inspection
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/inspections", map[string]any{
		"property_id":    "prop-evidence-1",
		"inspected_by":   "inspector-1",
		"evidence_hash":  "sha256:inspection",
		"evidence_ref":   "obj/inspection",
		"findings":       "no issues",
		"overall_status": "pass",
	}, handler)

	// Check activation holds: should have 2 (missing legal + safety)
	_, holdsBody := doJSON(t, "GET", "/v1/onboarding/cases/"+started.ID+"/activation-holds", nil, handler)
	var holdsResp struct {
		Holds       []onboarding.ActivationHold `json:"holds"`
		CanActivate bool                        `json:"can_activate"`
	}
	if err := json.Unmarshal(holdsBody, &holdsResp); err != nil {
		t.Fatalf("parse activation holds: %v", err)
	}
	if len(holdsResp.Holds) != 2 {
		t.Fatalf("expected 2 activation holds (legal+safety), got %d: %+v", len(holdsResp.Holds), holdsResp.Holds)
	}
	if holdsResp.CanActivate {
		t.Error("can_activate must be false with missing evidence")
	}

	// Activation must be blocked
	resp, actBody := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/activate", map[string]any{}, handler)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("activation without evidence: expected 409, got %d: %s", resp.StatusCode, string(actBody))
	}
	var actErr apiErr
	if err := json.Unmarshal(actBody, &actErr); err != nil {
		t.Fatalf("parse activation error: %v", err)
	}
	if actErr.Code != "ACTIVATION_BLOCKED" {
		t.Errorf("activation blocked must return ACTIVATION_BLOCKED, got %q", actErr.Code)
	}

	// Record legal evidence
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind":         "legal",
		"content_hash": "sha256:legal",
		"object_ref":   "obj/legal",
	}, handler)

	// Still blocked (safety missing)
	resp2, actBody2 := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/activate", map[string]any{}, handler)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("activation with only legal evidence: expected 409, got %d: %s", resp2.StatusCode, string(actBody2))
	}

	// Record safety evidence
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind":         "safety",
		"content_hash": "sha256:safety",
		"object_ref":   "obj/safety",
	}, handler)

	// Now check holds: should be 0
	_, holdsBody2 := doJSON(t, "GET", "/v1/onboarding/cases/"+started.ID+"/activation-holds", nil, handler)
	var holdsResp2 struct {
		Holds       []onboarding.ActivationHold `json:"holds"`
		CanActivate bool                        `json:"can_activate"`
	}
	if err := json.Unmarshal(holdsBody2, &holdsResp2); err != nil {
		t.Fatalf("parse activation holds: %v", err)
	}
	if len(holdsResp2.Holds) != 0 {
		t.Errorf("activation holds must be empty after evidence, got %d", len(holdsResp2.Holds))
	}
	if !holdsResp2.CanActivate {
		t.Error("can_activate must be true with all evidence")
	}

	// Activation must succeed
	resp3, actBody3 := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/activate", map[string]any{}, handler)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("activation with all evidence: expected 200, got %d: %s", resp3.StatusCode, string(actBody3))
	}

	var actData struct {
		Status string `json:"status"`
	}
	actRes := parseResource(t, actBody3)
	if err := json.Unmarshal(actRes.Data, &actData); err != nil {
		t.Fatalf("parse activated case data: %v", err)
	}
	if actData.Status != "activated" {
		t.Errorf("case must be activated, got %q", actData.Status)
	}

	// Double activation must fail
	resp4, _ := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/activate", map[string]any{}, handler)
	if resp4.StatusCode != http.StatusConflict {
		t.Errorf("double activation: expected 409, got %d", resp4.StatusCode)
	}
}

func TestOnboardingHandlerInspectionIsImmutable(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-insp"
	_, handler := handlerTestServer(t, pool, tenantID)

	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-insp-1",
		"owner_authority_id": "owner-insp-1",
	}, handler)
	started := parseResource(t, startBody)

	resp, inspBody := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/inspections", map[string]any{
		"property_id":    "prop-insp-1",
		"inspected_by":   "inspector-1",
		"evidence_hash":  "sha256:inspection-original",
		"evidence_ref":   "obj/inspection-original",
		"findings":       "all clear",
		"overall_status": "pass",
	}, handler)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("record inspection: expected 201, got %d: %s", resp.StatusCode, string(inspBody))
	}

	var inspRes apiRes
	if err := json.Unmarshal(inspBody, &inspRes); err != nil {
		t.Fatalf("parse inspection response: %v", err)
	}
	originalID := inspRes.ID

	// Verify via GET
	_, getBody := doJSON(t, "GET", "/v1/onboarding/cases/"+started.ID, nil, handler)
	getRes := parseResource(t, getBody)

	var caseData struct {
		Inspections []onboarding.Inspection `json:"inspections"`
	}
	if err := json.Unmarshal(getRes.Data, &caseData); err != nil {
		t.Fatalf("parse case data: %v", err)
	}
	if len(caseData.Inspections) != 1 {
		t.Fatalf("expected 1 inspection, got %d", len(caseData.Inspections))
	}
	if caseData.Inspections[0].EvidenceHash != "sha256:inspection-original" {
		t.Errorf("inspection evidence hash must round-trip, got %q", caseData.Inspections[0].EvidenceHash)
	}

	// Record a corrected inspection: must be a new record
	resp2, inspBody2 := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/inspections", map[string]any{
		"property_id":    "prop-insp-1",
		"inspected_by":   "inspector-1",
		"evidence_hash":  "sha256:inspection-correction",
		"evidence_ref":   "obj/inspection-correction",
		"findings":       "minor fix needed",
		"overall_status": "conditional",
	}, handler)

	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("record corrected inspection: expected 201, got %d: %s", resp2.StatusCode, string(inspBody2))
	}

	var correctionRes apiRes
	if err := json.Unmarshal(inspBody2, &correctionRes); err != nil {
		t.Fatalf("parse corrected inspection: %v", err)
	}
	if correctionRes.ID == originalID {
		t.Error("corrected inspection must be a new record, not the same ID")
	}

	// Both must be present
	_, finalBody := doJSON(t, "GET", "/v1/onboarding/cases/"+started.ID, nil, handler)
	finalRes := parseResource(t, finalBody)
	var finalData struct {
		Inspections []onboarding.Inspection `json:"inspections"`
	}
	if err := json.Unmarshal(finalRes.Data, &finalData); err != nil {
		t.Fatalf("parse final case data: %v", err)
	}
	if len(finalData.Inspections) != 2 {
		t.Fatalf("expected 2 inspections after correction, got %d", len(finalData.Inspections))
	}
	if finalData.Inspections[0].EvidenceHash != "sha256:inspection-original" {
		t.Errorf("original inspection must stay stable, got %q", finalData.Inspections[0].EvidenceHash)
	}
}

func TestOnboardingHandlerValidationErrors(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-validation"
	_, handler := handlerTestServer(t, pool, tenantID)

	// Missing property_id
	resp, body := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"owner_authority_id": "owner-v-1",
	}, handler)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("missing property_id: expected 422, got %d: %s", resp.StatusCode, string(body))
	}

	// Missing owner_authority_id
	resp2, body2 := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":   tenantID,
		"property_id": "prop-v-1",
	}, handler)
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("missing owner_authority_id: expected 422, got %d: %s", resp2.StatusCode, string(body2))
	}

	// Create a valid case first
	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-v-2",
		"owner_authority_id": "owner-v-2",
	}, handler)
	started := parseResource(t, startBody)

	// Invalid section name
	resp3, body3 := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/sections/bogus", map[string]any{
		"payload": map[string]any{},
	}, handler)
	if resp3.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid section: expected 422, got %d: %s", resp3.StatusCode, string(body3))
	}

	// Invalid evidence
	resp4, body4 := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind": "unknown",
	}, handler)
	if resp4.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid evidence kind: expected 422, got %d: %s", resp4.StatusCode, string(body4))
	}

	// Missing required fields in evidence
	resp5, body5 := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind": "legal",
	}, handler)
	if resp5.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("missing evidence fields: expected 422, got %d: %s", resp5.StatusCode, string(body5))
	}
}

func TestOnboardingHandlerCaseNotFound(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-notfound"
	_, handler := handlerTestServer(t, pool, tenantID)

	nonexistentID := "00000000-0000-0000-0000-000000000000"

	resp, _ := doJSON(t, "GET", "/v1/onboarding/cases/"+nonexistentID, nil, handler)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get nonexistent: expected 404, got %d", resp.StatusCode)
	}

	resp2, _ := doJSON(t, "PUT", "/v1/onboarding/cases/"+nonexistentID+"/sections/portfolio", map[string]any{
		"payload": map[string]any{"property_name": "nope"},
	}, handler)
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("save section to nonexistent: expected 404, got %d", resp2.StatusCode)
	}
}

func TestOnboardingHandlerActivatedCaseRejectsMutations(t *testing.T) {
	pool := handlerTestPool(t)
	tenantID := "tenant-onb-handler-activated"
	_, handler := handlerTestServer(t, pool, tenantID)

	_, startBody := doJSON(t, "POST", "/v1/onboarding/cases", map[string]any{
		"tenant_id":          tenantID,
		"property_id":        "prop-activated-1",
		"owner_authority_id": "owner-activated-1",
	}, handler)
	started := parseResource(t, startBody)

	// Complete all sections
	sections := map[string]map[string]any{
		"portfolio":           {"property_name": "Test", "property_type": "villa", "managed_units": 1},
		"goals":               {"primary_goal": "test", "rental_strategy": "fixed_price"},
		"service_preferences": {"communication_channel": "email", "currency": "INR"},
		"budgets":             {"currency": "INR", "monthly_budget_minor_units": 1000},
		"photographs":         {"objects": []map[string]any{{"object_ref": "obj/p1"}}},
		"amenities":           {"items": []map[string]any{{"name": "wifi", "quantity": 1}}},
		"safety":              {"smoke_detectors_installed": true},
		"furnishing":          {"furnishing_level": "basic", "inventory_count": 1},
		"remediation":         {},
		"fit_score_inputs":    {"property_score": 5, "market_score": 5, "operations_score": 5, "renovation_score": 5, "occupancy_score": 5},
	}
	for name, payload := range sections {
		doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/sections/"+name, map[string]any{
			"payload": payload,
		}, handler)
	}
	doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/contacts", map[string]any{
		"contacts": []map[string]any{{"name": "Asha", "phone": "+91-9000000000"}},
	}, handler)
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind": "document", "content_hash": "sha256:doc", "object_ref": "obj/doc",
	}, handler)
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind": "legal", "content_hash": "sha256:legal", "object_ref": "obj/legal",
	}, handler)
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind": "safety", "content_hash": "sha256:safety", "object_ref": "obj/safety",
	}, handler)
	doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/inspections", map[string]any{
		"property_id": "prop-activated-1", "inspected_by": "insp-1", "evidence_hash": "sha256:insp",
		"evidence_ref": "obj/insp", "findings": "ok", "overall_status": "pass",
	}, handler)

	// Activate
	resp, _ := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/activate", map[string]any{}, handler)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activation must succeed, got %d", resp.StatusCode)
	}

	// Mutations must now be rejected
	resp2, _ := doJSON(t, "PUT", "/v1/onboarding/cases/"+started.ID+"/sections/portfolio", map[string]any{
		"payload": map[string]any{"property_name": "mutated"},
	}, handler)
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("section save on activated case: expected 409, got %d", resp2.StatusCode)
	}

	resp3, _ := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/evidence", map[string]any{
		"kind": "legal", "content_hash": "sha256:extra", "object_ref": "obj/extra",
	}, handler)
	if resp3.StatusCode != http.StatusConflict {
		t.Errorf("evidence on activated case: expected 409, got %d", resp3.StatusCode)
	}

	resp4, _ := doJSON(t, "POST", "/v1/onboarding/cases/"+started.ID+"/inspections", map[string]any{
		"property_id": "prop-activated-1", "inspected_by": "insp-2", "evidence_hash": "sha256:extra2",
		"evidence_ref": "obj/extra2", "findings": "x", "overall_status": "fail",
	}, handler)
	if resp4.StatusCode != http.StatusConflict {
		t.Errorf("inspection on activated case: expected 409, got %d", resp4.StatusCode)
	}
}
