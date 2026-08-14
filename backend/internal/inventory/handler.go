package inventory

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
	mux.HandleFunc("POST /v1/inventory/locations", h.handleCreateLocation)
	mux.HandleFunc("GET /v1/inventory/locations", h.handleListLocations)
	mux.HandleFunc("GET /v1/inventory/locations/{location_id}", h.handleGetLocation)
	mux.HandleFunc("POST /v1/inventory/locations/{location_id}/movements", h.handleRecordMovement)
	mux.HandleFunc("GET /v1/inventory/locations/{location_id}/balances/{catalog_item_id}", h.handleGetBalance)
	mux.HandleFunc("GET /v1/inventory/locations/{location_id}/movements/{catalog_item_id}", h.handleListMovements)
	mux.HandleFunc("POST /v1/inventory/counts", h.handleCreateCount)
	mux.HandleFunc("GET /v1/inventory/counts/{count_id}", h.handleGetCount)
	mux.HandleFunc("PUT /v1/inventory/counts/{count_id}/lines", h.handleUpdateCountLine)
	mux.HandleFunc("POST /v1/inventory/counts/{count_id}/review", h.handleReviewCount)
	mux.HandleFunc("POST /v1/inventory/counts/{count_id}/reconcile", h.handleReconcileCount)
}

type inventoryResource struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Data    any    `json:"data"`
}

type inventoryError struct {
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
	json.NewEncoder(w).Encode(inventoryError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiResource(w http.ResponseWriter, status int, id string, version int64, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(inventoryResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiCollection(w http.ResponseWriter, items []inventoryResource) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

func (h *Handler) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Name         string `json:"name"`
		PropertyID   string `json:"property_id"`
		LocationType string `json:"location_type"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	loc, err := h.svc.CreateLocation(r.Context(), tenantID, CreateLocationParams{
		Name:         req.Name,
		PropertyID:   req.PropertyID,
		LocationType: req.LocationType,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidLocation) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, loc.ID, loc.Version, locationView(loc))
}

func (h *Handler) handleListLocations(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	locs, err := h.svc.ListLocations(r.Context(), tenantID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]inventoryResource, 0, len(locs))
	for i := range locs {
		items = append(items, inventoryResource{
			ID:      locs[i].ID,
			Version: locs[i].Version,
			Data:    locationView(&locs[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetLocation(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	locationID := r.PathValue("location_id")
	loc, err := h.svc.GetLocation(r.Context(), tenantID, locationID)
	if err != nil {
		if errors.Is(err, ErrLocationNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "stock location not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, loc.ID, loc.Version, locationView(loc))
}

func (h *Handler) handleRecordMovement(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	locationID := r.PathValue("location_id")

	var req struct {
		CatalogItemID string `json:"catalog_item_id"`
		MovementType  string `json:"movement_type"`
		Quantity      int64  `json:"quantity"`
		ReferenceType string `json:"reference_type"`
		ReferenceID   string `json:"reference_id"`
		Reason        string `json:"reason"`
		ExpiresAt     string `json:"expires_at"`
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

	movement, err := h.svc.RecordMovement(r.Context(), tenantID, locationID, RecordMovementParams{
		CatalogItemID: req.CatalogItemID,
		MovementType:  req.MovementType,
		Quantity:      req.Quantity,
		ReferenceType: req.ReferenceType,
		ReferenceID:   req.ReferenceID,
		Reason:        req.Reason,
		ExpiresAt:     expiresAt,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidMovement) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrNegativeStock) {
			status = http.StatusUnprocessableEntity
			code = "NEGATIVE_STOCK"
		} else if errors.Is(err, ErrExpiredStockCannotIssue) {
			status = http.StatusUnprocessableEntity
			code = "EXPIRED_STOCK"
		} else if errors.Is(err, ErrLocationNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, movement.ID, 1, movementView(movement))
}

func (h *Handler) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	locationID := r.PathValue("location_id")
	catalogItemID := r.PathValue("catalog_item_id")

	balance, movements, err := h.svc.GetBalance(r.Context(), tenantID, locationID, catalogItemID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"balance":         balance,
		"movement_count":  len(movements),
		"catalog_item_id": catalogItemID,
		"location_id":     locationID,
	})
}

func (h *Handler) handleListMovements(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	locationID := r.PathValue("location_id")
	catalogItemID := r.PathValue("catalog_item_id")

	movements, err := h.svc.ListMovements(r.Context(), tenantID, locationID, catalogItemID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]inventoryResource, 0, len(movements))
	for i := range movements {
		items = append(items, inventoryResource{
			ID:      movements[i].ID,
			Version: 1,
			Data:    movementView(&movements[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleCreateCount(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		LocationID string `json:"location_id"`
		CountedBy  string `json:"counted_by"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	count, err := h.svc.CreateCount(r.Context(), tenantID, CreateCountParams{
		LocationID: req.LocationID,
		CountedBy:  req.CountedBy,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidCount) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrLocationNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, count.ID, count.Version, countView(count))
}

func (h *Handler) handleGetCount(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	countID := r.PathValue("count_id")
	count, lines, err := h.svc.GetCount(r.Context(), tenantID, countID)
	if err != nil {
		if errors.Is(err, ErrCountNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "inventory count not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	lineViews := make([]map[string]any, 0, len(lines))
	for i := range lines {
		lineViews = append(lineViews, countLineView(&lines[i]))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      count.ID,
		"version": count.Version,
		"data": map[string]any{
			"count": countView(count),
			"lines": lineViews,
		},
	})
}

func (h *Handler) handleUpdateCountLine(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	countID := r.PathValue("count_id")

	var req struct {
		CatalogItemID   string `json:"catalog_item_id"`
		CountedQuantity int64  `json:"counted_quantity"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	line, err := h.svc.UpdateCountLine(r.Context(), tenantID, countID, UpdateCountLineParams{
		CatalogItemID:   req.CatalogItemID,
		CountedQuantity: req.CountedQuantity,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrCountNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrCountAlreadyReviewed) {
			status = http.StatusConflict
			code = "ALREADY_REVIEWED"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, line.ID, 1, countLineView(line))
}

func (h *Handler) handleReviewCount(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	countID := r.PathValue("count_id")

	var req struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	count, err := h.svc.ReviewCount(r.Context(), tenantID, countID, ReviewCountParams{
		ReviewedBy: req.ReviewedBy,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrCountNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrCountAlreadyReviewed) {
			status = http.StatusConflict
			code = "ALREADY_REVIEWED"
		} else if errors.Is(err, ErrInvalidCount) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, count.ID, count.Version, countView(count))
}

func (h *Handler) handleReconcileCount(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	countID := r.PathValue("count_id")

	var req struct {
		ReviewedBy string `json:"reviewed_by"`
		Reason     string `json:"reason"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	count, err := h.svc.ReconcileCount(r.Context(), tenantID, countID, ReconcileCountParams{
		ReviewedBy: req.ReviewedBy,
		Reason:     req.Reason,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrCountNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrCountAlreadyReviewed) {
			status = http.StatusConflict
			code = "ALREADY_RECONCILED"
		} else if errors.Is(err, ErrInvalidCount) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, count.ID, count.Version, countView(count))
}

func locationView(loc *StockLocation) map[string]any {
	return map[string]any{
		"id":            loc.ID,
		"tenant_id":     loc.TenantID,
		"property_id":   loc.PropertyID,
		"name":          loc.Name,
		"location_type": loc.LocationType,
		"version":       loc.Version,
		"created_at":    loc.CreatedAt.Format(time.RFC3339),
		"updated_at":    loc.UpdatedAt.Format(time.RFC3339),
	}
}

func movementView(m *InventoryMovement) map[string]any {
	vm := map[string]any{
		"id":              m.ID,
		"tenant_id":       m.TenantID,
		"location_id":     m.LocationID,
		"catalog_item_id": m.CatalogItemID,
		"movement_type":   m.MovementType,
		"quantity":        m.Quantity,
		"reference_type":  m.ReferenceType,
		"reference_id":    m.ReferenceID,
		"reason":          m.Reason,
		"actor_id":        m.ActorID,
		"created_at":      m.CreatedAt.Format(time.RFC3339),
	}
	if m.ExpiresAt != nil {
		vm["expires_at"] = m.ExpiresAt.Format(time.RFC3339)
	}
	return vm
}

func countView(c *InventoryCount) map[string]any {
	return map[string]any{
		"id":          c.ID,
		"tenant_id":   c.TenantID,
		"location_id": c.LocationID,
		"status":      c.Status,
		"counted_by":  c.CountedBy,
		"reviewed_by": c.ReviewedBy,
		"version":     c.Version,
		"created_at":  c.CreatedAt.Format(time.RFC3339),
		"updated_at":  c.UpdatedAt.Format(time.RFC3339),
	}
}

func countLineView(line *InventoryCountLine) map[string]any {
	return map[string]any{
		"id":                line.ID,
		"tenant_id":         line.TenantID,
		"count_id":          line.CountID,
		"catalog_item_id":   line.CatalogItemID,
		"expected_quantity": line.ExpectedQuantity,
		"counted_quantity":  line.CountedQuantity,
		"variance":          line.Variance,
		"created_at":        line.CreatedAt.Format(time.RFC3339),
	}
}
