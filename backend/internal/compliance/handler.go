package compliance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type ComplianceHandler struct {
	svc *ComplianceService
}

func NewComplianceHandler(svc *ComplianceService) *ComplianceHandler {
	return &ComplianceHandler{svc: svc}
}

func (h *ComplianceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/compliance/items", h.handleCreateItem)
	mux.HandleFunc("GET /v1/compliance/items/{item_id}", h.handleGetItem)
	mux.HandleFunc("GET /v1/compliance/properties/{property_id}/items", h.handleListItems)
	mux.HandleFunc("POST /v1/compliance/items/{item_id}/renew", h.handleRenewItem)
	mux.HandleFunc("POST /v1/compliance/scan-expiry", h.handleScanExpiry)
	mux.HandleFunc("GET /v1/compliance/properties/{property_id}/warnings", h.handleListWarnings)
	mux.HandleFunc("POST /v1/compliance/warnings/{warning_id}/acknowledge", h.handleAcknowledgeWarning)
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

type createItemRequest struct {
	PropertyID    string   `json:"property_id"`
	Kind          string   `json:"kind"`
	Severity      string   `json:"severity"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	EffectiveDate string   `json:"effective_date"`
	ExpiryDate    string   `json:"expiry_date"`
	EvidenceIDs   []string `json:"evidence_ids,omitempty"`
}

type renewItemRequest struct {
	NewExpiryDate string   `json:"new_expiry_date"`
	EvidenceIDs   []string `json:"evidence_ids,omitempty"`
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

func subjectFromRequest(r *http.Request) (tenantID, actorID string, roles []string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", nil, errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, subject.Roles, nil
}

func hasRole(roles []string, targets ...string) bool {
	for _, r := range roles {
		for _, t := range targets {
			if r == t {
				return true
			}
		}
	}
	return false
}

func itemData(item *ComplianceItem) map[string]any {
	m := map[string]any{
		"id":             item.ID,
		"property_id":    item.PropertyID,
		"tenant_id":      item.TenantID,
		"kind":           item.Kind,
		"severity":       item.Severity,
		"name":           item.Name,
		"description":    item.Description,
		"effective_date": item.EffectiveDate.Format(time.RFC3339),
		"expiry_date":    item.ExpiryDate.Format(time.RFC3339),
		"status":         item.Status,
		"evidence_ids":   item.EvidenceIDs,
		"created_at":     item.CreatedAt.Format(time.RFC3339),
		"updated_at":     item.UpdatedAt.Format(time.RFC3339),
	}
	if item.RenewedFromID != nil {
		m["renewed_from_id"] = *item.RenewedFromID
	}
	if item.HoldID != nil {
		m["hold_id"] = *item.HoldID
	}
	return m
}

func (h *ComplianceHandler) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, roles, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if hasRole(roles, RoleJarvis, RoleSuperhost) {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "superhost cannot manage compliance items")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createItemRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	effectiveDate, err := time.Parse(time.RFC3339, req.EffectiveDate)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid effective_date format, use RFC3339")
		return
	}
	expiryDate, err := time.Parse(time.RFC3339, req.ExpiryDate)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid expiry_date format, use RFC3339")
		return
	}

	params := ComplianceItemParams{
		PropertyID:    req.PropertyID,
		Kind:          req.Kind,
		Severity:      req.Severity,
		Name:          req.Name,
		Description:   req.Description,
		EffectiveDate: effectiveDate,
		ExpiryDate:    expiryDate,
		EvidenceIDs:   req.EvidenceIDs,
	}

	item, err := h.svc.CreateItem(r.Context(), params, tenantID, actorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidComplianceItem) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      item.ID,
		Version: 1,
		Data:    itemData(item),
	})
}

func (h *ComplianceHandler) handleGetItem(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	itemID := r.PathValue("item_id")

	item, err := h.svc.GetItem(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, ErrComplianceItemNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "compliance item not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      item.ID,
		Version: 1,
		Data:    itemData(item),
	})
}

func (h *ComplianceHandler) handleListItems(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	items, err := h.svc.ListItems(r.Context(), tenantID, propertyID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if items == nil {
		items = []ComplianceItem{}
	}

	resources := make([]apiResource, 0, len(items))
	for _, item := range items {
		resources = append(resources, apiResource{
			ID:      item.ID,
			Version: 1,
			Data:    itemData(&item),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *ComplianceHandler) handleRenewItem(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, roles, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if hasRole(roles, RoleJarvis, RoleSuperhost) {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "superhost cannot renew compliance items")
		return
	}

	itemID := r.PathValue("item_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req renewItemRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	newExpiryDate, err := time.Parse(time.RFC3339, req.NewExpiryDate)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid new_expiry_date format, use RFC3339")
		return
	}

	item, err := h.svc.RenewItem(r.Context(), itemID, newExpiryDate, req.EvidenceIDs, tenantID, actorID)
	if err != nil {
		if errors.Is(err, ErrComplianceItemNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "compliance item not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      item.ID,
		Version: 1,
		Data:    itemData(item),
	})
}

func (h *ComplianceHandler) handleScanExpiry(w http.ResponseWriter, r *http.Request) {
	_, actorID, roles, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if hasRole(roles, RoleJarvis, RoleSuperhost) {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "superhost cannot trigger compliance scans")
		return
	}

	result, err := h.svc.ScanExpired(r.Context(), actorID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      fmt.Sprintf("scan-%d", time.Now().Unix()),
		Version: 1,
		Data:    result,
	})
}

func (h *ComplianceHandler) handleListWarnings(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	warnings, err := h.svc.ListRenewalWarnings(r.Context(), tenantID, propertyID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if warnings == nil {
		warnings = []ComplianceRenewalWarning{}
	}

	resources := make([]apiResource, 0, len(warnings))
	for _, w := range warnings {
		ack := (*string)(nil)
		if w.Acknowledged != nil {
			val := w.Acknowledged.Format(time.RFC3339)
			ack = &val
		}
		resources = append(resources, apiResource{
			ID:      w.ID,
			Version: 1,
			Data: map[string]any{
				"id":                 w.ID,
				"item_id":            w.ItemID,
				"property_id":        w.PropertyID,
				"tenant_id":          w.TenantID,
				"days_before_expiry": w.DaysBeforeExpiry,
				"issued_at":          w.IssuedAt.Format(time.RFC3339),
				"acknowledged_at":    ack,
				"created_at":         w.CreatedAt.Format(time.RFC3339),
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *ComplianceHandler) handleAcknowledgeWarning(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	warningID := r.PathValue("warning_id")

	if err := h.svc.AcknowledgeWarning(r.Context(), warningID); err != nil {
		if errors.Is(err, ErrComplianceRenewalNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "renewal warning not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "acknowledged"})
}
