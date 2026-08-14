package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/documents"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/property"
)

type FinanceSliceHandler struct {
	billingSvc   *billing.Service
	documentsSvc *documents.Service
	propSvc      *property.PropertyService
	authorityFn  OwnerAuthorities
}

func NewFinanceSliceHandler(billingSvc *billing.Service, documentsSvc *documents.Service, propSvc *property.PropertyService, authorityFn OwnerAuthorities) *FinanceSliceHandler {
	return &FinanceSliceHandler{
		billingSvc:   billingSvc,
		documentsSvc: documentsSvc,
		propSvc:      propSvc,
		authorityFn:  authorityFn,
	}
}

func (h *FinanceSliceHandler) RegisterRoutes(mux *http.ServeMux) {
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

	mux.HandleFunc("POST /v1/documents", h.handleCreateDocument)
	mux.HandleFunc("POST /v1/documents/{document_id}/versions", h.handleCreateDocumentVersion)
	mux.HandleFunc("POST /v1/documents/{document_id}/reviews", h.handleReviewDocument)
	mux.HandleFunc("POST /v1/submission-packets/{packet_id}/confirmations", h.handleConfirmSubmissionPacket)
	mux.HandleFunc("GET /v1/documents/{document_id}", h.handleGetDocument)
	mux.HandleFunc("GET /v1/properties/{property_id}/documents", h.handleListDocuments)
	mux.HandleFunc("GET /v1/documents/{document_id}/versions", h.handleListDocumentVersions)
	mux.HandleFunc("GET /v1/document-versions/{version_id}/extractions", h.handleListExtractions)
	mux.HandleFunc("GET /v1/documents/{document_id}/reviews", h.handleListDocumentReviews)
	mux.HandleFunc("POST /v1/document-versions/{version_id}/extractions", h.handleCreateExtraction)
	mux.HandleFunc("POST /v1/properties/{property_id}/submission-packets", h.handleCreateSubmissionPacket)
	mux.HandleFunc("GET /v1/submission-packets/{packet_id}", h.handleGetSubmissionPacket)
	mux.HandleFunc("GET /v1/submission-packets/{packet_id}/receipt", h.handleGetReceipt)
	mux.HandleFunc("POST /v1/properties/{property_id}/documents/expiry-check", h.handleCheckExpiry)
}

func (h *FinanceSliceHandler) finSubject(r *http.Request) (security.Subject, error) {
	return subjectFromRequest(r)
}

func (h *FinanceSliceHandler) finSubjectActor(r *http.Request) (string, error) {
	subject, err := h.finSubject(r)
	if err != nil {
		return "", err
	}
	if subject.ActorID == "" {
		return "", errors.New("unauthenticated")
	}
	return subject.ActorID, nil
}

func (h *FinanceSliceHandler) scopeOK(ctx context.Context, subject security.Subject, propertyID string) bool {
	if propertyID == "" {
		return false
	}
	if !IsOwner(subject) {
		return true
	}
	if h.authorityFn == nil {
		return false
	}

	authority := propertyID
	if h.propSvc != nil {
		prop, err := h.propSvc.GetProperty(ctx, subject.TenantID, propertyID)
		if err != nil {
			return false
		}
		authority = prop.OwnerAuthorityID
	}

	for _, a := range h.authorityFn(subject.ActorID) {
		if a == authority {
			return true
		}
	}
	return false
}

func mapBillingError(err error) (int, string) {
	switch {
	case errors.Is(err, billing.ErrChargeNotFound),
		errors.Is(err, billing.ErrInvoiceNotFound),
		errors.Is(err, billing.ErrCreditNotFound),
		errors.Is(err, billing.ErrSubledgerEntryNotFound),
		errors.Is(err, billing.ErrAccountingExportNotFound),
		errors.Is(err, billing.ErrFinancialApprovalNotFound),
		errors.Is(err, billing.ErrMakerCheckerRequestNotFound),
		errors.Is(err, billing.ErrBankVerificationNotFound),
		errors.Is(err, billing.ErrReconciliationExceptionNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, billing.ErrInvoiceAlreadyIssued):
		return http.StatusConflict, "INVOICE_ALREADY_ISSUED"
	case errors.Is(err, billing.ErrRequestNotPendingApproval),
		errors.Is(err, billing.ErrRequestNotPendingVerification):
		return http.StatusConflict, "CONFLICT"
	case errors.Is(err, billing.ErrDuplicateCharge),
		errors.Is(err, billing.ErrDuplicateCredit):
		return http.StatusConflict, "DUPLICATE"
	case errors.Is(err, billing.ErrSelfApprovalDenied):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, billing.ErrAICannotPost):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, billing.ErrBankVerificationRequired),
		errors.Is(err, billing.ErrBankVerificationExpired):
		return http.StatusUnprocessableEntity, "VERIFICATION_REQUIRED"
	case errors.Is(err, billing.ErrOriginalEntryPreserved),
		errors.Is(err, billing.ErrFloatNotAllowed),
		errors.Is(err, billing.ErrNegativeAmount),
		errors.Is(err, billing.ErrInvalidCharge),
		errors.Is(err, billing.ErrInvalidInvoice),
		errors.Is(err, billing.ErrInvalidCredit),
		errors.Is(err, billing.ErrInvalidFinancialApproval),
		errors.Is(err, billing.ErrInvalidAccountingExport),
		errors.Is(err, billing.ErrInvalidMakerCheckerRequest),
		errors.Is(err, billing.ErrInvalidReconciliationException):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}

func mapDocumentError(err error) (int, string) {
	switch {
	case errors.Is(err, documents.ErrDocumentNotFound),
		errors.Is(err, documents.ErrDocumentVersionNotFound),
		errors.Is(err, documents.ErrReviewNotFound),
		errors.Is(err, documents.ErrExtractionNotFound),
		errors.Is(err, documents.ErrSubmissionPacketNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, documents.ErrVersionNotModifiable),
		errors.Is(err, documents.ErrDuplicateVersion),
		errors.Is(err, documents.ErrPacketAlreadySubmitted),
		errors.Is(err, documents.ErrReviewAlreadyCompleted):
		return http.StatusConflict, "CONFLICT"
	case errors.Is(err, documents.ErrDocumentExpired),
		errors.Is(err, documents.ErrPacketNotComplete),
		errors.Is(err, documents.ErrHumanReviewRequired):
		return http.StatusUnprocessableEntity, "INVALID_STATE"
	case errors.Is(err, documents.ErrAICannotCertify),
		errors.Is(err, documents.ErrSignatureAuthorityMissing):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, documents.ErrInvalidDocument),
		errors.Is(err, documents.ErrInvalidVersion),
		errors.Is(err, documents.ErrInvalidExtraction),
		errors.Is(err, documents.ErrInvalidReview),
		errors.Is(err, documents.ErrInvalidSubmissionPacket),
		errors.Is(err, documents.ErrInvalidSubmissionRequest):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}

func chargeAPIView(c *billing.Charge) map[string]any {
	view := ChargeTraceabilityView(*c)
	view["reason"] = c.Reason
	view["idempotency_key"] = c.IdempotencyKey
	view["updated_at"] = c.UpdatedAt.Format(time.RFC3339)
	return view
}

func invoiceAPIView(inv *billing.Invoice, lines []billing.InvoiceLine) map[string]any {
	view := InvoiceTraceabilityView(*inv, lines)
	view["idempotency_key"] = inv.IdempotencyKey
	view["updated_at"] = inv.UpdatedAt.Format(time.RFC3339)
	if inv.PeriodStart != nil {
		view["period_start"] = inv.PeriodStart.Format(time.RFC3339)
	}
	if inv.PeriodEnd != nil {
		view["period_end"] = inv.PeriodEnd.Format(time.RFC3339)
	}
	return view
}

func creditAPIView(c *billing.Credit) map[string]any {
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
		"idempotency_key":     c.IdempotencyKey,
		"status":              c.Status,
		"version":             c.Version,
		"evidence_link":       CreditTrace(*c),
		"created_at":          c.CreatedAt.Format(time.RFC3339),
		"updated_at":          c.UpdatedAt.Format(time.RFC3339),
	}
}

func financialApprovalAPIView(a *billing.FinancialApproval) map[string]any {
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

func makerCheckerRequestAPIView(rr *billing.MakerCheckerRequest) map[string]any {
	return map[string]any{
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
}

func bankVerificationAPIView(bv *billing.BankVerification) map[string]any {
	return map[string]any{
		"id":     bv.ID,
		"status": bv.Status,
	}
}

func reconciliationExceptionAPIView(re *billing.ReconciliationException) map[string]any {
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

func documentAPIView(d *documents.Document) map[string]any {
	m := map[string]any{
		"id":              d.ID,
		"tenant_id":       d.TenantID,
		"property_id":     d.PropertyID,
		"title":           d.Title,
		"document_type":   d.DocumentType,
		"status":          d.Status,
		"current_version": d.CurrentVersion,
		"version":         d.Version,
		"created_at":      d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":      d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if d.ExpiresAt != nil {
		m["expires_at"] = d.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return m
}

func documentVersionAPIView(v *documents.DocumentVersion) map[string]any {
	return map[string]any{
		"id":             v.ID,
		"document_id":    v.DocumentID,
		"tenant_id":      v.TenantID,
		"version_number": v.VersionNumber,
		"content_hash":   v.ContentHash,
		"object_key":     v.ObjectKey,
		"filename":       v.Filename,
		"content_type":   v.ContentType,
		"size_bytes":     v.SizeBytes,
		"uploaded_by":    v.UploadedBy,
		"metadata":       v.Metadata,
		"created_at":     v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func extractionAPIView(e *documents.Extraction) map[string]any {
	return map[string]any{
		"id":                  e.ID,
		"document_version_id": e.DocumentVersionID,
		"tenant_id":           e.TenantID,
		"field_name":          e.FieldName,
		"field_value":         e.FieldValue,
		"field_category":      e.FieldCategory,
		"source_location":     e.SourceLocation,
		"confidence":          e.Confidence,
		"confidence_score":    e.ConfidenceScore,
		"extracted_by":        e.ExtractedBy,
		"extracted_at":        e.ExtractedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func documentReviewAPIView(r *documents.Review) map[string]any {
	return map[string]any{
		"id":                  r.ID,
		"document_id":         r.DocumentID,
		"document_version_id": r.DocumentVersionID,
		"tenant_id":           r.TenantID,
		"reviewer_id":         r.ReviewerID,
		"status":              r.Status,
		"decision":            r.Decision,
		"comments":            r.Comments,
		"reviewed_at":         r.ReviewedAt.Format("2006-01-02T15:04:05Z07:00"),
		"created_at":          r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func packetAPIView(p *documents.SubmissionPacket) map[string]any {
	m := map[string]any{
		"id":           p.ID,
		"tenant_id":    p.TenantID,
		"property_id":  p.PropertyID,
		"status":       p.Status,
		"document_ids": p.DocumentIDs,
		"created_by":   p.CreatedBy,
		"version":      p.Version,
		"created_at":   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.SubmittedAt != nil {
		m["submitted_at"] = p.SubmittedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return m
}

func receiptAPIView(r *documents.SubmissionReceipt) map[string]any {
	refs := make([]map[string]any, 0, len(r.DocumentVersionRefs))
	for _, ref := range r.DocumentVersionRefs {
		refs = append(refs, map[string]any{
			"document_id":         ref.DocumentID,
			"document_version_id": ref.DocumentVersionID,
			"version_number":      ref.VersionNumber,
			"content_hash":        ref.ContentHash,
		})
	}
	return map[string]any{
		"id":                    r.ID,
		"packet_id":             r.PacketID,
		"tenant_id":             r.TenantID,
		"confirmed_by":          r.ConfirmedBy,
		"receipt_hash":          r.ReceiptHash,
		"document_version_refs": refs,
		"confirmed_at":          r.ConfirmedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ----------------------------------------------------------------
// Billing handlers
// ----------------------------------------------------------------

func (h *FinanceSliceHandler) handleCreateCharge(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	if !hasRole(subject.Roles, RoleOwner) && !hasRole(subject.Roles, "staff") {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "creating charges requires owner or staff role")
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
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.IdempotencyKey == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idempotency_key is required")
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

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	created, err := h.billingSvc.CreateCharge(r.Context(), subject.TenantID, billing.CreateChargeParams{
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
	}, subject.ActorID)
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	view := chargeAPIView(created)
	writeETag(w, int(created.Version))
	writeResource(w, http.StatusCreated, created.ID, int(created.Version), view)
}

func (h *FinanceSliceHandler) handleIssueInvoice(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	if !hasRole(subject.Roles, RoleOwner) && !hasRole(subject.Roles, "staff") {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "issuing invoices requires owner or staff role")
		return
	}

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Reason         string          `json:"reason"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.IdempotencyKey == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idempotency_key is required")
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

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	var lines []billing.CreateInvoiceLineParams
	if rawLines, ok := dataMap["lines"].([]any); ok {
		for _, raw := range rawLines {
			if lineMap, ok := raw.(map[string]any); ok {
				line := billing.CreateInvoiceLineParams{}
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

	invoice, err := h.billingSvc.IssueInvoice(r.Context(), subject.TenantID, billing.CreateInvoiceParams{
		PropertyID:     propertyID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		Currency:       currency,
		IdempotencyKey: req.IdempotencyKey,
		Lines:          lines,
	}, subject.ActorID)
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	invoiceLines, _ := h.billingSvc.GetInvoiceLines(r.Context(), subject.TenantID, invoice.ID)
	if invoiceLines == nil {
		invoiceLines = []billing.InvoiceLine{}
	}
	view := invoiceAPIView(invoice, invoiceLines)
	writeETag(w, int(invoice.Version))
	writeResource(w, http.StatusCreated, invoice.ID, int(invoice.Version), view)
}

func (h *FinanceSliceHandler) handleIssueCredit(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	if !hasRole(subject.Roles, RoleOwner) && !hasRole(subject.Roles, "staff") {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "issuing credits requires owner or staff role")
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
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.IdempotencyKey == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "idempotency_key is required")
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

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	created, err := h.billingSvc.IssueCredit(r.Context(), subject.TenantID, billing.CreateCreditParams{
		PropertyID:        propertyID,
		CreditType:        creditType,
		AmountMinorUnits:  req.Amount.MinorUnits,
		Currency:          req.Amount.Currency,
		Reason:            req.Reason,
		OriginalEntryID:   originalEntryID,
		OriginalEntryType: originalEntryType,
		Data:              dataBytes,
		IdempotencyKey:    req.IdempotencyKey,
	}, subject.ActorID)
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	view := creditAPIView(created)
	writeETag(w, int(created.Version))
	writeResource(w, http.StatusCreated, created.ID, int(created.Version), view)
}

func (h *FinanceSliceHandler) handleDecideFinancialApproval(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	actorID, err := h.finSubjectActor(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
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
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	approval, err := h.billingSvc.DecideFinancialApproval(r.Context(), subject.TenantID, approvalID, billing.CreateFinancialApprovalParams{
		RequestID:  requestID,
		ApproverID: actorID,
		Decision:   decision,
		Reason:     req.Reason,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusOK, approval.ID, 1, financialApprovalAPIView(approval))
}

func (h *FinanceSliceHandler) handleCreateAccountingExport(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Reason         string          `json:"reason"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	export, err := h.billingSvc.CreateAccountingExport(r.Context(), subject.TenantID, billing.CreateAccountingExportParams{
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Format:      format,
	}, subject.ActorID)
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"request_id":  req.IdempotencyKey,
		"status":      "accepted",
		"resource_id": export.ID,
	})
}

func (h *FinanceSliceHandler) handleGetPropertyContributionReport(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	report, err := h.billingSvc.GetPropertyContributionReport(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := []Resource{{ID: propertyID, Version: 1, Data: report}}
	writeCollection(w, items, nil)
}

func (h *FinanceSliceHandler) handleCreateMakerCheckerRequest(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		IdempotencyKey string          `json:"idempotency_key"`
		Data           json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	if propertyID != "" && !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	created, err := h.billingSvc.CreateMakerCheckerRequest(r.Context(), subject.TenantID, billing.CreateMakerCheckerRequestParams{
		RequestType:          requestType,
		PropertyID:           propertyID,
		Payload:              req.Data,
		IdempotencyKey:       req.IdempotencyKey,
		RequiresVerification: requiresVerification,
	}, subject.ActorID)
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeETag(w, int(created.Version))
	writeResource(w, http.StatusCreated, created.ID, int(created.Version), makerCheckerRequestAPIView(created))
}

func (h *FinanceSliceHandler) handleSubmitMakerCheckerRequest(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	actorID, err := h.finSubjectActor(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	submitted, err := h.billingSvc.SubmitMakerCheckerRequest(r.Context(), subject.TenantID, requestID, billing.SubmitMakerCheckerRequestParams{
		ActorID:   actorID,
		IsAIActor: isAIActor,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeETag(w, int(submitted.Version))
	writeResource(w, http.StatusOK, submitted.ID, int(submitted.Version), makerCheckerRequestAPIView(submitted))
}

func (h *FinanceSliceHandler) handleDecideMakerCheckerRequest(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	actorID, err := h.finSubjectActor(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Reason string          `json:"reason"`
		Data   json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	decided, err := h.billingSvc.DecideMakerCheckerRequest(r.Context(), subject.TenantID, requestID, billing.DecideMakerCheckerRequestParams{
		ActorID:   actorID,
		IsAIActor: isAIActor,
		Decision:  decision,
		Reason:    req.Reason,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeETag(w, int(decided.Version))
	writeResource(w, http.StatusOK, decided.ID, int(decided.Version), makerCheckerRequestAPIView(decided))
}

func (h *FinanceSliceHandler) handleCreateBankVerification(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	bv, err := h.billingSvc.CreateBankVerification(r.Context(), subject.TenantID, billing.CreateBankVerificationParams{
		RequestID: requestID,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusCreated, bv.ID, 1, bankVerificationAPIView(bv))
}

func (h *FinanceSliceHandler) handleConfirmBankVerification(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	verificationID := r.PathValue("verification_id")

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	bv, err := h.billingSvc.ConfirmBankVerification(r.Context(), subject.TenantID, verificationID, billing.ConfirmBankVerificationParams{
		Token: token,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusOK, bv.ID, 1, bankVerificationAPIView(bv))
}

func (h *FinanceSliceHandler) handleFinalizeJournal(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	actorID, err := h.finSubjectActor(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Data json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	export, err := h.billingSvc.FinalizeJournalPosting(r.Context(), subject.TenantID, billing.FinalizeJournalParams{
		ExportID:  exportID,
		ActorID:   actorID,
		IsAIActor: isAIActor,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusOK, export.ID, int(export.Version), map[string]any{
		"id":         export.ID,
		"status":     export.Status,
		"format":     export.Format,
		"created_at": export.CreatedAt.Format(time.RFC3339),
	})
}

func (h *FinanceSliceHandler) handleRecordReconciliationException(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Reason string          `json:"reason"`
		Data   json.RawMessage `json:"data"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	if propertyID != "" && !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	created, err := h.billingSvc.RecordReconciliationException(r.Context(), subject.TenantID, billing.CreateReconciliationExceptionParams{
		PropertyID:    propertyID,
		EntryID:       entryID,
		EntryType:     entryType,
		ExceptionType: exceptionType,
		Description:   description,
	}, subject.ActorID)
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusCreated, created.ID, 1, reconciliationExceptionAPIView(created))
}

func (h *FinanceSliceHandler) handleListReconciliationExceptions(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	if propertyID != "" && !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	exceptions, err := h.billingSvc.ListReconciliationExceptions(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var items []Resource
	for _, e := range exceptions {
		items = append(items, Resource{ID: e.ID, Version: 1, Data: reconciliationExceptionAPIView(&e)})
	}
	writeCollection(w, items, nil)
}

func (h *FinanceSliceHandler) handleResolveReconciliationException(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	actorID, err := h.finSubjectActor(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	exceptionID := r.PathValue("exception_id")

	resolved, err := h.billingSvc.ResolveReconciliationException(r.Context(), subject.TenantID, exceptionID, billing.ResolveReconciliationExceptionParams{
		ActorID: actorID,
	})
	if err != nil {
		status, code := mapBillingError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusOK, resolved.ID, 1, reconciliationExceptionAPIView(resolved))
}

// ----------------------------------------------------------------
// Document handlers
// ----------------------------------------------------------------

func (h *FinanceSliceHandler) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		Title        string `json:"title"`
		DocumentType string `json:"document_type"`
		PropertyID   string `json:"property_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if !h.scopeOK(r.Context(), subject, req.PropertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	doc, err := h.documentsSvc.CreateDocument(r.Context(), subject.TenantID, documents.CreateDocumentParams{
		Title:        req.Title,
		DocumentType: req.DocumentType,
		PropertyID:   req.PropertyID,
	}, subject.ActorID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeETag(w, doc.Version)
	writeResource(w, http.StatusCreated, doc.ID, doc.Version, documentAPIView(doc))
}

func (h *FinanceSliceHandler) handleCreateDocumentVersion(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		ContentHash string `json:"content_hash"`
		ObjectKey   string `json:"object_key"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		SizeBytes   int64  `json:"size_bytes"`
		Metadata    string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	ver, doc, err := h.documentsSvc.CreateVersion(r.Context(), subject.TenantID, documentID, documents.CreateVersionParams{
		ContentHash: req.ContentHash,
		ObjectKey:   req.ObjectKey,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		Metadata:    req.Metadata,
	}, subject.ActorID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeETag(w, ver.VersionNumber)
	writeResource(w, http.StatusCreated, ver.ID, ver.VersionNumber, map[string]any{
		"version":  documentVersionAPIView(ver),
		"document": documentAPIView(doc),
	})
}

func (h *FinanceSliceHandler) handleReviewDocument(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		Status   string `json:"status"`
		Decision string `json:"decision"`
		Comments string `json:"comments"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	review, err := h.documentsSvc.ReviewDocument(r.Context(), subject.TenantID, documentID, documents.CreateReviewParams{
		Status:   req.Status,
		Decision: req.Decision,
		Comments: req.Comments,
	}, subject.ActorID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusOK, review.ID, 1, documentReviewAPIView(review))
}

func (h *FinanceSliceHandler) handleConfirmSubmissionPacket(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	packetID := r.PathValue("packet_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		ReviewerAuth string `json:"reviewer_auth"`
	}
	json.Unmarshal(body, &req)

	receipt, packet, err := h.documentsSvc.ConfirmSubmission(r.Context(), subject.TenantID, packetID, subject.ActorID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"packet":  packetAPIView(packet),
		"receipt": receiptAPIView(receipt),
	})
}

func (h *FinanceSliceHandler) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")
	doc, err := h.documentsSvc.GetDocument(r.Context(), subject.TenantID, documentID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	if !h.scopeOK(r.Context(), subject, doc.PropertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "document not found")
		return
	}

	writeETag(w, doc.Version)
	writeResource(w, http.StatusOK, doc.ID, doc.Version, documentAPIView(doc))
}

func (h *FinanceSliceHandler) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	docs, err := h.documentsSvc.ListDocuments(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if docs == nil {
		docs = []documents.Document{}
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, parseErr := strconv.Atoi(l); parseErr == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	start := 0
	if cursor != "" {
		for i, d := range docs {
			if d.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(docs) {
		end = len(docs)
	}

	var items []Resource
	for _, d := range docs[start:end] {
		items = append(items, Resource{ID: d.ID, Version: d.Version, Data: documentAPIView(&d)})
	}
	var nextCursor *string
	if end < len(docs) {
		c := docs[end].ID
		nextCursor = &c
	}

	writeCollection(w, items, nextCursor)
}

func (h *FinanceSliceHandler) handleListDocumentVersions(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")
	versions, err := h.documentsSvc.ListVersions(r.Context(), subject.TenantID, documentID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if versions == nil {
		versions = []documents.DocumentVersion{}
	}

	var items []Resource
	for _, v := range versions {
		items = append(items, Resource{ID: v.ID, Version: v.VersionNumber, Data: documentVersionAPIView(&v)})
	}
	writeCollection(w, items, nil)
}

func (h *FinanceSliceHandler) handleListExtractions(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	versionID := r.PathValue("version_id")
	extractions, err := h.documentsSvc.ListExtractions(r.Context(), subject.TenantID, versionID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if extractions == nil {
		extractions = []documents.Extraction{}
	}

	var items []Resource
	for _, e := range extractions {
		items = append(items, Resource{ID: e.ID, Version: 1, Data: extractionAPIView(&e)})
	}
	writeCollection(w, items, nil)
}

func (h *FinanceSliceHandler) handleListDocumentReviews(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")
	reviews, err := h.documentsSvc.ListReviews(r.Context(), subject.TenantID, documentID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if reviews == nil {
		reviews = []documents.Review{}
	}

	var items []Resource
	for _, rv := range reviews {
		items = append(items, Resource{ID: rv.ID, Version: 1, Data: documentReviewAPIView(&rv)})
	}
	writeCollection(w, items, nil)
}

func (h *FinanceSliceHandler) handleCreateExtraction(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	versionID := r.PathValue("version_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		FieldName       string  `json:"field_name"`
		FieldValue      string  `json:"field_value"`
		FieldCategory   string  `json:"field_category"`
		SourceLocation  string  `json:"source_location"`
		Confidence      string  `json:"confidence"`
		ConfidenceScore float64 `json:"confidence_score"`
		ExtractedBy     string  `json:"extracted_by"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	ext, err := h.documentsSvc.CreateExtraction(r.Context(), subject.TenantID, versionID, documents.CreateExtractionParams{
		FieldName:       req.FieldName,
		FieldValue:      req.FieldValue,
		FieldCategory:   req.FieldCategory,
		SourceLocation:  req.SourceLocation,
		Confidence:      req.Confidence,
		ConfidenceScore: req.ConfidenceScore,
		ExtractedBy:     req.ExtractedBy,
	}, subject.ActorID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusCreated, ext.ID, 1, extractionAPIView(ext))
}

func (h *FinanceSliceHandler) handleCreateSubmissionPacket(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		DocumentIDs []string `json:"document_ids"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	packet, err := h.documentsSvc.CreateSubmissionPacket(r.Context(), subject.TenantID, documents.CreateSubmissionPacketParams{
		PropertyID:  propertyID,
		DocumentIDs: req.DocumentIDs,
	}, subject.ActorID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	writeETag(w, packet.Version)
	writeResource(w, http.StatusCreated, packet.ID, packet.Version, packetAPIView(packet))
}

func (h *FinanceSliceHandler) handleGetSubmissionPacket(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	packetID := r.PathValue("packet_id")
	packet, err := h.documentsSvc.GetSubmissionPacket(r.Context(), subject.TenantID, packetID)
	if err != nil {
		status, code := mapDocumentError(err)
		writeError(w, r, status, code, err.Error())
		return
	}

	if !h.scopeOK(r.Context(), subject, packet.PropertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "submission packet not found")
		return
	}

	writeETag(w, packet.Version)
	writeResource(w, http.StatusOK, packet.ID, packet.Version, packetAPIView(packet))
}

func (h *FinanceSliceHandler) handleGetReceipt(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	packetID := r.PathValue("packet_id")
	receipt, err := h.documentsSvc.GetReceipt(r.Context(), subject.TenantID, packetID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if receipt == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "no receipt found for this packet")
		return
	}

	writeResource(w, http.StatusOK, receipt.ID, 1, receiptAPIView(receipt))
}

func (h *FinanceSliceHandler) handleCheckExpiry(w http.ResponseWriter, r *http.Request) {
	subject, err := h.finSubject(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	if !h.scopeOK(r.Context(), subject, propertyID) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	expired, err := h.documentsSvc.DetectExpiry(r.Context(), subject.TenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	nearing, err := h.documentsSvc.FindNearingExpiry(r.Context(), subject.TenantID, 30)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	expiredItems := make([]map[string]any, 0, len(expired))
	for i := range expired {
		expiredItems = append(expiredItems, documentAPIView(&expired[i]))
	}
	nearingItems := make([]map[string]any, 0, len(nearing))
	for i := range nearing {
		nearingItems = append(nearingItems, documentAPIView(&nearing[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"expired":        expiredItems,
		"expired_count":  len(expiredItems),
		"nearing_expiry": nearingItems,
		"nearing_count":  len(nearingItems),
	})
}
