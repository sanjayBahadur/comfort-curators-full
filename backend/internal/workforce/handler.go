package workforce

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type WorkforceHandler struct {
	svc *WorkforceService
}

func NewWorkforceHandler(svc *WorkforceService) *WorkforceHandler {
	return &WorkforceHandler{svc: svc}
}

func (h *WorkforceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workers", h.handleListWorkers)
	mux.HandleFunc("POST /v1/workers", h.handleCreateWorker)
	mux.HandleFunc("GET /v1/workers/{worker_id}", h.handleGetWorker)
	mux.HandleFunc("POST /v1/workers/{worker_id}/availability-windows", h.handleCreateAvailabilityWindow)
	mux.HandleFunc("GET /v1/workers/{worker_id}/availability-windows", h.handleListAvailabilityWindows)
	mux.HandleFunc("POST /v1/workers/{worker_id}/certifications", h.handleAddCertification)
	mux.HandleFunc("POST /v1/workers/{worker_id}/ratings", h.handleRecordRating)
	mux.HandleFunc("POST /v1/workers/{worker_id}/adverse-actions", h.handleReviewAdverseAction)
	mux.HandleFunc("POST /v1/workers/{worker_id}/time-entries", h.handleRecordTimeEntry)
	mux.HandleFunc("GET /v1/workers/{worker_id}/time-entries", h.handleListTimeEntries)
	mux.HandleFunc("POST /v1/workers/{worker_id}/expenses", h.handleRecordExpense)
	mux.HandleFunc("GET /v1/workers/{worker_id}/expenses", h.handleListExpenses)
	mux.HandleFunc("POST /v1/workers/{worker_id}/grievances", h.handleSubmitGrievance)
	mux.HandleFunc("GET /v1/workers/{worker_id}/grievances", h.handleListGrievances)
	mux.HandleFunc("POST /v1/workers/{worker_id}/sos", h.handleTriggerSOS)
	mux.HandleFunc("GET /v1/workers/{worker_id}/sos-events", h.handleListSOSEvents)
	mux.HandleFunc("POST /v1/workers/{worker_id}/employment-terms", h.handleCreateEmploymentTerm)
	mux.HandleFunc("GET /v1/workers/{worker_id}/employment-terms", h.handleListEmploymentTerms)
	mux.HandleFunc("POST /v1/time-entries", h.handleRecordTimeEntryGlobal)
	mux.HandleFunc("POST /v1/grievances", h.handleSubmitGrievanceGlobal)
}

type apiResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

type apiError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

// --- request types ---

type createWorkerRequest struct {
	LegalName        string   `json:"legal_name"`
	DateOfBirth      string   `json:"date_of_birth"`
	ContactMethod    string   `json:"contact_method"`
	Classification   string   `json:"classification"`
	Specialist       bool     `json:"specialist,omitempty"`
	ServiceZone      string   `json:"service_zone"`
	Skills           []string `json:"skills,omitempty"`
	VerifiedIdentity bool     `json:"verified_identity,omitempty"`
}

type availabilityWindowRequest struct {
	DayOfWeek   int    `json:"day_of_week"`
	StartMinute int    `json:"start_minute"`
	EndMinute   int    `json:"end_minute"`
	EffectiveAt string `json:"effective_at"`
}

type certificationRequest struct {
	WorkType  string `json:"work_type"`
	Issuer    string `json:"issuer"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
}

type ratingRequest struct {
	Score         int    `json:"score"`
	Source        string `json:"source"`
	Comment       string `json:"comment,omitempty"`
	DesiredStatus string `json:"desired_status,omitempty"`
}

type adverseActionRequest struct {
	Action       string   `json:"action"`
	EvidenceRefs []string `json:"evidence_refs"`
	ReviewerID   string   `json:"reviewer_id"`
	Reason       string   `json:"reason"`
}

type timeEntryRequest struct {
	WorkerID      string `json:"worker_id"`
	TicketID      string `json:"ticket_id,omitempty"`
	WorkMinutes   int    `json:"work_minutes"`
	TravelMinutes int    `json:"travel_minutes"`
	OvertimeFlag  bool   `json:"overtime_flag,omitempty"`
}

type expenseRequest struct {
	WorkerID   string `json:"worker_id"`
	TicketID   string `json:"ticket_id,omitempty"`
	MinorUnits int64  `json:"minor_units"`
	Currency   string `json:"currency"`
	Category   string `json:"category,omitempty"`
	ReceiptRef string `json:"receipt_ref,omitempty"`
}

type grievanceRequest struct {
	WorkerID     string   `json:"worker_id"`
	Kind         string   `json:"kind"`
	Reason       string   `json:"reason"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type sosRequest struct {
	TicketID string `json:"ticket_id,omitempty"`
	Location string `json:"location,omitempty"`
}

type employmentTermRequest struct {
	Role             string `json:"role"`
	CompensationBand string `json:"compensation_band,omitempty"`
	EffectiveDate    string `json:"effective_date"`
	EndDate          string `json:"end_date,omitempty"`
	AgreementRef     string `json:"agreement_ref,omitempty"`
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	writeJSON(w, status, apiError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func parseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func workerData(w *Worker) map[string]any {
	m := map[string]any{
		"id":                w.ID,
		"tenant_id":         w.TenantID,
		"legal_name":        w.LegalName,
		"verified_identity": w.VerifiedIdentity,
		"date_of_birth":     w.DateOfBirth.Format(time.RFC3339),
		"age_eligible":      w.AgeEligible,
		"contact_method":    w.ContactMethod,
		"classification":    w.Classification,
		"specialist":        w.Specialist,
		"service_zone":      w.ServiceZone,
		"skills":            w.Skills,
		"status":            w.Status,
		"version":           w.Version,
		"created_at":        w.CreatedAt.Format(time.RFC3339),
		"updated_at":        w.UpdatedAt.Format(time.RFC3339),
	}
	return m
}

// --- handlers ---

func (h *WorkforceHandler) handleCreateWorker(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createWorkerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	dob, err := parseRFC3339(req.DateOfBirth)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid date_of_birth, use RFC3339")
		return
	}

	worker, err := h.svc.CreateWorker(r.Context(), CreateWorkerParams{
		TenantID:         tenantID,
		LegalName:        req.LegalName,
		VerifiedIdentity: req.VerifiedIdentity,
		DateOfBirth:      dob,
		ContactMethod:    req.ContactMethod,
		Classification:   req.Classification,
		Specialist:       req.Specialist,
		ServiceZone:      req.ServiceZone,
		Skills:           req.Skills,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrMissingLegalName) || errors.Is(err, ErrInvalidClassification) || errors.Is(err, ErrInvalidDateOfBirth) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      worker.ID,
		Version: worker.Version,
		Data:    workerData(worker),
	})
}

func (h *WorkforceHandler) handleGetWorker(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	worker, err := h.svc.GetWorker(r.Context(), tenantID, workerID)
	if err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "worker not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      worker.ID,
		Version: worker.Version,
		Data:    workerData(worker),
	})
}

func (h *WorkforceHandler) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workers, err := h.svc.ListWorkers(r.Context(), tenantID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if workers == nil {
		workers = []Worker{}
	}

	resources := make([]apiResource, 0, len(workers))
	for _, w := range workers {
		resources = append(resources, apiResource{
			ID:      w.ID,
			Version: w.Version,
			Data:    workerData(&w),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *WorkforceHandler) handleCreateAvailabilityWindow(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req availabilityWindowRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	effectiveAt, err := parseRFC3339(req.EffectiveAt)
	if err != nil && req.EffectiveAt != "" {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid effective_at, use RFC3339")
		return
	}

	window, err := h.svc.CreateAvailabilityWindow(r.Context(), tenantID, workerID, AvailabilityWindowParams{
		DayOfWeek:   req.DayOfWeek,
		StartMinute: req.StartMinute,
		EndMinute:   req.EndMinute,
		EffectiveAt: effectiveAt,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      window.ID,
		Version: 1,
		Data:    window,
	})
}

func (h *WorkforceHandler) handleListAvailabilityWindows(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	windows, err := h.svc.ListAvailabilityWindows(r.Context(), tenantID, workerID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if windows == nil {
		windows = []AvailabilityWindow{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": windows,
		"total": len(windows),
	})
}

func (h *WorkforceHandler) handleAddCertification(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req certificationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	issuedAt, err := parseRFC3339(req.IssuedAt)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid issued_at, use RFC3339")
		return
	}
	expiresAt, err := parseRFC3339(req.ExpiresAt)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid expires_at, use RFC3339")
		return
	}

	cert, err := h.svc.AddCertification(r.Context(), tenantID, workerID, CertificationParams{
		WorkType:  req.WorkType,
		Issuer:    req.Issuer,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      cert.ID,
		Version: 1,
		Data:    cert,
	})
}

func (h *WorkforceHandler) handleRecordRating(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req ratingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	rating, err := h.svc.RecordRating(r.Context(), tenantID, workerID, RatingParams{
		Score:         req.Score,
		Source:        req.Source,
		Comment:       req.Comment,
		DesiredStatus: req.DesiredStatus,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidRatingScore) || errors.Is(err, ErrInvalidRatingSource) || errors.Is(err, ErrRatingCannotDeactivate) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      rating.ID,
		Version: 1,
		Data:    rating,
	})
}

func (h *WorkforceHandler) handleReviewAdverseAction(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req adverseActionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	reviewed, err := h.svc.ReviewAdverseAction(r.Context(), tenantID, workerID, AdverseActionParams{
		Action:       req.Action,
		EvidenceRefs: req.EvidenceRefs,
		ReviewerID:   req.ReviewerID,
		Reason:       req.Reason,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidAdverseAction) || errors.Is(err, ErrAdverseActionRequiresReviewer) ||
			errors.Is(err, ErrAdverseActionRequiresEvidence) || errors.Is(err, ErrAdverseActionRequiresReason) ||
			errors.Is(err, ErrAdverseActionSelfReview) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      reviewed.ID,
		Version: reviewed.Version,
		Data:    workerData(reviewed),
	})
}

func (h *WorkforceHandler) handleRecordTimeEntry(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req timeEntryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	entry, err := h.svc.RecordTimeEntry(r.Context(), tenantID, workerID, TimeEntryParams{
		TicketID:      req.TicketID,
		WorkMinutes:   req.WorkMinutes,
		TravelMinutes: req.TravelMinutes,
		OvertimeFlag:  req.OvertimeFlag,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      entry.ID,
		Version: 1,
		Data:    entry,
	})
}

func (h *WorkforceHandler) handleListTimeEntries(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	entries, err := h.svc.ListTimeEntries(r.Context(), tenantID, workerID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if entries == nil {
		entries = []TimeEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": entries,
		"total": len(entries),
	})
}

func (h *WorkforceHandler) handleRecordTimeEntryGlobal(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req timeEntryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.WorkerID == "" {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "worker_id is required")
		return
	}

	entry, err := h.svc.RecordTimeEntry(r.Context(), tenantID, req.WorkerID, TimeEntryParams{
		TicketID:      req.TicketID,
		WorkMinutes:   req.WorkMinutes,
		TravelMinutes: req.TravelMinutes,
		OvertimeFlag:  req.OvertimeFlag,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      entry.ID,
		Version: 1,
		Data:    entry,
	})
}

func (h *WorkforceHandler) handleRecordExpense(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req expenseRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	expense, err := h.svc.RecordExpense(r.Context(), tenantID, workerID, ExpenseParams{
		TicketID:   req.TicketID,
		MinorUnits: req.MinorUnits,
		Currency:   req.Currency,
		Category:   req.Category,
		ReceiptRef: req.ReceiptRef,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      expense.ID,
		Version: 1,
		Data:    expense,
	})
}

func (h *WorkforceHandler) handleListExpenses(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	expenses, err := h.svc.ListExpenses(r.Context(), tenantID, workerID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if expenses == nil {
		expenses = []Expense{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": expenses,
		"total": len(expenses),
	})
}

func (h *WorkforceHandler) handleSubmitGrievance(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req grievanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	grievance, err := h.svc.SubmitGrievance(r.Context(), tenantID, workerID, GrievanceParams{
		Kind:         req.Kind,
		Reason:       req.Reason,
		EvidenceRefs: req.EvidenceRefs,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      grievance.ID,
		Version: 1,
		Data:    grievance,
	})
}

func (h *WorkforceHandler) handleListGrievances(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	grievances, err := h.svc.ListGrievances(r.Context(), tenantID, workerID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if grievances == nil {
		grievances = []Grievance{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": grievances,
		"total": len(grievances),
	})
}

func (h *WorkforceHandler) handleSubmitGrievanceGlobal(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req grievanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.WorkerID == "" {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "worker_id is required")
		return
	}

	grievance, err := h.svc.SubmitGrievance(r.Context(), tenantID, req.WorkerID, GrievanceParams{
		Kind:         req.Kind,
		Reason:       req.Reason,
		EvidenceRefs: req.EvidenceRefs,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      grievance.ID,
		Version: 1,
		Data:    grievance,
	})
}

func (h *WorkforceHandler) handleTriggerSOS(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req sosRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	sosEvent, err := h.svc.TriggerSOS(r.Context(), tenantID, workerID, SOSEventParams{
		TicketID: req.TicketID,
		Location: req.Location,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      sosEvent.ID,
		Version: 1,
		Data:    sosEvent,
	})
}

func (h *WorkforceHandler) handleListSOSEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	events, err := h.svc.ListSOSEvents(r.Context(), tenantID, workerID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if events == nil {
		events = []SOSEvent{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": events,
		"total": len(events),
	})
}

func (h *WorkforceHandler) handleCreateEmploymentTerm(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req employmentTermRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	effectiveDate, err := parseRFC3339(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid effective_date, use RFC3339")
		return
	}

	var endDate *time.Time
	if req.EndDate != "" {
		parsed, err := parseRFC3339(req.EndDate)
		if err != nil {
			writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid end_date, use RFC3339")
			return
		}
		endDate = &parsed
	}

	term, err := h.svc.CreateEmploymentTerm(r.Context(), tenantID, workerID, EmploymentTermParams{
		Role:             req.Role,
		CompensationBand: req.CompensationBand,
		EffectiveDate:    effectiveDate,
		EndDate:          endDate,
		AgreementRef:     req.AgreementRef,
	}, actorID)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      term.ID,
		Version: 1,
		Data:    term,
	})
}

func (h *WorkforceHandler) handleListEmploymentTerms(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	terms, err := h.svc.ListEmploymentTerms(r.Context(), tenantID, workerID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if terms == nil {
		terms = []EmploymentTerm{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": terms,
		"total": len(terms),
	})
}
