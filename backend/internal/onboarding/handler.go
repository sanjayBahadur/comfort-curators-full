package onboarding

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type OnboardingHandler struct {
	svc *Service
}

func NewOnboardingHandler(svc *Service) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

func (h *OnboardingHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/onboarding/cases", h.handleStartCase)
	mux.HandleFunc("GET /v1/onboarding/cases", h.handleListCases)
	mux.HandleFunc("GET /v1/onboarding/cases/{case_id}", h.handleGetCase)
	mux.HandleFunc("PUT /v1/onboarding/cases/{case_id}/sections/{section}", h.handleSaveSection)
	mux.HandleFunc("PUT /v1/onboarding/cases/{case_id}/contacts", h.handleSaveContacts)
	mux.HandleFunc("POST /v1/onboarding/cases/{case_id}/evidence", h.handleRecordEvidence)
	mux.HandleFunc("POST /v1/onboarding/cases/{case_id}/inspections", h.handleRecordInspection)
	mux.HandleFunc("GET /v1/onboarding/cases/{case_id}/progress", h.handleProgress)
	mux.HandleFunc("GET /v1/onboarding/cases/{case_id}/activation-holds", h.handleActivationHolds)
	mux.HandleFunc("POST /v1/onboarding/cases/{case_id}/activate", h.handleActivate)
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

type apiCollection struct {
	Items      []apiResource `json:"items"`
	NextCursor *string       `json:"next_cursor,omitempty"`
}

type startCaseRequest struct {
	TenantID         string `json:"tenant_id"`
	PropertyID       string `json:"property_id"`
	OwnerAuthorityID string `json:"owner_authority_id"`
}

type saveSectionRequest struct {
	Payload json.RawMessage `json:"payload"`
}

type saveContactsRequest struct {
	Contacts []Contact `json:"contacts"`
}

type recordEvidenceRequest struct {
	Kind        string    `json:"kind"`
	ContentHash string    `json:"content_hash"`
	ObjectRef   string    `json:"object_ref"`
	CapturedBy  string    `json:"captured_by,omitempty"`
	CapturedAt  time.Time `json:"captured_at,omitempty"`
}

type recordInspectionRequest struct {
	PropertyID    string    `json:"property_id"`
	PerformedAt   time.Time `json:"performed_at,omitempty"`
	InspectedBy   string    `json:"inspected_by"`
	EvidenceHash  string    `json:"evidence_hash"`
	EvidenceRef   string    `json:"evidence_ref"`
	Findings      string    `json:"findings"`
	OverallStatus string    `json:"overall_status"`
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

func onboardingCaseResource(c *Case) apiResource {
	data := map[string]any{
		"tenant_id":           c.TenantID,
		"property_id":         c.PropertyID,
		"owner_authority_id":  c.OwnerAuthorityID,
		"status":              c.Status,
		"portfolio":           c.Portfolio,
		"goals":               c.Goals,
		"service_preferences": c.ServicePreferences,
		"budgets":             c.Budgets,
		"contacts":            contactsValue(c.Contacts),
		"photographs":         c.Photographs,
		"amenities":           c.Amenities,
		"safety":              c.Safety,
		"furnishing":          c.Furnishing,
		"remediation":         c.Remediation,
		"fit_score_inputs":    c.FitScoreInputs,
		"evidence":            c.Evidence,
		"inspections":         c.Inspections,
		"activation_holds":    holdListValue(c.Holds),
		"created_at":          c.CreatedAt.Format(time.RFC3339),
		"updated_at":          c.UpdatedAt.Format(time.RFC3339),
	}
	if data["evidence"] == nil {
		data["evidence"] = []Evidence{}
	}
	if data["inspections"] == nil {
		data["inspections"] = []Inspection{}
	}
	return apiResource{
		ID:      c.ID,
		Version: c.Version,
		Data:    data,
	}
}

func contactsValue(c []Contact) []Contact {
	if c == nil {
		return []Contact{}
	}
	return c
}

func holdListValue(h []ActivationHold) []ActivationHold {
	if h == nil {
		return []ActivationHold{}
	}
	return h
}

func (h *OnboardingHandler) handleStartCase(w http.ResponseWriter, r *http.Request) {
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

	var req startCaseRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.TenantID == "" {
		req.TenantID = tenantID
	}
	if req.PropertyID == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id is required")
		return
	}
	if req.OwnerAuthorityID == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "owner_authority_id is required")
		return
	}

	started, err := h.svc.StartCase(r.Context(), StartCaseParams{
		TenantID:         req.TenantID,
		PropertyID:       req.PropertyID,
		OwnerAuthorityID: req.OwnerAuthorityID,
	}, actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, onboardingCaseResource(started))
}

func (h *OnboardingHandler) handleListCases(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	cases, err := h.svc.ListCases(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]apiResource, 0, len(cases))
	for _, c := range cases {
		cc := c
		items = append(items, onboardingCaseResource(&cc))
	}

	writeJSON(w, http.StatusOK, apiCollection{Items: items})
}

func (h *OnboardingHandler) handleGetCase(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	c, err := h.svc.GetCase(r.Context(), tenantID, caseID)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "onboarding case not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, c.Version))
	writeJSON(w, http.StatusOK, onboardingCaseResource(c))
}

func (h *OnboardingHandler) handleSaveSection(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")
	section := r.PathValue("section")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req saveSectionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Payload == nil {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "payload is required")
		return
	}

	c, err := h.svc.SaveSection(r.Context(), tenantID, caseID, section, req.Payload, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidSection):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrCaseNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCaseActivated):
			code = "INVALID_STATE"
			status = http.StatusConflict
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, c.Version))
	writeJSON(w, http.StatusOK, onboardingCaseResource(c))
}

func (h *OnboardingHandler) handleSaveContacts(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req saveContactsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if len(req.Contacts) == 0 {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "at least one contact is required")
		return
	}

	c, err := h.svc.SaveContacts(r.Context(), tenantID, caseID, req.Contacts, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrCaseNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCaseActivated):
			code = "INVALID_STATE"
			status = http.StatusConflict
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, c.Version))
	writeJSON(w, http.StatusOK, onboardingCaseResource(c))
}

func (h *OnboardingHandler) handleRecordEvidence(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req recordEvidenceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Kind == "" || req.ContentHash == "" || req.ObjectRef == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "kind, content_hash and object_ref are required")
		return
	}

	c, err := h.svc.RecordEvidence(r.Context(), tenantID, caseID, EvidenceParams{
		Kind:        req.Kind,
		ContentHash: req.ContentHash,
		ObjectRef:   req.ObjectRef,
		CapturedBy:  req.CapturedBy,
		CapturedAt:  req.CapturedAt,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidEvidence):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrCaseNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCaseActivated):
			code = "INVALID_STATE"
			status = http.StatusConflict
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, c.Version))
	writeJSON(w, http.StatusCreated, onboardingCaseResource(c))
}

func (h *OnboardingHandler) handleRecordInspection(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req recordInspectionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.PropertyID == "" || req.InspectedBy == "" || req.EvidenceHash == "" || req.OverallStatus == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id, inspected_by, evidence_hash and overall_status are required")
		return
	}

	insp, err := h.svc.RecordInspection(r.Context(), tenantID, caseID, InspectionParams{
		PropertyID:    req.PropertyID,
		PerformedAt:   req.PerformedAt,
		InspectedBy:   req.InspectedBy,
		EvidenceHash:  req.EvidenceHash,
		EvidenceRef:   req.EvidenceRef,
		Findings:      req.Findings,
		OverallStatus: req.OverallStatus,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidInspection):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrCaseNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCaseActivated):
			code = "INVALID_STATE"
			status = http.StatusConflict
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      insp.ID,
		Version: 1,
		Data:    insp,
	})
}

func (h *OnboardingHandler) handleProgress(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	progress, err := h.svc.Progress(r.Context(), tenantID, caseID)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "onboarding case not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"progress": progress,
	})
}

func (h *OnboardingHandler) handleActivationHolds(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	holds, err := h.svc.ActivationHolds(r.Context(), tenantID, caseID)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "onboarding case not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"holds":        holds,
		"can_activate": len(holds) == 0,
	})
}

func (h *OnboardingHandler) handleActivate(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	caseID := r.PathValue("case_id")

	c, err := h.svc.Activate(r.Context(), tenantID, caseID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrActivationBlocked):
			code = "ACTIVATION_BLOCKED"
			status = http.StatusConflict
		case errors.Is(err, ErrIncomplete):
			code = "INCOMPLETE"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrCaseNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCaseActivated):
			code = "INVALID_STATE"
			status = http.StatusConflict
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, c.Version))
	writeJSON(w, http.StatusOK, onboardingCaseResource(c))
}
