package billing

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/billing/charges", h.handleCreateCharge)
	mux.HandleFunc("POST /v1/billing/invoices", h.handleIssueInvoice)
	mux.HandleFunc("POST /v1/billing/credits", h.handleIssueCredit)
	mux.HandleFunc("POST /v1/financial-approvals/{approval_id}/decisions", h.handleDecideFinancialApproval)
	mux.HandleFunc("POST /v1/accounting-exports", h.handleCreateAccountingExport)
	mux.HandleFunc("GET /v1/reports/property-contribution", h.handleGetPropertyContributionReport)
	mux.HandleFunc("POST /v1/maker-checker/requests", h.handleCreateMakerCheckerRequest)
	mux.HandleFunc("POST /v1/maker-checker/requests/{request_id}/submit", h.handleSubmitMakerCheckerRequest)
	mux.HandleFunc("POST /v1/maker-checker/decisions", h.handleDecideMakerCheckerRequest)
	mux.HandleFunc("POST /v1/bank-verifications", h.handleCreateBankVerification)
	mux.HandleFunc("POST /v1/bank-verifications/{verification_id}/confirm", h.handleConfirmBankVerification)
	mux.HandleFunc("POST /v1/journal/finalize", h.handleFinalizeJournal)
	mux.HandleFunc("POST /v1/reconciliation-exceptions", h.handleRecordReconciliationException)
	mux.HandleFunc("GET /v1/reconciliation-exceptions", h.handleListReconciliationExceptions)
	mux.HandleFunc("POST /v1/reconciliation-exceptions/{exception_id}/resolve", h.handleResolveReconciliationException)
}

type billingResource struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Data    any    `json:"data"`
}

type billingError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

type billingAccepted struct {
	RequestID  string `json:"request_id"`
	Status     string `json:"status"`
	ResourceID string `json:"resource_id"`
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func subjectActor(r *http.Request) (string, error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.ActorID == "" {
		return "", errors.New("unauthenticated")
	}
	return subject.ActorID, nil
}

func apiError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(billingError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiResource(w http.ResponseWriter, status int, id string, version int64, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(billingResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiAccepted(w http.ResponseWriter, requestID, resourceID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(billingAccepted{
		RequestID:  requestID,
		Status:     "accepted",
		ResourceID: resourceID,
	})
}

func apiCollection(w http.ResponseWriter, items []billingResource) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
	})
}

func mapError(err error) (status int, code string) {
	switch {
	case errors.Is(err, ErrChargeNotFound),
		errors.Is(err, ErrInvoiceNotFound),
		errors.Is(err, ErrCreditNotFound),
		errors.Is(err, ErrSubledgerEntryNotFound),
		errors.Is(err, ErrAccountingExportNotFound),
		errors.Is(err, ErrFinancialApprovalNotFound),
		errors.Is(err, ErrMakerCheckerRequestNotFound),
		errors.Is(err, ErrBankVerificationNotFound),
		errors.Is(err, ErrReconciliationExceptionNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrInvoiceAlreadyIssued):
		return http.StatusConflict, "INVOICE_ALREADY_ISSUED"
	case errors.Is(err, ErrRequestNotPendingApproval),
		errors.Is(err, ErrRequestNotPendingVerification):
		return http.StatusConflict, "CONFLICT"
	case errors.Is(err, ErrDuplicateCharge),
		errors.Is(err, ErrDuplicateCredit):
		return http.StatusConflict, "DUPLICATE"
	case errors.Is(err, ErrSelfApprovalDenied):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, ErrAICannotPost):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, ErrBankVerificationRequired),
		errors.Is(err, ErrBankVerificationExpired):
		return http.StatusUnprocessableEntity, "VERIFICATION_REQUIRED"
	case errors.Is(err, ErrOriginalEntryPreserved),
		errors.Is(err, ErrFloatNotAllowed):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	default:
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	}
}

// ============================================================
// POST /v1/billing/charges
// ============================================================

func (h *Handler) handleCreateCharge(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		Amount         struct {
			MinorUnits int64  `json:"minor_units"`
			Currency   string `json:"currency"`
		} `json:"amount"`
		Reason string          `json:"reason"`
		Data   json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.IdempotencyKey == "" {
		apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idempotency_key is required")
		return
	}

	var dataBytes []byte
	if req.Data != nil {
		dataBytes = req.Data
	}

	chargeData := map[string]any{}
	json.Unmarshal(dataBytes, &chargeData)

	propertyID, _ := chargeData["property_id"].(string)
	chargeType, _ := chargeData["charge_type"].(string)
	contractRuleID, _ := chargeData["contract_rule_id"].(string)
	evidenceID, _ := chargeData["evidence_id"].(string)
	ticketID, _ := chargeData["ticket_id"].(string)
	orderID, _ := chargeData["order_id"].(string)
	approvalID, _ := chargeData["approval_id"].(string)

	created, err := h.svc.CreateCharge(r.Context(), tenantID, CreateChargeParams{
		PropertyID:       propertyID,
		ChargeType:       chargeType,
		AmountMinorUnits: req.Amount.MinorUnits,
		Currency:         req.Amount.Currency,
		Reason:           req.Reason,
		Data:             dataBytes,
		ContractRuleID:   contractRuleID,
		EvidenceID:       evidenceID,
		TicketID:         ticketID,
		OrderID:          orderID,
		ApprovalID:       approvalID,
		IdempotencyKey:   req.IdempotencyKey,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, created.ID, created.Version, chargeView(created))
}

// ============================================================
// POST /v1/billing/invoices
// ============================================================

func (h *Handler) handleIssueInvoice(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Reason         string          `json:"reason"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.IdempotencyKey == "" {
		apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idempotency_key is required")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	propertyID, _ := dataMap["property_id"].(string)
	currency, _ := dataMap["currency"].(string)
	if currency == "" {
		currency = "INR"
	}

	var periodStart, periodEnd *time.Time
	if ps, ok := dataMap["period_start"].(string); ok && ps != "" {
		t, err := time.Parse(time.RFC3339, ps)
		if err == nil {
			periodStart = &t
		}
	}
	if pe, ok := dataMap["period_end"].(string); ok && pe != "" {
		t, err := time.Parse(time.RFC3339, pe)
		if err == nil {
			periodEnd = &t
		}
	}

	var lines []CreateInvoiceLineParams
	if rawLines, ok := dataMap["lines"].([]any); ok {
		for _, raw := range rawLines {
			if lineMap, ok := raw.(map[string]any); ok {
				line := CreateInvoiceLineParams{}
				if v, _ := lineMap["charge_type"].(string); v != "" {
					line.ChargeType = v
				}
				if v, _ := lineMap["description"].(string); v != "" {
					line.Description = v
				}
				if v, ok := lineMap["amount_minor_units"].(float64); ok {
					line.AmountMinorUnits = int64(v)
				}
				if v, _ := lineMap["contract_rule_id"].(string); v != "" {
					line.ContractRuleID = v
				}
				if v, _ := lineMap["ticket_id"].(string); v != "" {
					line.TicketID = v
				}
				if v, _ := lineMap["order_id"].(string); v != "" {
					line.OrderID = v
				}
				if v, _ := lineMap["adjustment_id"].(string); v != "" {
					line.AdjustmentID = v
				}
				lines = append(lines, line)
			}
		}
	}

	invoice, err := h.svc.IssueInvoice(r.Context(), tenantID, CreateInvoiceParams{
		PropertyID:     propertyID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		Currency:       currency,
		IdempotencyKey: req.IdempotencyKey,
		Lines:          lines,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, invoice.ID, invoice.Version, invoiceView(invoice))
}

// ============================================================
// POST /v1/billing/credits
// ============================================================

func (h *Handler) handleIssueCredit(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		Amount         struct {
			MinorUnits int64  `json:"minor_units"`
			Currency   string `json:"currency"`
		} `json:"amount"`
		Reason string          `json:"reason"`
		Data   json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.IdempotencyKey == "" {
		apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idempotency_key is required")
		return
	}

	var dataBytes []byte
	if req.Data != nil {
		dataBytes = req.Data
	}

	creditData := map[string]any{}
	json.Unmarshal(dataBytes, &creditData)

	propertyID, _ := creditData["property_id"].(string)
	creditType, _ := creditData["credit_type"].(string)
	originalEntryID, _ := creditData["original_entry_id"].(string)
	originalEntryType, _ := creditData["original_entry_type"].(string)

	credit, err := h.svc.IssueCredit(r.Context(), tenantID, CreateCreditParams{
		PropertyID:        propertyID,
		CreditType:        creditType,
		AmountMinorUnits:  req.Amount.MinorUnits,
		Currency:          req.Amount.Currency,
		Reason:            req.Reason,
		OriginalEntryID:   originalEntryID,
		OriginalEntryType: originalEntryType,
		Data:              dataBytes,
		IdempotencyKey:    req.IdempotencyKey,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, credit.ID, credit.Version, creditView(credit))
}

// ============================================================
// POST /v1/financial-approvals/{approval_id}/decisions
// ============================================================

func (h *Handler) handleDecideFinancialApproval(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	approvalID := r.PathValue("approval_id")

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Reason         string          `json:"reason"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	decision, _ := dataMap["decision"].(string)
	requestID, _ := dataMap["request_id"].(string)

	approval, err := h.svc.DecideFinancialApproval(r.Context(), tenantID, approvalID, CreateFinancialApprovalParams{
		RequestID:  requestID,
		ApproverID: actorID,
		Decision:   decision,
		Reason:     req.Reason,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, approval.ID, 1, financialApprovalView(approval))
}

// ============================================================
// POST /v1/accounting-exports
// ============================================================

func (h *Handler) handleCreateAccountingExport(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Reason         string          `json:"reason"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	format, _ := dataMap["format"].(string)
	if format == "" {
		format = "journal_csv"
	}

	var periodStart, periodEnd *time.Time
	if ps, ok := dataMap["period_start"].(string); ok && ps != "" {
		t, err := time.Parse(time.RFC3339, ps)
		if err == nil {
			periodStart = &t
		}
	}
	if pe, ok := dataMap["period_end"].(string); ok && pe != "" {
		t, err := time.Parse(time.RFC3339, pe)
		if err == nil {
			periodEnd = &t
		}
	}

	export, err := h.svc.CreateAccountingExport(r.Context(), tenantID, CreateAccountingExportParams{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Format:      format,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiAccepted(w, req.IdempotencyKey, export.ID)
}

// ============================================================
// GET /v1/reports/property-contribution
// ============================================================

func (h *Handler) handleGetPropertyContributionReport(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	report, err := h.svc.GetPropertyContributionReport(r.Context(), tenantID, propertyID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := []billingResource{{
		ID:      propertyID,
		Version: 1,
		Data:    report,
	}}
	apiCollection(w, items)
}

// ============================================================
// View functions
// ============================================================

func chargeView(c *Charge) map[string]any {
	v := map[string]any{
		"id":                 c.ID,
		"tenant_id":          c.TenantID,
		"property_id":        c.PropertyID,
		"charge_type":        c.ChargeType,
		"amount_minor_units": c.AmountMinorUnits,
		"currency":           c.Currency,
		"reason":             c.Reason,
		"data":               json.RawMessage(c.Data),
		"contract_rule_id":   c.ContractRuleID,
		"evidence_id":        c.EvidenceID,
		"ticket_id":          c.TicketID,
		"order_id":           c.OrderID,
		"approval_id":        c.ApprovalID,
		"idempotency_key":    c.IdempotencyKey,
		"status":             c.Status,
		"version":            c.Version,
		"created_at":         c.CreatedAt.Format(time.RFC3339),
		"updated_at":         c.UpdatedAt.Format(time.RFC3339),
	}
	return v
}

func invoiceView(inv *Invoice) map[string]any {
	v := map[string]any{
		"id":                inv.ID,
		"tenant_id":         inv.TenantID,
		"property_id":       inv.PropertyID,
		"total_minor_units": inv.TotalMinorUnits,
		"currency":          inv.Currency,
		"status":            inv.Status,
		"idempotency_key":   inv.IdempotencyKey,
		"version":           inv.Version,
		"created_at":        inv.CreatedAt.Format(time.RFC3339),
		"updated_at":        inv.UpdatedAt.Format(time.RFC3339),
	}
	if inv.PeriodStart != nil {
		v["period_start"] = inv.PeriodStart.Format(time.RFC3339)
	}
	if inv.PeriodEnd != nil {
		v["period_end"] = inv.PeriodEnd.Format(time.RFC3339)
	}
	return v
}

func creditView(c *Credit) map[string]any {
	return map[string]any{
		"id":                  c.ID,
		"tenant_id":           c.TenantID,
		"property_id":         c.PropertyID,
		"credit_type":         c.CreditType,
		"amount_minor_units":  c.AmountMinorUnits,
		"currency":            c.Currency,
		"reason":              c.Reason,
		"original_entry_id":   c.OriginalEntryID,
		"original_entry_type": c.OriginalEntryType,
		"data":                json.RawMessage(c.Data),
		"idempotency_key":     c.IdempotencyKey,
		"status":              c.Status,
		"version":             c.Version,
		"created_at":          c.CreatedAt.Format(time.RFC3339),
		"updated_at":          c.UpdatedAt.Format(time.RFC3339),
	}
}

func financialApprovalView(a *FinancialApproval) map[string]any {
	return map[string]any{
		"id":          a.ID,
		"tenant_id":   a.TenantID,
		"request_id":  a.RequestID,
		"approver_id": a.ApproverID,
		"decision":    a.Decision,
		"reason":      a.Reason,
		"created_at":  a.CreatedAt.Format(time.RFC3339),
	}
}

// ============================================================
// POST /v1/maker-checker/requests
// ============================================================

func (h *Handler) handleCreateMakerCheckerRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	requestType, _ := dataMap["request_type"].(string)
	propertyID, _ := dataMap["property_id"].(string)
	requiresVerification, _ := dataMap["requires_verification"].(bool)

	created, err := h.svc.CreateMakerCheckerRequest(r.Context(), tenantID, CreateMakerCheckerRequestParams{
		RequestType:          requestType,
		PropertyID:           propertyID,
		Payload:              req.Data,
		IdempotencyKey:       req.IdempotencyKey,
		RequiresVerification: requiresVerification,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, created.ID, created.Version, makerCheckerRequestView(created))
}

// ============================================================
// POST /v1/maker-checker/requests/{request_id}/submit
// ============================================================

func (h *Handler) handleSubmitMakerCheckerRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	isAIActor, _ := dataMap["is_ai_actor"].(bool)

	submitted, err := h.svc.SubmitMakerCheckerRequest(r.Context(), tenantID, requestID, SubmitMakerCheckerRequestParams{
		ActorID:   actorID,
		IsAIActor: isAIActor,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, submitted.ID, submitted.Version, makerCheckerRequestView(submitted))
}

// ============================================================
// POST /v1/maker-checker/decisions
// ============================================================

func (h *Handler) handleDecideMakerCheckerRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Reason string          `json:"reason"`
		Data   json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	requestID, _ := dataMap["request_id"].(string)
	decision, _ := dataMap["decision"].(string)
	isAIActor, _ := dataMap["is_ai_actor"].(bool)

	decided, err := h.svc.DecideMakerCheckerRequest(r.Context(), tenantID, requestID, DecideMakerCheckerRequestParams{
		ActorID:   actorID,
		IsAIActor: isAIActor,
		Decision:  decision,
		Reason:    req.Reason,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, decided.ID, decided.Version, makerCheckerRequestView(decided))
}

// ============================================================
// POST /v1/bank-verifications
// ============================================================

func (h *Handler) handleCreateBankVerification(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	requestID, _ := dataMap["request_id"].(string)

	bv, err := h.svc.CreateBankVerification(r.Context(), tenantID, CreateBankVerificationParams{
		RequestID: requestID,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, bv.ID, 1, bankVerificationView(bv))
}

// ============================================================
// POST /v1/bank-verifications/{verification_id}/confirm
// ============================================================

func (h *Handler) handleConfirmBankVerification(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	verificationID := r.PathValue("verification_id")

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	token, _ := dataMap["verification_token"].(string)

	bv, err := h.svc.ConfirmBankVerification(r.Context(), tenantID, verificationID, ConfirmBankVerificationParams{
		Token: token,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, bv.ID, 1, bankVerificationView(bv))
}

// ============================================================
// POST /v1/journal/finalize
// ============================================================

func (h *Handler) handleFinalizeJournal(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	exportID, _ := dataMap["export_id"].(string)
	isAIActor, _ := dataMap["is_ai_actor"].(bool)

	export, err := h.svc.FinalizeJournalPosting(r.Context(), tenantID, FinalizeJournalParams{
		ExportID:  exportID,
		ActorID:   actorID,
		IsAIActor: isAIActor,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, export.ID, export.Version, map[string]any{
		"id":         export.ID,
		"status":     export.Status,
		"format":     export.Format,
		"created_at": export.CreatedAt.Format(time.RFC3339),
	})
}

// ============================================================
// POST /v1/reconciliation-exceptions
// ============================================================

func (h *Handler) handleRecordReconciliationException(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Reason string          `json:"reason"`
		Data   json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var dataMap map[string]any
	if req.Data != nil {
		json.Unmarshal(req.Data, &dataMap)
	}
	if dataMap == nil {
		dataMap = map[string]any{}
	}

	propertyID, _ := dataMap["property_id"].(string)
	entryID, _ := dataMap["entry_id"].(string)
	entryType, _ := dataMap["entry_type"].(string)
	exceptionType, _ := dataMap["exception_type"].(string)
	description, _ := dataMap["description"].(string)

	created, err := h.svc.RecordReconciliationException(r.Context(), tenantID, CreateReconciliationExceptionParams{
		PropertyID:    propertyID,
		EntryID:       entryID,
		EntryType:     entryType,
		ExceptionType: exceptionType,
		Description:   description,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, created.ID, 1, reconciliationExceptionView(created))
}

// ============================================================
// GET /v1/reconciliation-exceptions
// ============================================================

func (h *Handler) handleListReconciliationExceptions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	exceptions, err := h.svc.ListReconciliationExceptions(r.Context(), tenantID, propertyID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var items []billingResource
	for _, e := range exceptions {
		items = append(items, billingResource{
			ID:      e.ID,
			Version: 1,
			Data:    reconciliationExceptionView(&e),
		})
	}
	apiCollection(w, items)
}

// ============================================================
// POST /v1/reconciliation-exceptions/{exception_id}/resolve
// ============================================================

func (h *Handler) handleResolveReconciliationException(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	exceptionID := r.PathValue("exception_id")

	resolved, err := h.svc.ResolveReconciliationException(r.Context(), tenantID, exceptionID, ResolveReconciliationExceptionParams{
		ActorID: actorID,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, resolved.ID, 1, reconciliationExceptionView(resolved))
}

// ============================================================
// View helpers
// ============================================================

func makerCheckerRequestView(rr *MakerCheckerRequest) map[string]any {
	v := map[string]any{
		"id":                    rr.ID,
		"tenant_id":             rr.TenantID,
		"request_type":          rr.RequestType,
		"property_id":           rr.PropertyID,
		"status":                rr.Status,
		"created_by":            rr.CreatedBy,
		"submitted_by":          rr.SubmittedBy,
		"approved_by":           rr.ApprovedBy,
		"rejected_by":           rr.RejectedBy,
		"idempotency_key":       rr.IdempotencyKey,
		"requires_verification": rr.RequiresVerification,
		"version":               rr.Version,
		"created_at":            rr.CreatedAt.Format(time.RFC3339),
		"updated_at":            rr.UpdatedAt.Format(time.RFC3339),
	}
	if len(rr.Payload) > 0 {
		v["payload"] = json.RawMessage(rr.Payload)
	}
	return v
}

func bankVerificationView(bv *BankVerification) map[string]any {
	v := map[string]any{
		"id":     bv.ID,
		"status": bv.Status,
	}
	return v
}

func reconciliationExceptionView(re *ReconciliationException) map[string]any {
	v := map[string]any{
		"id":             re.ID,
		"tenant_id":      re.TenantID,
		"property_id":    re.PropertyID,
		"entry_id":       re.EntryID,
		"entry_type":     re.EntryType,
		"exception_type": re.ExceptionType,
		"description":    re.Description,
		"status":         re.Status,
		"recorded_by":    re.RecordedBy,
		"created_at":     re.CreatedAt.Format(time.RFC3339),
	}
	if re.ResolvedBy != "" {
		v["resolved_by"] = re.ResolvedBy
	}
	if re.ResolvedAt != nil {
		v["resolved_at"] = re.ResolvedAt.Format(time.RFC3339)
	}
	return v
}
