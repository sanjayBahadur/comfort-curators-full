package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"comfort-curators-backend/internal/contracts"
	"comfort-curators-backend/internal/onboarding"
	"comfort-curators-backend/internal/property"
)

type PropertySliceHandler struct {
	svc         PropService
	authorityFn OwnerAuthorities
}

func NewPropertySliceHandler(svc PropService, authorityFn OwnerAuthorities) *PropertySliceHandler {
	return &PropertySliceHandler{svc: svc, authorityFn: authorityFn}
}

func (h *PropertySliceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/properties", h.handleCreateProperty)
	mux.HandleFunc("GET /v1/properties", h.handleListProperties)
	mux.HandleFunc("GET /v1/properties/{property_id}", h.handleGetProperty)
	mux.HandleFunc("POST /v1/properties/{property_id}/transitions", h.handleTransitionProperty)
	mux.HandleFunc("GET /v1/properties/{property_id}/transitions", h.handleListTransitions)
	mux.HandleFunc("PUT /v1/properties/{property_id}/readiness", h.handleSetReadiness)
	mux.HandleFunc("POST /v1/properties/{property_id}/compliance-holds", h.handleAddComplianceHold)
	mux.HandleFunc("POST /v1/properties/{property_id}/compliance-holds/{hold_id}/resolve", h.handleResolveComplianceHold)
	mux.HandleFunc("POST /v1/properties/{property_id}/compliance-holds/{hold_id}/exception", h.handleGrantComplianceException)
	mux.HandleFunc("POST /v1/properties/{property_id}/access-disclosures", h.handleDiscloseAccess)
}

type createPropertyRequest struct {
	TenantID          string                      `json:"tenant_id"`
	OwnerAuthorityID  string                      `json:"owner_authority_id"`
	ServiceAddress    property.Address            `json:"service_address"`
	GeolocationZone   string                      `json:"geolocation_zone,omitempty"`
	Timezone          string                      `json:"timezone"`
	AccessMethod      string                      `json:"access_method"`
	Status            string                      `json:"status,omitempty"`
	MaximumOccupancy  int                         `json:"maximum_occupancy,omitempty"`
	EmergencyContacts []property.EmergencyContact `json:"emergency_contacts,omitempty"`
}

type transitionPropertyRequest struct {
	ToState string `json:"to_state"`
	Reason  string `json:"reason"`
}

type setReadinessRequest struct {
	OwnerContractAccepted bool `json:"owner_contract_accepted"`
	ComplianceComplete    bool `json:"compliance_complete"`
	MandatoryFieldsSet    bool `json:"mandatory_fields_set"`
}

type addComplianceHoldRequest struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

type grantExceptionRequest struct {
	ReviewerID string `json:"reviewer_id"`
	Reason     string `json:"reason"`
	TTLHours   int    `json:"ttl_hours"`
}

func (h *PropertySliceHandler) handleCreateProperty(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createPropertyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.OwnerAuthorityID == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "owner_authority_id is required")
		return
	}
	if req.ServiceAddress.Line1 == "" || req.ServiceAddress.City == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "service_address.line1 and service_address.city are required")
		return
	}

	params := property.CreatePropertyParams{
		TenantID:          subject.TenantID,
		OwnerAuthorityID:  req.OwnerAuthorityID,
		ServiceAddress:    req.ServiceAddress,
		GeolocationZone:   req.GeolocationZone,
		Timezone:          req.Timezone,
		EmergencyContacts: req.EmergencyContacts,
		AccessMethod:      req.AccessMethod,
		MaximumOccupancy:  req.MaximumOccupancy,
		InitialState:      req.Status,
	}

	p, err := h.svc.CreateProperty(r.Context(), params, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, property.ErrInvalidState) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	view := OrdinaryProperty(*p)
	writeETag(w, p.Version)
	writeResource(w, http.StatusCreated, p.ID, p.Version, view)
}

func (h *PropertySliceHandler) handleListProperties(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, parseErr := strconv.Atoi(l); parseErr == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	props, err := h.svc.ListProperties(r.Context(), subject.TenantID)
	if err != nil {
		if errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	filtered := OwnedProperties(subject, props, h.authorityFn)

	items := make([]Resource, 0, len(filtered))
	start := 0
	if cursor != "" {
		for i, p := range filtered {
			if p.ID == cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	for _, p := range filtered[start:end] {
		view := OrdinaryProperty(p)
		items = append(items, Resource{ID: p.ID, Version: p.Version, Data: view})
	}

	var nextCursor *string
	if end < len(filtered) {
		c := filtered[end].ID
		nextCursor = &c
	}

	writeCollection(w, items, nextCursor)
}

func (h *PropertySliceHandler) handleGetProperty(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	p, err := h.svc.GetProperty(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) || errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if !OwnsProperty(subject, *p, h.authorityFn) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	view := OrdinaryProperty(*p)
	writeETag(w, p.Version)
	writeResource(w, http.StatusOK, p.ID, p.Version, view)
}

func (h *PropertySliceHandler) handleTransitionProperty(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req transitionPropertyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.ToState == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "to_state is required")
		return
	}

	p, err := h.svc.TransitionProperty(r.Context(), subject.TenantID, propertyID, req.ToState, req.Reason, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError

		switch {
		case errors.Is(err, property.ErrInvalidState):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrInvalidTransition):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrArchivedTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrComplianceHold):
			code = "COMPLIANCE_HOLD"
			status = http.StatusConflict
		case errors.Is(err, property.ErrNotReady):
			code = "NOT_READY"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, property.ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case err.Error() == "property state update lost a concurrent write (optimistic version)":
			code = "CONCURRENT_MODIFICATION"
			status = http.StatusConflict
		}

		writeError(w, r, status, code, err.Error())
		return
	}

	view := OrdinaryProperty(*p)
	writeETag(w, p.Version)
	writeResource(w, http.StatusOK, p.ID, p.Version, view)
}

func (h *PropertySliceHandler) handleListTransitions(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	transitions, err := h.svc.ListTransitions(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) || errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	type transData struct {
		ID          string `json:"id"`
		PropertyID  string `json:"property_id"`
		FromState   string `json:"from_state"`
		ToState     string `json:"to_state"`
		ActorID     string `json:"actor_id"`
		Reason      string `json:"reason"`
		FromVersion int    `json:"from_version"`
		ToVersion   int    `json:"to_version"`
		CreatedAt   string `json:"created_at"`
	}

	items := make([]Resource, 0, len(transitions))
	for _, t := range transitions {
		items = append(items, Resource{
			ID:      t.ID,
			Version: 1,
			Data: transData{
				ID:          t.ID,
				PropertyID:  t.PropertyID,
				FromState:   t.FromState,
				ToState:     t.ToState,
				ActorID:     t.ActorID,
				Reason:      t.Reason,
				FromVersion: t.FromVersion,
				ToVersion:   t.ToVersion,
				CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			},
		})
	}

	writeCollection(w, items, nil)
}

func (h *PropertySliceHandler) handleSetReadiness(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req setReadinessRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	p, err := h.svc.SetReadiness(r.Context(), subject.TenantID, propertyID, property.Readiness{
		OwnerContractAccepted: req.OwnerContractAccepted,
		ComplianceComplete:    req.ComplianceComplete,
		MandatoryFieldsSet:    req.MandatoryFieldsSet,
	}, subject.ActorID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	view := OrdinaryProperty(*p)
	writeETag(w, p.Version)
	writeResource(w, http.StatusOK, p.ID, p.Version, view)
}

func (h *PropertySliceHandler) handleAddComplianceHold(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req addComplianceHoldRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	hold, err := h.svc.AddComplianceHold(r.Context(), subject.TenantID, propertyID, property.ComplianceHoldParams{
		Kind:     req.Kind,
		Severity: req.Severity,
		Reason:   req.Reason,
	}, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, property.ErrInvalidComplianceHold):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusCreated, hold.ID, 1, holdMapView(*hold))
}

func (h *PropertySliceHandler) handleResolveComplianceHold(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	holdID := r.PathValue("hold_id")

	p, err := h.svc.ResolveComplianceHold(r.Context(), subject.TenantID, propertyID, holdID, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, property.ErrHoldNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, property.ErrHoldNotOpen):
			code = "INVALID_STATE"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	view := OrdinaryProperty(*p)
	writeETag(w, p.Version)
	writeResource(w, http.StatusOK, p.ID, p.Version, view)
}

func (h *PropertySliceHandler) handleGrantComplianceException(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	holdID := r.PathValue("hold_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req grantExceptionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.TTLHours <= 0 {
		req.TTLHours = 24
	}

	p, err := h.svc.GrantComplianceException(r.Context(), subject.TenantID, propertyID, holdID, req.ReviewerID, req.Reason, durationHours(req.TTLHours), subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, property.ErrHoldNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, property.ErrHoldNotOpen):
			code = "INVALID_STATE"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrExceptionDenied):
			code = "EXCEPTION_DENIED"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, property.ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	view := OrdinaryProperty(*p)
	writeETag(w, p.Version)
	writeResource(w, http.StatusOK, p.ID, p.Version, view)
}

func (h *PropertySliceHandler) handleDiscloseAccess(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	p, err := h.svc.GetProperty(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) || errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if !OwnsProperty(subject, *p, h.authorityFn) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	if !hasRole(subject.Roles, RoleOwner) && !hasRole(subject.Roles, "staff") {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "access disclosure requires owner or staff role")
		return
	}

	writeJSON(w, http.StatusOK, AccessMaterialOnly(*p))
}

func holdMapView(h property.ComplianceHold) map[string]any {
	m := map[string]any{
		"id":          h.ID,
		"property_id": h.PropertyID,
		"kind":        h.Kind,
		"severity":    h.Severity,
		"status":      h.Status,
		"reason":      h.Reason,
		"created_at":  h.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if h.ExpiresAt != nil {
		m["expires_at"] = h.ExpiresAt.Format("2006-01-02T15:04:05Z")
	}
	if h.ExceptionBy != "" {
		m["exception_by"] = h.ExceptionBy
	}
	if h.ExceptionAt != nil {
		m["exception_at"] = h.ExceptionAt.Format("2006-01-02T15:04:05Z")
	}
	if h.ExceptionExpiresAt != nil {
		m["exception_expires_at"] = h.ExceptionExpiresAt.Format("2006-01-02T15:04:05Z")
	}
	if h.ResolvedAt != nil {
		m["resolved_at"] = h.ResolvedAt.Format("2006-01-02T15:04:05Z")
	}
	return m
}

func hasRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}

func durationHours(h int) time.Duration {
	return time.Duration(h) * time.Hour
}

type OnboardingSliceHandler struct {
	svc         *onboarding.Service
	propSvc     *property.PropertyService
	authorityFn OwnerAuthorities
}

func NewOnboardingSliceHandler(svc *onboarding.Service, propSvc *property.PropertyService, authorityFn OwnerAuthorities) *OnboardingSliceHandler {
	return &OnboardingSliceHandler{svc: svc, propSvc: propSvc, authorityFn: authorityFn}
}

func (h *OnboardingSliceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/owners/onboarding-cases", h.handleStartOwnerOnboarding)
	mux.HandleFunc("POST /v1/properties/{property_id}/inspections", h.handleRecordPropertyInspection)
}

type startOwnerOnboardingRequest struct {
	TenantID         string `json:"tenant_id"`
	PropertyID       string `json:"property_id"`
	OwnerAuthorityID string `json:"owner_authority_id"`
}

func (h *OnboardingSliceHandler) handleStartOwnerOnboarding(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if !IsOwner(subject) {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "only owners can start onboarding")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req startOwnerOnboardingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.TenantID == "" {
		req.TenantID = subject.TenantID
	}
	if req.PropertyID == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id is required")
		return
	}
	if req.OwnerAuthorityID == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "owner_authority_id is required")
		return
	}

	p, err := h.propSvc.GetProperty(r.Context(), subject.TenantID, req.PropertyID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) || errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if !OwnsProperty(subject, *p, h.authorityFn) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	started, err := h.svc.StartCase(r.Context(), onboarding.StartCaseParams{
		TenantID:         req.TenantID,
		PropertyID:       req.PropertyID,
		OwnerAuthorityID: req.OwnerAuthorityID,
	}, subject.ActorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	data := onboardingCaseView(started)
	writeETag(w, started.Version)
	writeResource(w, http.StatusCreated, started.ID, started.Version, data)
}

type recordInspectionRequest struct {
	InspectedBy   string `json:"inspected_by"`
	EvidenceHash  string `json:"evidence_hash"`
	EvidenceRef   string `json:"evidence_ref,omitempty"`
	Findings      string `json:"findings"`
	OverallStatus string `json:"overall_status"`
}

func (h *OnboardingSliceHandler) handleRecordPropertyInspection(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

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

	if req.InspectedBy == "" || req.EvidenceHash == "" || req.OverallStatus == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "inspected_by, evidence_hash and overall_status are required")
		return
	}

	p, err := h.propSvc.GetProperty(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) || errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if !OwnsProperty(subject, *p, h.authorityFn) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	cases, err := h.svc.ListCases(r.Context(), subject.TenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var caseID string
	for _, c := range cases {
		if c.PropertyID == propertyID {
			caseID = c.ID
			break
		}
	}
	if caseID == "" {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "no onboarding case found for this property")
		return
	}

	insp, err := h.svc.RecordInspection(r.Context(), subject.TenantID, caseID, onboarding.InspectionParams{
		PropertyID:    propertyID,
		InspectedBy:   req.InspectedBy,
		EvidenceHash:  req.EvidenceHash,
		EvidenceRef:   req.EvidenceRef,
		Findings:      req.Findings,
		OverallStatus: req.OverallStatus,
	}, subject.ActorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, onboarding.ErrInvalidInspection):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, onboarding.ErrCaseNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, onboarding.ErrCaseActivated):
			code = "INVALID_STATE"
			status = http.StatusConflict
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeResource(w, http.StatusCreated, insp.ID, 1, insp)
}

func onboardingCaseView(c *onboarding.Case) map[string]any {
	data := map[string]any{
		"tenant_id":           c.TenantID,
		"property_id":         c.PropertyID,
		"owner_authority_id":  c.OwnerAuthorityID,
		"status":              c.Status,
		"portfolio":           c.Portfolio,
		"goals":               c.Goals,
		"service_preferences": c.ServicePreferences,
		"budgets":             c.Budgets,
		"contacts":            c.Contacts,
		"photographs":         c.Photographs,
		"amenities":           c.Amenities,
		"safety":              c.Safety,
		"furnishing":          c.Furnishing,
		"remediation":         c.Remediation,
		"fit_score_inputs":    c.FitScoreInputs,
		"evidence":            c.Evidence,
		"inspections":         c.Inspections,
		"activation_holds":    c.Holds,
		"created_at":          c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":          c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if data["evidence"] == nil {
		data["evidence"] = []onboarding.Evidence{}
	}
	if data["inspections"] == nil {
		data["inspections"] = []onboarding.Inspection{}
	}
	if data["contacts"] == nil {
		data["contacts"] = []onboarding.Contact{}
	}
	if data["activation_holds"] == nil {
		data["activation_holds"] = []onboarding.ActivationHold{}
	}
	return data
}

type ContractSliceHandler struct {
	svc         *contracts.Service
	propSvc     *property.PropertyService
	authorityFn OwnerAuthorities
}

func NewContractSliceHandler(svc *contracts.Service, propSvc *property.PropertyService, authorityFn OwnerAuthorities) *ContractSliceHandler {
	return &ContractSliceHandler{svc: svc, propSvc: propSvc, authorityFn: authorityFn}
}

func (h *ContractSliceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/properties/{property_id}/contracts", h.handleCreatePropertyContract)
}

type createPropertyContractRequest struct {
	TenantID string          `json:"tenant_id"`
	Terms    json.RawMessage `json:"terms"`
}

func (h *ContractSliceHandler) handleCreatePropertyContract(w http.ResponseWriter, r *http.Request) {
	subject, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	if !IsOwner(subject) {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "only owners can create property contracts")
		return
	}

	p, err := h.propSvc.GetProperty(r.Context(), subject.TenantID, propertyID)
	if err != nil {
		if errors.Is(err, property.ErrPropertyNotFound) || errors.Is(err, property.ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if !OwnsProperty(subject, *p, h.authorityFn) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createPropertyContractRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.TenantID == "" {
		req.TenantID = subject.TenantID
	}
	if len(req.Terms) == 0 {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "terms are required")
		return
	}

	created, err := h.svc.CreateAgreement(r.Context(), contracts.CreateAgreementParams{
		TenantID:   req.TenantID,
		PropertyID: propertyID,
		Terms:      req.Terms,
	}, subject.ActorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, contracts.ErrEmptyTerms):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, contracts.ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	data := agreementView(created)
	writeETag(w, created.Version)
	writeResource(w, http.StatusCreated, created.ID, created.Version, data)
}

func agreementView(a *contracts.Agreement) map[string]any {
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

	return data
}
