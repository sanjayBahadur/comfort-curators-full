package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/documents"
)

func TestConformanceFinanceSliceOperations(t *testing.T) {
	spec := loadSpec(t)
	ops, err := spec.FinanceOperations()
	if err != nil {
		t.Fatalf("FinanceOperations: %v", err)
	}

	expected := map[string]struct {
		method    string
		path      string
		tag       string
		responses []string
	}{
		"createDocument": {
			method:    "POST",
			path:      "/v1/documents",
			tag:       "Documents",
			responses: []string{"201"},
		},
		"createDocumentVersion": {
			method:    "POST",
			path:      "/v1/documents/{document_id}/versions",
			tag:       "Documents",
			responses: []string{"201"},
		},
		"reviewDocument": {
			method:    "POST",
			path:      "/v1/documents/{document_id}/reviews",
			tag:       "Documents",
			responses: []string{"200"},
		},
		"confirmSubmissionPacket": {
			method:    "POST",
			path:      "/v1/submission-packets/{packet_id}/confirmations",
			tag:       "Documents",
			responses: []string{"200"},
		},
		"createCharge": {
			method:    "POST",
			path:      "/v1/billing/charges",
			tag:       "Billing",
			responses: []string{"201"},
		},
		"issueInvoice": {
			method:    "POST",
			path:      "/v1/billing/invoices",
			tag:       "Billing",
			responses: []string{"201", "409"},
		},
		"issueCredit": {
			method:    "POST",
			path:      "/v1/billing/credits",
			tag:       "Billing",
			responses: []string{"201"},
		},
		"decideFinancialApproval": {
			method:    "POST",
			path:      "/v1/financial-approvals/{approval_id}/decisions",
			tag:       "Billing",
			responses: []string{"200", "403"},
		},
		"createAccountingExport": {
			method:    "POST",
			path:      "/v1/accounting-exports",
			tag:       "Billing",
			responses: []string{"202"},
		},
		"getPropertyContributionReport": {
			method:    "GET",
			path:      "/v1/reports/property-contribution",
			tag:       "Billing",
			responses: []string{"200"},
		},
	}

	if len(ops) != len(expected) {
		t.Fatalf("FinanceOperations: expected %d operations, got %d: %+v", len(expected), len(ops), ops)
	}

	seen := map[string]bool{}
	for _, op := range ops {
		exp, ok := expected[op.OperationID]
		if !ok {
			t.Fatalf("unexpected finance operation %q (%s %s)", op.OperationID, op.Method, op.Path)
		}
		seen[op.OperationID] = true
		if op.Method != exp.method {
			t.Errorf("%s: method expected %s, got %s", op.OperationID, exp.method, op.Method)
		}
		if op.Path != exp.path {
			t.Errorf("%s: path expected %s, got %s", op.OperationID, exp.path, op.Path)
		}
		if op.Tag != exp.tag {
			t.Errorf("%s: tag expected %s, got %s", op.OperationID, exp.tag, op.Tag)
		}
		if !equalStrings(op.Responses, exp.responses) {
			t.Errorf("%s: declared responses expected %v, got %v", op.OperationID, exp.responses, op.Responses)
		}
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("finance operation %q is missing from the contract slice", id)
		}
	}
}

func TestConformanceFinanceSliceIsDistinctFromPropertySlice(t *testing.T) {
	spec := loadSpec(t)
	propertyOps, err := spec.ProtectedOperations()
	if err != nil {
		t.Fatalf("ProtectedOperations: %v", err)
	}
	financeOps, err := spec.FinanceOperations()
	if err != nil {
		t.Fatalf("FinanceOperations: %v", err)
	}

	propertyIDs := map[string]bool{}
	for _, op := range propertyOps {
		propertyIDs[op.OperationID] = true
	}
	for _, op := range financeOps {
		if propertyIDs[op.OperationID] {
			t.Errorf("finance operation %q must not be part of the property slice", op.OperationID)
		}
	}
}

func TestConformanceFinanceEnvelopesOnStubServer(t *testing.T) {
	spec := loadSpec(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/billing/charges", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, map[string]any{
			"id":      "chg_0000000000000001",
			"version": 1,
			"data": map[string]any{
				"charge_type":        "management_fee",
				"amount_minor_units": 60000000,
				"currency":           "INR",
			},
		})
	})
	mux.HandleFunc("GET /v1/reports/property-contribution", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"items": []map[string]any{
				{"id": "chg_0000000000000001", "version": 1, "data": map[string]any{"property_id": "prop_0000000000000001"}},
			},
			"next_cursor": nil,
		})
	})
	mux.HandleFunc("POST /v1/billing/invoices", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		writeJSON(t, w, map[string]any{
			"request_id": "req_0123456789abcdef",
			"code":       "INVOICE_ALREADY_ISSUED",
			"message":    "invoice already issued for this period",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, body := doPost(t, server.URL+"/v1/billing/charges", `{}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create charge: expected 201, got %d", resp.StatusCode)
	}
	if err := spec.ValidateResource(body); err != nil {
		t.Errorf("create charge: resource does not conform: %v", err)
	}

	resp, body = doGet(t, server.URL+"/v1/reports/property-contribution")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("property contribution: expected 200, got %d", resp.StatusCode)
	}
	if err := spec.ValidateCollection(body); err != nil {
		t.Errorf("property contribution: collection does not conform: %v", err)
	}

	resp, body = doPost(t, server.URL+"/v1/billing/invoices", `{}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("issue invoice: expected 409, got %d", resp.StatusCode)
	}
	if err := spec.ValidateError(body); err != nil {
		t.Errorf("issue invoice: error envelope does not conform: %v", err)
	}
}

func TestConformanceFinanceLiveHandlersEmitConformantErrors(t *testing.T) {
	spec := loadSpec(t)

	mux := http.NewServeMux()
	documents.NewHandler(nil).RegisterRoutes(mux)
	billing.NewHandler(nil).RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	financePaths := []string{
		"/v1/documents",
		"/v1/documents/doc_0000000000000001/versions",
		"/v1/billing/charges",
		"/v1/billing/invoices",
		"/v1/accounting-exports",
	}

	for _, path := range financePaths {
		resp, err := http.Post(server.URL+path, "application/json", nil)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("POST %s: expected 401, got %d: %s", path, resp.StatusCode, string(body))
		}
		// Without a correlation id the request_id is empty, so the validator
		// must flag the envelope as non-conformant, proving the finance
		// conformance check has teeth rather than accepting any shape.
		if err := spec.ValidateError(body); err == nil {
			t.Errorf("POST %s: validator must flag the empty request_id error envelope (body: %s)", path, string(body))
		}
	}

	// With a correlation id present, the live finance handlers' error envelope
	// conforms to the contract.
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/billing/charges", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-Correlation-ID", "req_0123456789abcdef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/billing/charges: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /v1/billing/charges: expected 401, got %d", resp.StatusCode)
	}
	if err := spec.ValidateError(body); err != nil {
		t.Errorf("billing error envelope does not conform with correlation id: %v", err)
	}
}

func TestConformanceFinanceErrorCodeStable(t *testing.T) {
	mux := http.NewServeMux()
	billing.NewHandler(nil).RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/billing/invoices", nil)
	req.Header.Set("X-Correlation-ID", "req_0123456789abcdef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/billing/invoices: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var errBody api.ErrorBody
	if err := json.Unmarshal(body, &errBody); err != nil {
		t.Fatalf("decode error: %v (body: %s)", err, string(body))
	}
	if errBody.Code != "UNAUTHORIZED" {
		t.Errorf("stable error code expected UNAUTHORIZED, got %s", errBody.Code)
	}
}
