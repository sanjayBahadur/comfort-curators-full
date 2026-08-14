package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/contracts/quotes", h.handleQuote)
	mux.HandleFunc("POST /v1/contracts/agreements", h.handleCreateAgreement)
	mux.HandleFunc("GET /v1/contracts/agreements", h.handleListAgreements)
	mux.HandleFunc("GET /v1/contracts/agreements/{agreement_id}", h.handleGetAgreement)
	mux.HandleFunc("PUT /v1/contracts/agreements/{agreement_id}/versions", h.handleAddVersion)
	mux.HandleFunc("POST /v1/contracts/agreements/{agreement_id}/versions", h.handleAddVersion)
	mux.HandleFunc("POST /v1/contracts/agreements/{agreement_id}/accept", h.handleAccept)
	mux.HandleFunc("POST /v1/contracts/fee-rules", h.handleSaveFeeRule)
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

type quoteRequest struct {
	TenantID                       string              `json:"tenant_id"`
	PropertyID                     string              `json:"property_id"`
	ServiceTier                    string              `json:"service_tier"`
	ManagedUnits                   int                 `json:"managed_units"`
	Currency                       string              `json:"currency"`
	RevenuePeriod                  string              `json:"revenue_period"`
	AccommodationRevenueMinorUnits int64               `json:"accommodation_revenue_minor_units"`
	PassThroughs                   []PassThroughAmount `json:"pass_throughs,omitempty"`
	IncludedPassThroughs           []string            `json:"included_pass_throughs,omitempty"`
	RuleVersion                    string              `json:"rule_version"`
}

type createAgreementRequest struct {
	TenantID   string          `json:"tenant_id"`
	PropertyID string          `json:"property_id"`
	Terms      json.RawMessage `json:"terms"`
}

type addVersionRequest struct {
	Terms json.RawMessage `json:"terms"`
}

type saveFeeRuleRequest struct {
	Version                     string `json:"version"`
	Currency                    string `json:"currency"`
	ServiceTier                 string `json:"service_tier"`
	PercentageBasisPoints       int64  `json:"percentage_basis_points"`
	MinimumMonthlyFeeMinorUnits int64  `json:"minimum_monthly_fee_minor_units"`
	SetupFeeMinorUnits          int64  `json:"setup_fee_minor_units"`
	EffectiveFrom               string `json:"effective_from,omitempty"`
	EffectiveTo                 string `json:"effective_to,omitempty"`
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

func (h *Handler) handleQuote(w http.ResponseWriter, r *http.Request) {
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

	var req quoteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.TenantID == "" {
		req.TenantID = tenantID
	}
	if req.RuleVersion == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "rule_version is required")
		return
	}

	inputs := QuoteInputs{
		TenantID:                       req.TenantID,
		PropertyID:                     req.PropertyID,
		ServiceTier:                    req.ServiceTier,
		ManagedUnits:                   req.ManagedUnits,
		Currency:                       req.Currency,
		RevenuePeriod:                  req.RevenuePeriod,
		AccommodationRevenueMinorUnits: req.AccommodationRevenueMinorUnits,
		PassThroughs:                   req.PassThroughs,
		IncludedPassThroughs:           req.IncludedPassThroughs,
	}

	quote, err := h.svc.Quote(r.Context(), tenantID, inputs, req.RuleVersion)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrQuoteInputsInvalid):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrFeeRuleNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, quote)
}

func (h *Handler) handleCreateAgreement(w http.ResponseWriter, r *http.Request) {
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

	var req createAgreementRequest
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
	if len(req.Terms) == 0 {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "terms are required")
		return
	}

	created, err := h.svc.CreateAgreement(r.Context(), CreateAgreementParams{
		TenantID:   req.TenantID,
		PropertyID: req.PropertyID,
		Terms:      req.Terms,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrEmptyTerms):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, agreementResource(created))
}

func (h *Handler) handleListAgreements(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	agreements, err := h.svc.ListAgreements(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]apiResource, 0, len(agreements))
	for _, a := range agreements {
		aa := a
		items = append(items, agreementResource(&aa))
	}

	writeJSON(w, http.StatusOK, apiCollection{Items: items})
}

func (h *Handler) handleGetAgreement(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	agreementID := r.PathValue("agreement_id")

	a, err := h.svc.GetAgreement(r.Context(), tenantID, agreementID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrAgreementNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, a.Version))
	writeJSON(w, http.StatusOK, agreementResource(a))
}

func (h *Handler) handleAddVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	agreementID := r.PathValue("agreement_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req addVersionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if len(req.Terms) == 0 {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "terms are required")
		return
	}

	updated, err := h.svc.AddVersion(r.Context(), tenantID, agreementID, req.Terms, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrAcceptedImmutable):
			code = "INVALID_STATE"
			status = http.StatusConflict
		case errors.Is(err, ErrEmptyTerms):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrAgreementNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, updated.Version))
	writeJSON(w, http.StatusOK, agreementResource(updated))
}

func (h *Handler) handleAccept(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	agreementID := r.PathValue("agreement_id")

	accepted, err := h.svc.Accept(r.Context(), tenantID, agreementID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrAlreadyAccepted):
			code = "INVALID_STATE"
			status = http.StatusConflict
		case errors.Is(err, ErrAgreementNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, accepted.Version))
	writeJSON(w, http.StatusOK, agreementResource(accepted))
}

func (h *Handler) handleSaveFeeRule(w http.ResponseWriter, r *http.Request) {
	_, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req saveFeeRuleRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	rule := &FeeRule{
		Version:                     req.Version,
		Currency:                    req.Currency,
		ServiceTier:                 req.ServiceTier,
		PercentageBasisPoints:       req.PercentageBasisPoints,
		MinimumMonthlyFeeMinorUnits: req.MinimumMonthlyFeeMinorUnits,
		SetupFeeMinorUnits:          req.SetupFeeMinorUnits,
		EffectiveFrom:               req.EffectiveFrom,
		EffectiveTo:                 req.EffectiveTo,
	}

	if err := h.svc.SaveFeeRule(r.Context(), rule); err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidFeeRule) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": rule.ID, "version": rule.Version})
}

func agreementResource(a *Agreement) apiResource {
	versionData := make([]map[string]any, 0, len(a.Versions))
	for _, v := range a.Versions {
		var terms any
		if err := json.Unmarshal(v.Terms, &terms); err != nil {
			terms = string(v.Terms)
		}
		versionData = append(versionData, map[string]any{
			"id":             v.ID,
			"version_number": v.VersionNumber,
			"content_hash":   v.ContentHash,
			"terms":          terms,
			"created_at":     v.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	data := map[string]any{
		"tenant_id":       a.TenantID,
		"property_id":     a.PropertyID,
		"status":          a.Status,
		"current_version": a.CurrentVersion,
		"versions":        versionData,
		"created_at":      a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":      a.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if a.Acceptance != nil {
		data["acceptance"] = map[string]any{
			"id":             a.Acceptance.ID,
			"version_number": a.Acceptance.VersionNumber,
			"content_hash":   a.Acceptance.ContentHash,
			"accepted_by":    a.Acceptance.AcceptedBy,
			"accepted_at":    a.Acceptance.AcceptedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return apiResource{
		ID:      a.ID,
		Version: a.Version,
		Data:    data,
	}
}
