package maintenance

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
	mux.HandleFunc("POST /v1/maintenance/requests", h.handleCreateRequest)
	mux.HandleFunc("GET /v1/maintenance/requests", h.handleListRequests)
	mux.HandleFunc("GET /v1/maintenance/requests/{request_id}", h.handleGetRequest)
	mux.HandleFunc("POST /v1/maintenance/requests/{request_id}/triage", h.handleTriageRequest)
	mux.HandleFunc("POST /v1/maintenance/requests/{request_id}/estimates", h.handleCreateEstimate)
	mux.HandleFunc("GET /v1/maintenance/requests/{request_id}/estimates", h.handleListEstimates)
	mux.HandleFunc("GET /v1/maintenance/requests/{request_id}/approvals", h.handleListApprovals)
	mux.HandleFunc("POST /v1/maintenance/requests/{request_id}/approvals", h.handleDecideRequestEstimate)

	mux.HandleFunc("POST /v1/maintenance/estimates/{estimate_id}/submit", h.handleSubmitEstimate)
	mux.HandleFunc("POST /v1/maintenance/estimates/{estimate_id}/decide", h.handleDecideEstimate)

	mux.HandleFunc("POST /v1/maintenance/requests/{request_id}/work-orders", h.handleAssignWorkOrder)
	mux.HandleFunc("GET /v1/maintenance/work-orders", h.handleListWorkOrders)
	mux.HandleFunc("GET /v1/maintenance/work-orders/{work_order_id}", h.handleGetWorkOrder)
	mux.HandleFunc("POST /v1/maintenance/work-orders/{work_order_id}/start", h.handleStartWorkOrder)
	mux.HandleFunc("POST /v1/maintenance/work-orders/{work_order_id}/complete", h.handleCompleteWorkOrder)
	mux.HandleFunc("POST /v1/maintenance/work-orders/{work_order_id}/verify", h.handleVerifyWorkOrder)
	mux.HandleFunc("POST /v1/maintenance/work-orders/{work_order_id}/warranty", h.handleRecordWarranty)

	mux.HandleFunc("GET /v1/maintenance/vendor/work-orders", h.handleListVendorWorkOrders)
	mux.HandleFunc("GET /v1/maintenance/vendor/work-orders/{work_order_id}", h.handleGetVendorWorkOrder)

	mux.HandleFunc("GET /v1/maintenance/warranties", h.handleListWarranties)
	mux.HandleFunc("GET /v1/maintenance/warranties/{warranty_id}", h.handleGetWarranty)
}

type maintenanceResource struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Data    any    `json:"data"`
}

type maintenanceError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func apiError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(maintenanceError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiResource(w http.ResponseWriter, status int, id string, version int64, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(maintenanceResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiCollection(w http.ResponseWriter, items []maintenanceResource) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

func mapError(err error) (status int, code string) {
	switch {
	case errors.Is(err, ErrRequestNotFound),
		errors.Is(err, ErrEstimateNotFound),
		errors.Is(err, ErrWorkOrderNotFound),
		errors.Is(err, ErrWarrantyNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrSelfVerificationDenied),
		errors.Is(err, ErrIndependentVerificationNeeded),
		errors.Is(err, ErrVendorScopeDenied),
		errors.Is(err, ErrAICannotApprove),
		errors.Is(err, ErrSelfApprovalDenied):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, ErrEstimateNotApproved):
		return http.StatusConflict, "ESTIMATE_NOT_APPROVED"
	case errors.Is(err, ErrCompletionEvidenceRequired):
		return http.StatusUnprocessableEntity, "EVIDENCE_REQUIRED"
	case errors.Is(err, ErrRequestNotApproved):
		return http.StatusConflict, "REQUEST_NOT_APPROVED"
	default:
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	}
}

func (h *Handler) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		PropertyID string `json:"property_id"`
		Title      string `json:"title"`
		Category   string `json:"category"`
		Priority   string `json:"priority"`
		RiskLevel  string `json:"risk_level"`
		Notes      string `json:"notes"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	created, err := h.svc.CreateRequest(r.Context(), tenantID, CreateRequestParams{
		PropertyID: req.PropertyID,
		Title:      req.Title,
		Category:   req.Category,
		Priority:   req.Priority,
		RiskLevel:  req.RiskLevel,
		Notes:      req.Notes,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, created.ID, created.Version, requestView(created))
}

func (h *Handler) handleListRequests(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	requests, err := h.svc.ListRequests(r.Context(), tenantID, propertyID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]maintenanceResource, 0, len(requests))
	for i := range requests {
		items = append(items, maintenanceResource{
			ID:      requests[i].ID,
			Version: requests[i].Version,
			Data:    requestView(&requests[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")
	req, err := h.svc.GetRequest(r.Context(), tenantID, requestID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, req.ID, req.Version, requestView(req))
}

func (h *Handler) handleTriageRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")

	var req struct {
		Category  string `json:"category"`
		Priority  string `json:"priority"`
		RiskLevel string `json:"risk_level"`
		Notes     string `json:"notes"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	triaged, err := h.svc.TriageRequest(r.Context(), tenantID, requestID, TriageRequestParams{
		Category:  req.Category,
		Priority:  req.Priority,
		RiskLevel: req.RiskLevel,
		Notes:     req.Notes,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, triaged.ID, triaged.Version, requestView(triaged))
}

func (h *Handler) handleCreateEstimate(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")

	var req struct {
		AmountMinorUnits int64  `json:"amount_minor_units"`
		Currency         string `json:"currency"`
		Scope            string `json:"scope"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	est, err := h.svc.CreateEstimate(r.Context(), tenantID, requestID, CreateEstimateParams{
		AmountMinorUnits: req.AmountMinorUnits,
		Currency:         req.Currency,
		Scope:            req.Scope,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, est.ID, est.Version, estimateView(est))
}

func (h *Handler) handleListEstimates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")
	estimates, err := h.svc.ListEstimates(r.Context(), tenantID, requestID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]maintenanceResource, 0, len(estimates))
	for i := range estimates {
		items = append(items, maintenanceResource{
			ID:      estimates[i].ID,
			Version: estimates[i].Version,
			Data:    estimateView(&estimates[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")
	approvals, err := h.svc.GetApprovals(r.Context(), tenantID, requestID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]maintenanceResource, 0, len(approvals))
	for i := range approvals {
		items = append(items, maintenanceResource{
			ID:      approvals[i].ID,
			Version: 1,
			Data:    approvalView(&approvals[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleDecideRequestEstimate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")
	req, err := h.svc.GetRequest(r.Context(), tenantID, requestID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}
	if req.EstimateID == "" {
		apiError(w, r, http.StatusNotFound, "NOT_FOUND", "no estimate found for request")
		return
	}
	h.decideEstimate(w, r, tenantID, req.EstimateID)
}

func (h *Handler) handleSubmitEstimate(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	estimateID := r.PathValue("estimate_id")
	submitted, err := h.svc.SubmitEstimate(r.Context(), tenantID, estimateID, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, submitted.ID, submitted.Version, estimateView(submitted))
}

func (h *Handler) handleDecideEstimate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	estimateID := r.PathValue("estimate_id")
	h.decideEstimate(w, r, tenantID, estimateID)
}

func (h *Handler) decideEstimate(w http.ResponseWriter, r *http.Request, tenantID, estimateID string) {
	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Decision  string `json:"decision"`
		Reason    string `json:"reason"`
		IsAIActor bool   `json:"is_ai_actor"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)

	decided, err := h.svc.DecideEstimate(r.Context(), tenantID, estimateID, DecideEstimateParams{
		ActorID:   actorID,
		Decision:  req.Decision,
		Reason:    req.Reason,
		IsAIActor: req.IsAIActor,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, decided.ID, decided.Version, estimateView(decided))
}

func subjectActor(r *http.Request) (string, error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.ActorID == "" {
		return "", errors.New("unauthenticated")
	}
	return subject.ActorID, nil
}

func (h *Handler) handleAssignWorkOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.PathValue("request_id")

	var req struct {
		VendorID string `json:"vendor_id"`
		Scope    string `json:"scope"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	wo, err := h.svc.AssignVendorWorkOrder(r.Context(), tenantID, requestID, AssignVendorWorkOrderParams{
		VendorID: req.VendorID,
		Scope:    req.Scope,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, wo.ID, wo.Version, workOrderView(wo))
}

func (h *Handler) handleListWorkOrders(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requestID := r.URL.Query().Get("request_id")
	orders, err := h.svc.ListWorkOrders(r.Context(), tenantID, requestID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]maintenanceResource, 0, len(orders))
	for i := range orders {
		items = append(items, maintenanceResource{
			ID:      orders[i].ID,
			Version: orders[i].Version,
			Data:    workOrderView(&orders[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetWorkOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workOrderID := r.PathValue("work_order_id")
	wo, err := h.svc.GetWorkOrder(r.Context(), tenantID, workOrderID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, wo.ID, wo.Version, workOrderView(wo))
}

func (h *Handler) handleStartWorkOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workOrderID := r.PathValue("work_order_id")
	wo, err := h.svc.StartWorkOrder(r.Context(), tenantID, workOrderID, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, wo.ID, wo.Version, workOrderView(wo))
}

func (h *Handler) handleCompleteWorkOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workOrderID := r.PathValue("work_order_id")

	var req struct {
		CompletedBy           string `json:"completed_by"`
		CompletionEvidenceRef string `json:"completion_evidence_ref"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	wo, err := h.svc.CompleteWorkOrder(r.Context(), tenantID, workOrderID, CompleteWorkOrderParams{
		CompletedBy:           req.CompletedBy,
		CompletionEvidenceRef: req.CompletionEvidenceRef,
	})
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, wo.ID, wo.Version, workOrderView(wo))
}

func (h *Handler) handleVerifyWorkOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workOrderID := r.PathValue("work_order_id")
	actorID, err := subjectActor(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	wo, err := h.svc.VerifyWorkOrder(r.Context(), tenantID, workOrderID, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, wo.ID, wo.Version, workOrderView(wo))
}

func (h *Handler) handleRecordWarranty(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workOrderID := r.PathValue("work_order_id")

	var req struct {
		Provider  string `json:"provider"`
		Coverage  string `json:"coverage"`
		ExpiresAt string `json:"expires_at"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid expires_at, use RFC3339")
			return
		}
		expiresAt = &t
	}

	record, err := h.svc.RecordWarranty(r.Context(), tenantID, workOrderID, RecordWarrantyParams{
		Provider:  req.Provider,
		Coverage:  req.Coverage,
		ExpiresAt: expiresAt,
	}, actorID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, record.ID, 1, warrantyView(record))
}

func (h *Handler) handleListVendorWorkOrders(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	orders, err := h.svc.ListVendorWorkOrders(r.Context(), tenantID, actorID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]maintenanceResource, 0, len(orders))
	for i := range orders {
		items = append(items, maintenanceResource{
			ID:      orders[i].ID,
			Version: orders[i].Version,
			Data:    vendorWorkOrderView(&orders[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetVendorWorkOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workOrderID := r.PathValue("work_order_id")
	wo, err := h.svc.GetVendorWorkOrder(r.Context(), tenantID, actorID, workOrderID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, wo.ID, wo.Version, vendorWorkOrderView(wo))
}

func (h *Handler) handleListWarranties(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	records, err := h.svc.ListWarranties(r.Context(), tenantID, propertyID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]maintenanceResource, 0, len(records))
	for i := range records {
		items = append(items, maintenanceResource{
			ID:      records[i].ID,
			Version: 1,
			Data:    warrantyView(&records[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetWarranty(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	warrantyID := r.PathValue("warranty_id")
	record, err := h.svc.GetWarranty(r.Context(), tenantID, warrantyID)
	if err != nil {
		status, code := mapError(err)
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, record.ID, 1, warrantyView(record))
}

func requestView(r *MaintenanceRequest) map[string]any {
	v := map[string]any{
		"id":          r.ID,
		"tenant_id":   r.TenantID,
		"property_id": r.PropertyID,
		"title":       r.Title,
		"category":    r.Category,
		"priority":    r.Priority,
		"risk_level":  r.RiskLevel,
		"status":      r.Status,
		"reported_by": r.ReportedBy,
		"triaged_by":  r.TriagedBy,
		"estimate_id": r.EstimateID,
		"notes":       r.Notes,
		"version":     r.Version,
		"created_at":  r.CreatedAt.Format(time.RFC3339),
		"updated_at":  r.UpdatedAt.Format(time.RFC3339),
	}
	if r.TriagedAt != nil {
		v["triaged_at"] = r.TriagedAt.Format(time.RFC3339)
	}
	return v
}

func estimateView(e *MaintenanceEstimate) map[string]any {
	v := map[string]any{
		"id":                 e.ID,
		"tenant_id":          e.TenantID,
		"request_id":         e.RequestID,
		"property_id":        e.PropertyID,
		"prepared_by":        e.PreparedBy,
		"amount_minor_units": e.AmountMinorUnits,
		"currency":           e.Currency,
		"scope":              e.Scope,
		"status":             e.Status,
		"approved_by":        e.ApprovedBy,
		"rejected_by":        e.RejectedBy,
		"version":            e.Version,
		"created_at":         e.CreatedAt.Format(time.RFC3339),
		"updated_at":         e.UpdatedAt.Format(time.RFC3339),
	}
	if e.SubmittedAt != nil {
		v["submitted_at"] = e.SubmittedAt.Format(time.RFC3339)
	}
	if e.ApprovedAt != nil {
		v["approved_at"] = e.ApprovedAt.Format(time.RFC3339)
	}
	if e.RejectedAt != nil {
		v["rejected_at"] = e.RejectedAt.Format(time.RFC3339)
	}
	return v
}

func approvalView(a *MaintenanceApproval) map[string]any {
	return map[string]any{
		"id":          a.ID,
		"tenant_id":   a.TenantID,
		"request_id":  a.RequestID,
		"estimate_id": a.EstimateID,
		"actor_id":    a.ActorID,
		"decision":    a.Decision,
		"reason":      a.Reason,
		"is_ai_actor": a.IsAIActor,
		"created_at":  a.CreatedAt.Format(time.RFC3339),
	}
}

func workOrderView(wo *VendorWorkOrder) map[string]any {
	v := map[string]any{
		"id":                      wo.ID,
		"tenant_id":               wo.TenantID,
		"request_id":              wo.RequestID,
		"estimate_id":             wo.EstimateID,
		"property_id":             wo.PropertyID,
		"vendor_id":               wo.VendorID,
		"scope":                   wo.Scope,
		"risk_level":              wo.RiskLevel,
		"status":                  wo.Status,
		"assigned_by":             wo.AssignedBy,
		"assigned_at":             wo.AssignedAt.Format(time.RFC3339),
		"completed_by":            wo.CompletedBy,
		"completion_evidence_ref": wo.CompletionEvidenceRef,
		"verified_by":             wo.VerifiedBy,
		"version":                 wo.Version,
		"created_at":              wo.CreatedAt.Format(time.RFC3339),
		"updated_at":              wo.UpdatedAt.Format(time.RFC3339),
	}
	if wo.StartedAt != nil {
		v["started_at"] = wo.StartedAt.Format(time.RFC3339)
	}
	if wo.CompletedAt != nil {
		v["completed_at"] = wo.CompletedAt.Format(time.RFC3339)
	}
	if wo.VerifiedAt != nil {
		v["verified_at"] = wo.VerifiedAt.Format(time.RFC3339)
	}
	return v
}

// vendorWorkOrderView is the scope-limited view a vendor receives. It exposes
// only the assigned scope and its execution state, never the work of another
// vendor.
func vendorWorkOrderView(wo *VendorWorkOrder) map[string]any {
	v := map[string]any{
		"id":                      wo.ID,
		"property_id":             wo.PropertyID,
		"estimate_id":             wo.EstimateID,
		"scope":                   wo.Scope,
		"risk_level":              wo.RiskLevel,
		"status":                  wo.Status,
		"assigned_at":             wo.AssignedAt.Format(time.RFC3339),
		"completion_evidence_ref": wo.CompletionEvidenceRef,
		"version":                 wo.Version,
	}
	if wo.StartedAt != nil {
		v["started_at"] = wo.StartedAt.Format(time.RFC3339)
	}
	if wo.CompletedAt != nil {
		v["completed_at"] = wo.CompletedAt.Format(time.RFC3339)
	}
	if wo.VerifiedAt != nil {
		v["verified_at"] = wo.VerifiedAt.Format(time.RFC3339)
	}
	return v
}

func warrantyView(w *WarrantyRecord) map[string]any {
	v := map[string]any{
		"id":            w.ID,
		"tenant_id":     w.TenantID,
		"work_order_id": w.WorkOrderID,
		"property_id":   w.PropertyID,
		"vendor_id":     w.VendorID,
		"provider":      w.Provider,
		"coverage":      w.Coverage,
		"status":        w.Status,
		"recorded_by":   w.RecordedBy,
		"created_at":    w.CreatedAt.Format(time.RFC3339),
	}
	if w.ExpiresAt != nil {
		v["expires_at"] = w.ExpiresAt.Format(time.RFC3339)
	}
	return v
}
