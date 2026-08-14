package privacy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *PrivacyService
}

func NewHandler(svc *PrivacyService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/privacy/purposes", h.handleCreatePurpose)
	mux.HandleFunc("GET /v1/privacy/purposes/{purpose_id}", h.handleGetPurpose)
	mux.HandleFunc("GET /v1/privacy/purposes", h.handleListPurposes)

	mux.HandleFunc("POST /v1/privacy/notices", h.handleRecordNotice)
	mux.HandleFunc("GET /v1/privacy/notices/{notice_id}", h.handleGetNotice)

	mux.HandleFunc("POST /v1/privacy/consents", h.handleRecordConsent)
	mux.HandleFunc("POST /v1/privacy/consents/{consent_id}/withdraw", h.handleWithdrawConsent)
	mux.HandleFunc("GET /v1/privacy/consents/{consent_id}", h.handleGetConsent)

	mux.HandleFunc("POST /v1/privacy/rights-requests", h.handleSubmitRightsRequest)
	mux.HandleFunc("GET /v1/privacy/rights-requests/{request_id}", h.handleGetRightsRequest)
	mux.HandleFunc("POST /v1/privacy/rights-requests/{request_id}/process", h.handleProcessRightsRequest)

	mux.HandleFunc("POST /v1/privacy/retention-records", h.handleCreateRetentionRecord)
	mux.HandleFunc("GET /v1/privacy/retention-records/{record_id}", h.handleGetRetentionRecord)

	mux.HandleFunc("POST /v1/privacy/processors", h.handleRecordProcessor)
	mux.HandleFunc("POST /v1/privacy/processors/{contract_id}/review", h.handleReviewProcessor)
	mux.HandleFunc("GET /v1/privacy/processors/{contract_id}", h.handleGetProcessor)

	mux.HandleFunc("POST /v1/privacy/security-log-settings", h.handleSetSecurityLogRetention)
	mux.HandleFunc("GET /v1/privacy/security-log-settings/{setting_id}", h.handleGetSecurityLogSetting)

	mux.HandleFunc("POST /v1/privacy/identity-alternatives", h.handleRecordIdentityAlternative)
	mux.HandleFunc("GET /v1/privacy/identity-alternatives/{alt_id}", h.handleGetIdentityAlternative)

	mux.HandleFunc("POST /v1/privacy/aadhaar-preferences", h.handleSetAadhaarPreference)
	mux.HandleFunc("GET /v1/privacy/aadhaar-preferences/{actor_id}", h.handleGetAadhaarPreference)

	mux.HandleFunc("POST /v1/privacy/evaluation-exports", h.handleRequestEvalExport)
	mux.HandleFunc("GET /v1/privacy/evaluation-exports/{export_id}", h.handleGetEvalExport)
	mux.HandleFunc("POST /v1/privacy/evaluation-exports/{export_id}/approve", h.handleApproveEvalExport)
	mux.HandleFunc("POST /v1/privacy/evaluation-exports/{export_id}/deny", h.handleDenyEvalExport)
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

// --- Purpose handlers ---

func (h *Handler) handleCreatePurpose(w http.ResponseWriter, r *http.Request) {
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
	var req CreatePurposeParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	p, err := h.svc.CreatePurpose(r.Context(), req, tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: p.ID, Version: 1, Data: p})
}

func (h *Handler) handleGetPurpose(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	p, err := h.svc.GetPurpose(r.Context(), tenantID, r.PathValue("purpose_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "purpose not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: p.ID, Version: 1, Data: p})
}

func (h *Handler) handleListPurposes(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	purposes, err := h.svc.ListPurposes(r.Context(), tenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	resources := make([]apiResource, 0, len(purposes))
	for _, p := range purposes {
		resources = append(resources, apiResource{ID: p.ID, Version: 1, Data: p})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resources, "total": len(resources)})
}

// --- Notice handlers ---

func (h *Handler) handleRecordNotice(w http.ResponseWriter, r *http.Request) {
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
	var req CreateNoticeParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	n, err := h.svc.RecordNotice(r.Context(), req, tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: n.ID, Version: 1, Data: n})
}

func (h *Handler) handleGetNotice(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	n, err := h.svc.GetNotice(r.Context(), tenantID, r.PathValue("notice_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "notice not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: n.ID, Version: 1, Data: n})
}

// --- Consent handlers ---

func (h *Handler) handleRecordConsent(w http.ResponseWriter, r *http.Request) {
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
	var req CreateConsentParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	c, err := h.svc.RecordConsent(r.Context(), req, tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: c.ID, Version: 1, Data: c})
}

func (h *Handler) handleWithdrawConsent(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	consentID := r.PathValue("consent_id")
	var req struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
	}
	c, err := h.svc.WithdrawConsent(r.Context(), consentID, tenantID, actorID, req.Reason)
	if err != nil {
		if errors.Is(err, ErrConsentWithdrawn) {
			writeError(w, r, http.StatusConflict, "CONFLICT", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: c.ID, Version: 1, Data: c})
}

func (h *Handler) handleGetConsent(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	c, err := h.svc.GetConsent(r.Context(), tenantID, r.PathValue("consent_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "consent not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: c.ID, Version: 1, Data: c})
}

// --- Rights Request handlers ---

func (h *Handler) handleSubmitRightsRequest(w http.ResponseWriter, r *http.Request) {
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
	var req CreateRightsRequestParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	rr, err := h.svc.SubmitRightsRequest(r.Context(), req, tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: rr.ID, Version: 1, Data: rr})
}

func (h *Handler) handleGetRightsRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	rr, err := h.svc.GetRightsRequest(r.Context(), tenantID, r.PathValue("request_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "rights request not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: rr.ID, Version: 1, Data: rr})
}

func (h *Handler) handleProcessRightsRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	requestID := r.PathValue("request_id")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	var req struct {
		Approved     bool   `json:"approved"`
		ResponseData string `json:"response_data"`
		BlockReason  string `json:"block_reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	rr, err := h.svc.ProcessRightsRequest(r.Context(), requestID, tenantID, actorID, req.Approved, req.ResponseData, req.BlockReason)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: rr.ID, Version: 1, Data: rr})
}

// --- Retention Record handlers ---

func (h *Handler) handleCreateRetentionRecord(w http.ResponseWriter, r *http.Request) {
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
	var req CreateRetentionRecordParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	record, err := h.svc.CreateRetentionRecord(r.Context(), req, tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: record.ID, Version: 1, Data: record})
}

func (h *Handler) handleGetRetentionRecord(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	record, err := h.svc.GetRetentionRecord(r.Context(), tenantID, r.PathValue("record_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "retention record not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: record.ID, Version: 1, Data: record})
}

// --- Processor handlers ---

func (h *Handler) handleRecordProcessor(w http.ResponseWriter, r *http.Request) {
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
	var req CreateProcessorContractParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	pc, err := h.svc.RecordProcessorContract(r.Context(), req, tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: pc.ID, Version: 1, Data: pc})
}

func (h *Handler) handleReviewProcessor(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	contractID := r.PathValue("contract_id")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	var req struct {
		Approved    bool   `json:"approved"`
		ReviewNotes string `json:"review_notes"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	pc, err := h.svc.ReviewProcessorContract(r.Context(), contractID, tenantID, actorID, req.Approved, req.ReviewNotes)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: pc.ID, Version: 1, Data: pc})
}

func (h *Handler) handleGetProcessor(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	pc, err := h.svc.GetProcessorContract(r.Context(), tenantID, r.PathValue("contract_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "processor contract not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: pc.ID, Version: 1, Data: pc})
}

// --- Security Log handlers ---

func (h *Handler) handleSetSecurityLogRetention(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Region          string `json:"region"`
		RetentionYears  int    `json:"retention_years"`
		IncidentProcess string `json:"incident_report_process"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	sls, err := h.svc.SetSecurityLogRetention(r.Context(), tenantID, actorID, req.Region, req.RetentionYears, req.IncidentProcess)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: sls.ID, Version: 1, Data: sls})
}

func (h *Handler) handleGetSecurityLogSetting(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	sls, err := h.svc.GetSecurityLogSetting(r.Context(), tenantID, r.PathValue("setting_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "security log setting not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: sls.ID, Version: 1, Data: sls})
}

// --- Identity Alternative handlers ---

func (h *Handler) handleRecordIdentityAlternative(w http.ResponseWriter, r *http.Request) {
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
	var req CreateIdentityAlternativeParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	alt, err := h.svc.RecordIdentityAlternative(r.Context(), req, tenantID, actorID)
	if err != nil {
		if errors.Is(err, ErrAadhaarRequired) {
			writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: alt.ID, Version: 1, Data: alt})
}

func (h *Handler) handleGetIdentityAlternative(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	alt, err := h.svc.GetIdentityAlternative(r.Context(), tenantID, r.PathValue("alt_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "identity alternative not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: alt.ID, Version: 1, Data: alt})
}

// --- Aadhaar Preference handlers ---

func (h *Handler) handleSetAadhaarPreference(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
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
		ActorID         string `json:"actor_id"`
		AadhaarProvided bool   `json:"aadhaar_provided"`
		AadhaarMasked   string `json:"aadhaar_masked"`
		AltIDType       string `json:"alternate_id_type"`
		AltIDValue      string `json:"alternate_id_value"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	ap, err := h.svc.SetAadhaarPreference(r.Context(), req.ActorID, tenantID, req.AadhaarProvided, req.AadhaarMasked, req.AltIDType, req.AltIDValue)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: ap.ID, Version: 1, Data: ap})
}

func (h *Handler) handleGetAadhaarPreference(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	actorID := r.PathValue("actor_id")
	ap, err := h.svc.GetAadhaarPreference(r.Context(), tenantID, actorID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "aadhaar preference not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: ap.ID, Version: 1, Data: ap})
}

// --- Evaluation Export handlers ---

func (h *Handler) handleRequestEvalExport(w http.ResponseWriter, r *http.Request) {
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
	var req CreateEvalExportParams
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	ee, err := h.svc.RequestEvalExport(r.Context(), req, tenantID, actorID)
	if err != nil {
		if errors.Is(err, ErrProductionDataInEval) {
			data := map[string]any{
				"id":            ee.ID,
				"status":        ee.Status,
				"denial_reason": ee.DenialReason,
			}
			writeJSON(w, http.StatusUnprocessableEntity, apiResource{ID: ee.ID, Version: 1, Data: data})
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, apiResource{ID: ee.ID, Version: 1, Data: ee})
}

func (h *Handler) handleGetEvalExport(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	ee, err := h.svc.GetEvalExport(r.Context(), tenantID, r.PathValue("export_id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "evaluation export not found")
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: ee.ID, Version: 1, Data: ee})
}

func (h *Handler) handleApproveEvalExport(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	exportID := r.PathValue("export_id")
	ee, err := h.svc.ApproveEvalExport(r.Context(), exportID, tenantID, actorID)
	if err != nil {
		if errors.Is(err, ErrProductionDataInEval) {
			writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: ee.ID, Version: 1, Data: ee})
}

func (h *Handler) handleDenyEvalExport(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	exportID := r.PathValue("export_id")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	ee, err := h.svc.DenyEvalExport(r.Context(), exportID, tenantID, actorID, req.Reason)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResource{ID: ee.ID, Version: 1, Data: ee})
}
