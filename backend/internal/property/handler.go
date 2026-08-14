package property

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"comfort-curators-backend/internal/iam"
)

type PropertyHandler struct {
	svc *PropertyService
}

func NewPropertyHandler(svc *PropertyService) *PropertyHandler {
	return &PropertyHandler{svc: svc}
}

func (h *PropertyHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/properties", h.handleCreateProperty)
	mux.HandleFunc("GET /v1/properties", h.handleListProperties)
	mux.HandleFunc("GET /v1/properties/{property_id}", h.handleGetProperty)
	mux.HandleFunc("POST /v1/properties/{property_id}/transitions", h.handleTransitionProperty)
	mux.HandleFunc("GET /v1/properties/{property_id}/transitions", h.handleListTransitions)
	mux.HandleFunc("PUT /v1/properties/{property_id}/readiness", h.handleSetReadiness)
	mux.HandleFunc("POST /v1/properties/{property_id}/compliance-holds", h.handleAddComplianceHold)
	mux.HandleFunc("POST /v1/properties/{property_id}/compliance-holds/{hold_id}/resolve", h.handleResolveComplianceHold)
	mux.HandleFunc("POST /v1/properties/{property_id}/compliance-holds/{hold_id}/exception", h.handleGrantComplianceException)
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
	NextCursor *string       `json:"next_cursor"`
}

type createPropertyRequest struct {
	IdempotencyKey    string             `json:"idempotency_key"`
	TenantID          string             `json:"tenant_id"`
	OwnerAuthorityID  string             `json:"owner_authority_id"`
	ServiceAddress    Address            `json:"service_address"`
	Timezone          string             `json:"timezone"`
	Status            string             `json:"status,omitempty"`
	MaximumOccupancy  int                `json:"maximum_occupancy,omitempty"`
	EmergencyContacts []EmergencyContact `json:"emergency_contacts,omitempty"`
}

type transitionPropertyRequest struct {
	IdempotencyKey string   `json:"idempotency_key"`
	ToState        string   `json:"to_state"`
	Reason         string   `json:"reason"`
	EvidenceIDs    []string `json:"evidence_ids,omitempty"`
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

func propertyResource(p *Property) apiResource {
	return apiResource{
		ID:      p.ID,
		Version: p.Version,
		Data: map[string]any{
			"tenant_id":          p.TenantID,
			"owner_authority_id": p.OwnerAuthorityID,
			"service_address":    p.ServiceAddress,
			"geolocation_zone":   p.GeolocationZone,
			"timezone":           p.Timezone,
			"emergency_contacts": p.EmergencyContacts,
			"access_method":      p.AccessMethod,
			"maximum_occupancy":  p.MaximumOccupancy,
			"state":              p.State,
			"readiness":          p.Readiness,
			"compliance_holds":   holdListValue(p.ComplianceHolds),
			"created_at":         p.CreatedAt.Format(time.RFC3339),
			"updated_at":         p.UpdatedAt.Format(time.RFC3339),
		},
	}
}

func holdListValue(holds []ComplianceHold) []map[string]any {
	if holds == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(holds))
	for _, h := range holds {
		out = append(out, holdMap(h))
	}
	return out
}

func holdMap(h ComplianceHold) map[string]any {
	m := map[string]any{
		"id":          h.ID,
		"property_id": h.PropertyID,
		"kind":        h.Kind,
		"severity":    h.Severity,
		"status":      h.Status,
		"reason":      h.Reason,
		"created_at":  h.CreatedAt.Format(time.RFC3339),
	}
	if h.ExpiresAt != nil {
		m["expires_at"] = h.ExpiresAt.Format(time.RFC3339)
	}
	if h.ExceptionBy != "" {
		m["exception_by"] = h.ExceptionBy
	}
	if h.ExceptionAt != nil {
		m["exception_at"] = h.ExceptionAt.Format(time.RFC3339)
	}
	if h.ExceptionExpiresAt != nil {
		m["exception_expires_at"] = h.ExceptionExpiresAt.Format(time.RFC3339)
	}
	if h.ResolvedAt != nil {
		m["resolved_at"] = h.ResolvedAt.Format(time.RFC3339)
	}
	return m
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func (h *PropertyHandler) handleCreateProperty(w http.ResponseWriter, r *http.Request) {
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

	params := CreatePropertyParams{
		TenantID:          tenantID,
		OwnerAuthorityID:  req.OwnerAuthorityID,
		ServiceAddress:    req.ServiceAddress,
		Timezone:          req.Timezone,
		EmergencyContacts: req.EmergencyContacts,
		MaximumOccupancy:  req.MaximumOccupancy,
		InitialState:      req.Status,
	}

	p, err := h.svc.CreateProperty(r.Context(), params, actorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidState) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, propertyResource(p))
}

func (h *PropertyHandler) handleListProperties(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
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

	props, err := h.svc.ListProperties(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var items []apiResource
	start := 0
	if cursor != "" {
		for i, p := range props {
			if p.ID == cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + limit
	if end > len(props) {
		end = len(props)
	}

	for _, p := range props[start:end] {
		items = append(items, propertyResource(&p))
	}

	var nextCursor *string
	if end < len(props) {
		c := props[end].ID
		nextCursor = &c
	}

	writeJSON(w, http.StatusOK, apiCollection{
		Items:      items,
		NextCursor: nextCursor,
	})
}

func (h *PropertyHandler) handleGetProperty(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	p, err := h.svc.GetProperty(r.Context(), tenantID, propertyID)
	if err != nil {
		if errors.Is(err, ErrPropertyNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, propertyResource(p))
}

func (h *PropertyHandler) handleTransitionProperty(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "If-Match header is required")
		return
	}

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

	_, err = h.svc.TransitionProperty(r.Context(), tenantID, propertyID, req.ToState, req.Reason, actorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError

		switch {
		case errors.Is(err, ErrInvalidState):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrInvalidTransition):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrArchivedTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrComplianceHold):
			code = "COMPLIANCE_HOLD"
			status = http.StatusConflict
		case errors.Is(err, ErrNotReady):
			code = "NOT_READY"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case err.Error() == "property state update lost a concurrent write (optimistic version)":
			code = "CONCURRENT_MODIFICATION"
			status = http.StatusConflict
		}

		writeError(w, r, status, code, err.Error())
		return
	}

	p, err := h.svc.GetProperty(r.Context(), tenantID, propertyID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, p.Version))
	writeJSON(w, http.StatusOK, propertyResource(p))
}

func (h *PropertyHandler) handleListTransitions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	transitions, err := h.svc.ListTransitions(r.Context(), tenantID, propertyID)
	if err != nil {
		if errors.Is(err, ErrPropertyNotFound) || errors.Is(err, ErrCrossTenantDenied) {
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

	items := make([]apiResource, 0, len(transitions))
	for _, t := range transitions {
		items = append(items, apiResource{
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
				CreatedAt:   t.CreatedAt.Format(time.RFC3339),
			},
		})
	}

	writeJSON(w, http.StatusOK, apiCollection{Items: items})
}

func (h *PropertyHandler) handleSetReadiness(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
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

	p, err := h.svc.SetReadiness(r.Context(), tenantID, propertyID, Readiness{
		OwnerContractAccepted: req.OwnerContractAccepted,
		ComplianceComplete:    req.ComplianceComplete,
		MandatoryFieldsSet:    req.MandatoryFieldsSet,
	}, actorID)
	if err != nil {
		if errors.Is(err, ErrPropertyNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "property not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, propertyResource(p))
}

func (h *PropertyHandler) handleAddComplianceHold(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
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

	hold, err := h.svc.AddComplianceHold(r.Context(), tenantID, propertyID, ComplianceHoldParams{
		Kind:     req.Kind,
		Severity: req.Severity,
		Reason:   req.Reason,
	}, actorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrInvalidComplianceHold):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      hold.ID,
		Version: 1,
		Data:    holdMap(*hold),
	})
}

func (h *PropertyHandler) handleResolveComplianceHold(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	holdID := r.PathValue("hold_id")

	p, err := h.svc.ResolveComplianceHold(r.Context(), tenantID, propertyID, holdID, actorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrHoldNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrHoldNotOpen):
			code = "INVALID_STATE"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, propertyResource(p))
}

func (h *PropertyHandler) handleGrantComplianceException(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
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

	p, err := h.svc.GrantComplianceException(r.Context(), tenantID, propertyID, holdID, req.ReviewerID, req.Reason, time.Duration(req.TTLHours)*time.Hour, actorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrHoldNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrHoldNotOpen):
			code = "INVALID_STATE"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrExceptionDenied):
			code = "EXCEPTION_DENIED"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrPropertyNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, propertyResource(p))
}
