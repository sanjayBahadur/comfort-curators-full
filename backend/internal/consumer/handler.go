package consumer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *ConsumerService
}

func NewHandler(svc *ConsumerService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/consumer/disclosures", h.handleRecordDisclosure)
	mux.HandleFunc("GET /v1/consumer/disclosures", h.handleListDisclosures)
	mux.HandleFunc("GET /v1/consumer/disclosures/{disclosure_id}", h.handleGetDisclosure)

	mux.HandleFunc("POST /v1/consumer/acceptances", h.handleAccept)
	mux.HandleFunc("GET /v1/consumer/acceptances/{acceptance_id}", h.handleGetAcceptance)

	mux.HandleFunc("POST /v1/consumer/history-exports", h.handleCreateHistoryExport)
	mux.HandleFunc("GET /v1/consumer/history-exports", h.handleListHistoryExports)
	mux.HandleFunc("GET /v1/consumer/history-exports/{export_id}", h.handleGetHistoryExport)
}

type apiError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

type apiResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
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

// --- Disclosures ---

type recordDisclosureRequest struct {
	PropertyID              string `json:"property_id"`
	ResourceType            string `json:"resource_type"`
	ResourceID              string `json:"resource_id"`
	PriceMinorUnits         int64  `json:"price_minor_units"`
	TaxMinorUnits           int64  `json:"tax_minor_units"`
	Currency                string `json:"currency"`
	Recurrence              string `json:"recurrence"`
	RecurringCostMinorUnits *int64 `json:"recurring_cost_minor_units"`
	SubstitutionPolicy      string `json:"substitution_policy"`
	CancellationPolicy      string `json:"cancellation_policy"`
	RefundPolicy            string `json:"refund_policy"`
	Seller                  string `json:"seller"`
	CountryOfOrigin         string `json:"country_of_origin"`
	GrievanceContact        string `json:"grievance_contact"`
}

func (h *Handler) handleRecordDisclosure(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	var req recordDisclosureRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	d, err := h.svc.RecordDisclosure(r.Context(), tenantID, DisclosureParams{
		PropertyID:                 req.PropertyID,
		ResourceType:               req.ResourceType,
		ResourceID:                 req.ResourceID,
		PriceMinorUnits:            req.PriceMinorUnits,
		TaxMinorUnits:              req.TaxMinorUnits,
		Currency:                   req.Currency,
		Recurrence:                 req.Recurrence,
		RecurrenceAmountMinorUnits: req.RecurringCostMinorUnits,
		SubstitutionPolicy:         req.SubstitutionPolicy,
		CancellationPolicy:         req.CancellationPolicy,
		RefundPolicy:               req.RefundPolicy,
		Seller:                     req.Seller,
		CountryOfOrigin:            req.CountryOfOrigin,
		GrievanceContact:           req.GrievanceContact,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidDisclosure),
			errors.Is(err, ErrInvalidCurrency),
			errors.Is(err, ErrInvalidRecurrence),
			errors.Is(err, ErrInvalidResourceType):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrHiddenRecurringCost):
			status = http.StatusUnprocessableEntity
			code = "HIDDEN_RECURRING_COST"
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{ID: d.ID, Version: 1, Data: d})
}

func (h *Handler) handleGetDisclosure(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	disclosureID := r.PathValue("disclosure_id")
	d, err := h.svc.GetDisclosure(r.Context(), tenantID, disclosureID)
	if err != nil {
		if errors.Is(err, ErrDisclosureNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "disclosure not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{ID: d.ID, Version: 1, Data: d})
}

func (h *Handler) handleListDisclosures(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	disclosures, err := h.svc.ListDisclosures(r.Context(), tenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resources := make([]apiResource, 0, len(disclosures))
	for i := range disclosures {
		resources = append(resources, apiResource{ID: disclosures[i].ID, Version: 1, Data: &disclosures[i]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resources, "total": len(resources)})
}

// --- Acceptances ---

type acceptRequest struct {
	PropertyID   string `json:"property_id"`
	DisclosureID string `json:"disclosure_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

func (h *Handler) handleAccept(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	var req acceptRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	a, err := h.svc.Accept(r.Context(), tenantID, AcceptanceParams{
		PropertyID:   req.PropertyID,
		DisclosureID: req.DisclosureID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidAcceptance),
			errors.Is(err, ErrInvalidResourceType):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrNoDisclosureBeforeAccept):
			status = http.StatusUnprocessableEntity
			code = "DISCLOSURE_REQUIRED"
		case errors.Is(err, ErrRecurringCostNotVisible),
			errors.Is(err, ErrHiddenRecurringCost):
			status = http.StatusUnprocessableEntity
			code = "RECURRING_COST_HIDDEN"
		case errors.Is(err, ErrDisclosureResourceMismatch):
			status = http.StatusUnprocessableEntity
			code = "DISCLOSURE_MISMATCH"
		case errors.Is(err, ErrDisclosureNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{ID: a.ID, Version: 1, Data: a})
}

func (h *Handler) handleGetAcceptance(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	acceptanceID := r.PathValue("acceptance_id")
	a, err := h.svc.GetAcceptance(r.Context(), tenantID, acceptanceID)
	if err != nil {
		if errors.Is(err, ErrAcceptanceNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "acceptance not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{ID: a.ID, Version: 1, Data: a})
}

// --- History exports ---

type createHistoryExportRequest struct {
	PropertyID string `json:"property_id"`
}

func (h *Handler) handleCreateHistoryExport(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req createHistoryExportRequest
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
			return
		}
	}

	exp, err := h.svc.CreateHistoryExport(r.Context(), tenantID, req.PropertyID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidExport) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{ID: exp.ID, Version: 1, Data: exp})
}

func (h *Handler) handleGetHistoryExport(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	exportID := r.PathValue("export_id")
	exp, err := h.svc.GetHistoryExport(r.Context(), tenantID, exportID)
	if err != nil {
		if errors.Is(err, ErrExportNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "history export not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{ID: exp.ID, Version: 1, Data: exp})
}

func (h *Handler) handleListHistoryExports(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	exports, err := h.svc.ListHistoryExports(r.Context(), tenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resources := make([]apiResource, 0, len(exports))
	for i := range exports {
		resources = append(resources, apiResource{ID: exports[i].ID, Version: 1, Data: &exports[i]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resources, "total": len(resources)})
}
