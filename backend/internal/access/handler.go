package access

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
	mux.HandleFunc("POST /v1/properties/{property_id}/access-secrets", h.handleStoreSecret)
	mux.HandleFunc("GET /v1/properties/{property_id}/access-secrets", h.handleListSecrets)
	mux.HandleFunc("POST /v1/properties/{property_id}/access-grants", h.handleCreateGrant)
	mux.HandleFunc("GET /v1/properties/{property_id}/access-grants", h.handleListGrants)
	mux.HandleFunc("GET /v1/access-grants/{grant_id}", h.handleGetGrant)
	mux.HandleFunc("POST /v1/access-grants/{grant_id}/disclose", h.handleDiscloseSecret)
	mux.HandleFunc("POST /v1/access-grants/{grant_id}/acknowledge", h.handleAcknowledge)
	mux.HandleFunc("POST /v1/access-grants/{grant_id}/return", h.handleReturn)
	mux.HandleFunc("POST /v1/access-grants/{grant_id}/revoke", h.handleRevoke)
	mux.HandleFunc("POST /v1/properties/{property_id}/emergency-access", h.handleEmergencyAccess)
	mux.HandleFunc("GET /v1/properties/{property_id}/access-custody-events", h.handleListCustodyEvents)
	mux.HandleFunc("POST /v1/properties/{property_id}/access-holds", h.handlePlaceHold)
	mux.HandleFunc("DELETE /v1/access-holds/{hold_id}", h.handleReleaseHold)
	mux.HandleFunc("GET /v1/access-grants/{grant_id}/disclosures", h.handleListDisclosures)
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

func parseRFC3339Required(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("required time field is empty")
	}
	return time.Parse(time.RFC3339, s)
}

func secretView(s *PropertyAccessSecret) map[string]any {
	return map[string]any{
		"id":                s.ID,
		"tenant_id":         s.TenantID,
		"property_id":       s.PropertyID,
		"secret_type":       s.SecretType,
		"label":             s.Label,
		"encryption_key_id": s.EncryptionKeyID,
		"metadata":          s.Metadata,
		"version":           s.Version,
		"created_at":        s.CreatedAt.Format(time.RFC3339),
		"updated_at":        s.UpdatedAt.Format(time.RFC3339),
	}
}

func grantView(g *AccessGrant) map[string]any {
	m := map[string]any{
		"id":               g.ID,
		"tenant_id":        g.TenantID,
		"property_id":      g.PropertyID,
		"secret_id":        g.SecretID,
		"grantee_id":       g.GranteeID,
		"granter_id":       g.GranterID,
		"window_start":     g.WindowStart.Format(time.RFC3339),
		"window_end":       g.WindowEnd.Format(time.RFC3339),
		"reason":           g.Reason,
		"status":           g.Status,
		"is_emergency":     g.IsEmergency,
		"emergency_reason": g.EmergencyReason,
		"version":          g.Version,
		"created_at":       g.CreatedAt.Format(time.RFC3339),
		"updated_at":       g.UpdatedAt.Format(time.RFC3339),
	}
	if g.AcknowledgedAt != nil {
		m["acknowledged_at"] = g.AcknowledgedAt.Format(time.RFC3339)
	}
	if g.ReturnedAt != nil {
		m["returned_at"] = g.ReturnedAt.Format(time.RFC3339)
	}
	if g.RevokedAt != nil {
		m["revoked_at"] = g.RevokedAt.Format(time.RFC3339)
		m["revoked_by"] = g.RevokedBy
		m["revoke_reason"] = g.RevokeReason
	}
	return m
}

func custodyEventView(e *AccessCustodyEvent) map[string]any {
	return map[string]any{
		"id":          e.ID,
		"tenant_id":   e.TenantID,
		"property_id": e.PropertyID,
		"grant_id":    e.GrantID,
		"secret_id":   e.SecretID,
		"event_type":  e.EventType,
		"actor_id":    e.ActorID,
		"grantee_id":  e.GranteeID,
		"reason":      e.Reason,
		"metadata":    e.Metadata,
		"created_at":  e.CreatedAt.Format(time.RFC3339),
	}
}

func holdView(h *AccessHold) map[string]any {
	m := map[string]any{
		"id":          h.ID,
		"tenant_id":   h.TenantID,
		"property_id": h.PropertyID,
		"reason":      h.Reason,
		"placed_by":   h.PlacedBy,
		"status":      h.Status,
		"created_at":  h.CreatedAt.Format(time.RFC3339),
		"updated_at":  h.UpdatedAt.Format(time.RFC3339),
	}
	if h.ReleasedAt != nil {
		m["released_at"] = h.ReleasedAt.Format(time.RFC3339)
		m["released_by"] = h.ReleasedBy
	}
	return m
}

func disclosureView(d *AccessDisclosure) map[string]any {
	return map[string]any{
		"id":            d.ID,
		"grant_id":      d.GrantID,
		"tenant_id":     d.TenantID,
		"property_id":   d.PropertyID,
		"secret_id":     d.SecretID,
		"requestor_id":  d.RequestorID,
		"result":        d.Result,
		"denial_reason": d.DenialReason,
		"disclosed_at":  d.DisclosedAt.Format(time.RFC3339),
	}
}

type storeSecretRequest struct {
	SecretType      string `json:"secret_type"`
	Label           string `json:"label,omitempty"`
	EncryptedValue  string `json:"encrypted_value"`
	EncryptionKeyID string `json:"encryption_key_id,omitempty"`
	Metadata        string `json:"metadata,omitempty"`
}

type createGrantRequest struct {
	SecretID    string `json:"secret_id"`
	GranteeID   string `json:"grantee_id"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	Reason      string `json:"reason,omitempty"`
}

type revokeGrantRequest struct {
	Reason string `json:"reason"`
}

type emergencyAccessRequest struct {
	Reason      string `json:"reason"`
	WindowStart string `json:"window_start,omitempty"`
	WindowEnd   string `json:"window_end,omitempty"`
}

type placeHoldRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) handleStoreSecret(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req storeSecretRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	sec, err := h.svc.StoreSecret(r.Context(), tenantID, propertyID, CreateSecretParams{
		SecretType:      req.SecretType,
		Label:           req.Label,
		EncryptedValue:  req.EncryptedValue,
		EncryptionKeyID: req.EncryptionKeyID,
		Metadata:        req.Metadata,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidSecret) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      sec.ID,
		Version: sec.Version,
		Data:    secretView(sec),
	})
}

func (h *Handler) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	secrets, err := h.svc.ListSecrets(r.Context(), tenantID, propertyID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if secrets == nil {
		secrets = []PropertyAccessSecret{}
	}

	resources := make([]apiResource, 0, len(secrets))
	for i := range secrets {
		resources = append(resources, apiResource{
			ID:      secrets[i].ID,
			Version: secrets[i].Version,
			Data:    secretView(&secrets[i]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *Handler) handleCreateGrant(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createGrantRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	windowStart, err := parseRFC3339Required(req.WindowStart)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid window_start, use RFC3339")
		return
	}
	windowEnd, err := parseRFC3339Required(req.WindowEnd)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid window_end, use RFC3339")
		return
	}

	grant, err := h.svc.CreateGrant(r.Context(), tenantID, propertyID, req.SecretID, CreateGrantParams{
		GranteeID:   req.GranteeID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Reason:      req.Reason,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidGrant) || errors.Is(err, ErrInvalidWindow) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrSecretNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrAccessHeld) {
			status = http.StatusUnprocessableEntity
			code = "ACCESS_HELD"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      grant.ID,
		Version: grant.Version,
		Data:    grantView(grant),
	})
}

func (h *Handler) handleListGrants(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	grants, err := h.svc.ListGrants(r.Context(), tenantID, propertyID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if grants == nil {
		grants = []AccessGrant{}
	}

	resources := make([]apiResource, 0, len(grants))
	for i := range grants {
		resources = append(resources, apiResource{
			ID:      grants[i].ID,
			Version: grants[i].Version,
			Data:    grantView(&grants[i]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *Handler) handleGetGrant(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	grantID := r.PathValue("grant_id")
	grant, err := h.svc.GetGrant(r.Context(), tenantID, grantID)
	if err != nil {
		if errors.Is(err, ErrGrantNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "grant not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      grant.ID,
		Version: grant.Version,
		Data:    grantView(grant),
	})
}

func (h *Handler) handleDiscloseSecret(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	grantID := r.PathValue("grant_id")
	now := time.Now().UTC()

	sec, disclosure, err := h.svc.DiscloseSecret(r.Context(), tenantID, grantID, actorID, now)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrGrantNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		case errors.Is(err, ErrGrantNotActive), errors.Is(err, ErrGrantAlreadyRevoked):
			status = http.StatusGone
			code = "GRANT_NOT_ACTIVE"
		case errors.Is(err, ErrGrantWindowMismatch):
			status = http.StatusForbidden
			code = "OUT_OF_WINDOW"
		case errors.Is(err, ErrAccessHeld):
			status = http.StatusUnprocessableEntity
			code = "ACCESS_HELD"
		case errors.Is(err, ErrUnauthorized):
			status = http.StatusForbidden
			code = "UNAUTHORIZED_DISCLOSURE"
		}

		response := map[string]any{
			"error": apiError{
				RequestID: r.Header.Get("X-Correlation-ID"),
				Code:      code,
				Message:   err.Error(),
			},
		}
		if disclosure != nil {
			response["disclosure"] = disclosureView(disclosure)
		}
		writeJSON(w, status, response)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"grant":        grantView(&AccessGrant{ID: grantID, TenantID: tenantID}),
		"disclosure":   disclosureView(disclosure),
		"secret_value": sec.EncryptedValue,
		"secret_type":  sec.SecretType,
	})
}

func (h *Handler) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	grantID := r.PathValue("grant_id")
	grant, err := h.svc.AcknowledgeAccess(r.Context(), tenantID, grantID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrGrantNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrGrantAlreadyAcknowledged) || errors.Is(err, ErrGrantNotActive) {
			status = http.StatusConflict
			code = "INVALID_STATE"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      grant.ID,
		Version: grant.Version,
		Data:    grantView(grant),
	})
}

func (h *Handler) handleReturn(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	grantID := r.PathValue("grant_id")
	grant, err := h.svc.ReturnAccess(r.Context(), tenantID, grantID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrGrantNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrGrantAlreadyReturned) || errors.Is(err, ErrGrantNotActive) {
			status = http.StatusConflict
			code = "INVALID_STATE"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      grant.ID,
		Version: grant.Version,
		Data:    grantView(grant),
	})
}

func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	grantID := r.PathValue("grant_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req revokeGrantRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Reason == "" {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "revocation reason is required")
		return
	}

	grant, err := h.svc.RevokeGrant(r.Context(), tenantID, grantID, actorID, req.Reason)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrGrantNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      grant.ID,
		Version: grant.Version,
		Data:    grantView(grant),
	})
}

func (h *Handler) handleEmergencyAccess(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req emergencyAccessRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var windowStart, windowEnd time.Time
	if req.WindowStart != "" {
		windowStart, err = parseRFC3339(req.WindowStart)
		if err != nil {
			writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid window_start, use RFC3339")
			return
		}
	}
	if req.WindowEnd != "" {
		windowEnd, err = parseRFC3339(req.WindowEnd)
		if err != nil {
			writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid window_end, use RFC3339")
			return
		}
	}

	grant, secret, err := h.svc.EmergencyAccess(r.Context(), tenantID, propertyID, EmergencyAccessParams{
		Reason:      req.Reason,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidEmergency) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrSecretNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"grant_id":     grant.ID,
		"tenant_id":    grant.TenantID,
		"property_id":  grant.PropertyID,
		"is_emergency": true,
		"reason":       grant.Reason,
		"secret_type":  secret.SecretType,
		"secret_value": secret.EncryptedValue,
		"window_start": grant.WindowStart.Format(time.RFC3339),
		"window_end":   grant.WindowEnd.Format(time.RFC3339),
		"created_at":   grant.CreatedAt.Format(time.RFC3339),
	})
}

func (h *Handler) handleListCustodyEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	events, err := h.svc.ListCustodyEvents(r.Context(), tenantID, propertyID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if events == nil {
		events = []AccessCustodyEvent{}
	}

	items := make([]map[string]any, 0, len(events))
	for i := range events {
		items = append(items, custodyEventView(&events[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func (h *Handler) handlePlaceHold(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req placeHoldRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	hold, err := h.svc.PlaceHold(r.Context(), tenantID, propertyID, CreateHoldParams{
		Reason: req.Reason,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidSecret) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      hold.ID,
		Version: 1,
		Data:    holdView(hold),
	})
}

func (h *Handler) handleReleaseHold(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	holdID := r.PathValue("hold_id")
	hold, err := h.svc.ReleaseHold(r.Context(), tenantID, holdID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrHoldNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrHoldAlreadyReleased) {
			status = http.StatusConflict
			code = "ALREADY_RELEASED"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      hold.ID,
		Version: 1,
		Data:    holdView(hold),
	})
}

func (h *Handler) handleListDisclosures(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	grantID := r.PathValue("grant_id")
	disclosures, err := h.svc.ListDisclosures(r.Context(), tenantID, grantID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if disclosures == nil {
		disclosures = []AccessDisclosure{}
	}

	items := make([]map[string]any, 0, len(disclosures))
	for i := range disclosures {
		items = append(items, disclosureView(&disclosures[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}
