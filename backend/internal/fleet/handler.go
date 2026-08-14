package fleet

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
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
	mux.HandleFunc("POST /v1/fleet/assets", h.handleCreateAsset)
	mux.HandleFunc("GET /v1/fleet/assets", h.handleListAssets)
	mux.HandleFunc("GET /v1/fleet/assets/{asset_id}", h.handleGetAsset)
	mux.HandleFunc("POST /v1/fleet/assets/{asset_id}/safety-items", h.handleScheduleSafetyItem)
	mux.HandleFunc("POST /v1/fleet/safety-items/{record_id}/complete", h.handleCompleteSafetyItem)
	mux.HandleFunc("GET /v1/fleet/assets/{asset_id}/safety-items/overdue", h.handleListOverdueSafetyItems)
	mux.HandleFunc("POST /v1/fleet/assets/{asset_id}/inspections", h.handleRecordInspection)
	mux.HandleFunc("POST /v1/fleet/assets/{asset_id}/custody/handover", h.handleHandover)
	mux.HandleFunc("POST /v1/fleet/assets/{asset_id}/custody/return", h.handleReturn)
	mux.HandleFunc("GET /v1/fleet/assets/{asset_id}/custody-events", h.handleListCustodyEvents)
	mux.HandleFunc("POST /v1/fleet/assets/{asset_id}/incidents", h.handleRecordIncident)
	mux.HandleFunc("POST /v1/fleet/incidents/{incident_id}/review", h.handleReviewIncident)
	mux.HandleFunc("GET /v1/fleet/incidents/{incident_id}", h.handleGetIncident)
	mux.HandleFunc("GET /v1/fleet/assets/{asset_id}/incidents/open", h.handleListOpenIncidents)
	mux.HandleFunc("GET /v1/fleet/assets/{asset_id}/dispatch-eligibility", h.handleDispatchEligibility)
	mux.HandleFunc("GET /v1/fleet/workers/{worker_id}/tracking-status", h.handleGetTrackingStatus)
	mux.HandleFunc("POST /v1/fleet/workers/{worker_id}/locations", h.handleCollectLocation)
}

type fleetResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

type fleetError struct {
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
	json.NewEncoder(w).Encode(fleetError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiResource(w http.ResponseWriter, status int, id string, version int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(fleetResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiCollection(w http.ResponseWriter, items []fleetResource) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

func createAssetRequest(r *http.Request) (CreateAssetParams, error) {
	var req struct {
		Model                  string `json:"model"`
		SerialNumber           string `json:"serial_number"`
		RatedMotorPowerWatts   int    `json:"rated_motor_power_watts"`
		MaximumDesignSpeedKmh  int    `json:"maximum_design_speed_kmh"`
		DesignSpeedEvidenceRef string `json:"design_speed_evidence_ref"`
		ComplianceDocumentRef  string `json:"compliance_document_ref"`
		BatterySerial          string `json:"battery_serial"`
		Charger                string `json:"charger"`
		PurchaseDate           string `json:"purchase_date"`
		WarrantyExpiresAt      string `json:"warranty_expires_at"`
		WarrantyTerms          string `json:"warranty_terms"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return CreateAssetParams{}, err
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return CreateAssetParams{}, err
	}

	purchaseDate, err := time.Parse(time.RFC3339, req.PurchaseDate)
	if err != nil && req.PurchaseDate != "" {
		return CreateAssetParams{}, err
	}

	var warrantyExpiresAt *time.Time
	if req.WarrantyExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.WarrantyExpiresAt)
		if err != nil {
			return CreateAssetParams{}, err
		}
		warrantyExpiresAt = &t
	}

	return CreateAssetParams{
		Model:                  req.Model,
		SerialNumber:           req.SerialNumber,
		RatedMotorPowerWatts:   req.RatedMotorPowerWatts,
		MaximumDesignSpeedKmh:  req.MaximumDesignSpeedKmh,
		DesignSpeedEvidenceRef: req.DesignSpeedEvidenceRef,
		ComplianceDocumentRef:  req.ComplianceDocumentRef,
		BatterySerial:          req.BatterySerial,
		Charger:                req.Charger,
		PurchaseDate:           purchaseDate,
		WarrantyExpiresAt:      warrantyExpiresAt,
		WarrantyTerms:          req.WarrantyTerms,
	}, nil
}

func (h *Handler) handleCreateAsset(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	params, err := createAssetRequest(r)
	if err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	asset, battery, err := h.svc.CreateAsset(r.Context(), tenantID, params, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidAsset),
			errors.Is(err, ErrPowerLimitExceeded),
			errors.Is(err, ErrDesignSpeedLimitExceeded),
			errors.Is(err, ErrComplianceEvidenceRequired):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	data := map[string]any{
		"asset":   assetView(asset),
		"battery": batteryView(battery),
	}
	apiResource(w, http.StatusCreated, asset.ID, asset.Version, data)
}

func (h *Handler) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	assetID := r.PathValue("asset_id")
	asset, err := h.svc.GetAsset(r.Context(), tenantID, assetID)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "fleet asset not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, asset.ID, asset.Version, assetView(asset))
}

func (h *Handler) handleListAssets(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	assets, err := h.svc.ListAssets(r.Context(), tenantID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]fleetResource, 0, len(assets))
	for i := range assets {
		items = append(items, fleetResource{
			ID:      assets[i].ID,
			Version: assets[i].Version,
			Data:    assetView(&assets[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleScheduleSafetyItem(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	var req struct {
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Description string `json:"description"`
		DueAt       string `json:"due_at"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	dueAt, err := time.Parse(time.RFC3339, req.DueAt)
	if err != nil {
		apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid due_at, use RFC3339")
		return
	}

	record, err := h.svc.ScheduleSafetyItem(r.Context(), tenantID, assetID, SafetyItemParams{
		Kind:        req.Kind,
		Title:       req.Title,
		Description: req.Description,
		DueAt:       dueAt,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidSafetyKind) || errors.Is(err, ErrSafetyItemDueRequired) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrAssetNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, record.ID, record.Version, safetyItemView(record))
}

func (h *Handler) handleCompleteSafetyItem(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	recordID := r.PathValue("record_id")

	var req struct {
		CompletedAt string `json:"completed_at"`
		PerformedBy string `json:"performed_by"`
		Notes       string `json:"notes"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var completedAt time.Time
	if req.CompletedAt != "" {
		completedAt, err = time.Parse(time.RFC3339, req.CompletedAt)
		if err != nil {
			apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid completed_at, use RFC3339")
			return
		}
	}

	record, err := h.svc.CompleteSafetyItem(r.Context(), tenantID, recordID, CompleteSafetyItemParams{
		CompletedAt: completedAt,
		PerformedBy: req.PerformedBy,
		Notes:       req.Notes,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrSafetyItemNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrSafetyItemAlreadyCompleted) {
			status = http.StatusConflict
			code = "ALREADY_COMPLETED"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, record.ID, record.Version, safetyItemView(record))
}

func (h *Handler) handleListOverdueSafetyItems(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	items, err := h.svc.ListOverdueSafetyItems(r.Context(), tenantID, assetID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "fleet asset not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resources := make([]fleetResource, 0, len(items))
	for i := range items {
		resources = append(resources, fleetResource{
			ID:      items[i].ID,
			Version: items[i].Version,
			Data:    safetyItemView(&items[i]),
		})
	}
	apiCollection(w, resources)
}

func (h *Handler) handleRecordInspection(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	var req struct {
		WorkerID          string `json:"worker_id"`
		InspectionType    string `json:"inspection_type"`
		Result            string `json:"result"`
		DamageReported    bool   `json:"damage_reported"`
		DamageDescription string `json:"damage_description"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	inspection, err := h.svc.RecordInspection(r.Context(), tenantID, assetID, InspectionParams{
		WorkerID:          req.WorkerID,
		InspectionType:    req.InspectionType,
		Result:            req.Result,
		DamageReported:    req.DamageReported,
		DamageDescription: req.DamageDescription,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidInspection) || errors.Is(err, ErrInspectionResultInvalid) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrAssetNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, inspection.ID, 1, inspectionView(inspection))
}

func (h *Handler) handleHandover(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	var req struct {
		FromWorkerID   string `json:"from_worker_id"`
		ToWorkerID     string `json:"to_worker_id"`
		Condition      string `json:"condition"`
		Accessories    string `json:"accessories"`
		AcknowledgedBy string `json:"acknowledged_by"`
		Notes          string `json:"notes"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	event, err := h.svc.Handover(r.Context(), tenantID, assetID, CustodyParams{
		FromWorkerID:   req.FromWorkerID,
		ToWorkerID:     req.ToWorkerID,
		Condition:      req.Condition,
		Accessories:    req.Accessories,
		AcknowledgedBy: req.AcknowledgedBy,
		Notes:          req.Notes,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrCustodyEventInvalid) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrAssetNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, event.ID, 1, custodyEventView(event))
}

func (h *Handler) handleReturn(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	var req struct {
		FromWorkerID   string `json:"from_worker_id"`
		ToWorkerID     string `json:"to_worker_id"`
		Condition      string `json:"condition"`
		Accessories    string `json:"accessories"`
		AcknowledgedBy string `json:"acknowledged_by"`
		Notes          string `json:"notes"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	event, err := h.svc.Return(r.Context(), tenantID, assetID, CustodyParams{
		FromWorkerID:   req.FromWorkerID,
		ToWorkerID:     req.ToWorkerID,
		Condition:      req.Condition,
		Accessories:    req.Accessories,
		AcknowledgedBy: req.AcknowledgedBy,
		Notes:          req.Notes,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrNoActiveCustody) {
			status = http.StatusConflict
			code = "NO_ACTIVE_CUSTODY"
		} else if errors.Is(err, ErrCustodyMismatch) {
			status = http.StatusUnprocessableEntity
			code = "CUSTODY_MISMATCH"
		} else if errors.Is(err, ErrAssetNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, event.ID, 1, custodyEventView(event))
}

func (h *Handler) handleListCustodyEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	events, err := h.svc.ListCustodyEvents(r.Context(), tenantID, assetID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]fleetResource, 0, len(events))
	for i := range events {
		items = append(items, fleetResource{
			ID:      events[i].ID,
			Version: 1,
			Data:    custodyEventView(&events[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleRecordIncident(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	var req struct {
		Kind           string `json:"kind"`
		Severity       string `json:"severity"`
		Description    string `json:"description"`
		SafetyTicketID string `json:"safety_ticket_id"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	incident, err := h.svc.RecordIncident(r.Context(), tenantID, assetID, IncidentParams{
		Kind:           req.Kind,
		Severity:       req.Severity,
		Description:    req.Description,
		SafetyTicketID: req.SafetyTicketID,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrIncidentSeverityInvalid) || errors.Is(err, ErrInvalidAsset) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrAssetNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, incident.ID, incident.Version, incidentView(incident))
}

func (h *Handler) handleReviewIncident(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	incidentID := r.PathValue("incident_id")

	var req struct {
		Resolution string `json:"resolution"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	incident, err := h.svc.ReviewIncident(r.Context(), tenantID, incidentID, ReviewIncidentParams{
		Resolution: req.Resolution,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrIncidentRequiresResolution) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrIncidentNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrIncidentAlreadyResolved) {
			status = http.StatusConflict
			code = "ALREADY_RESOLVED"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, incident.ID, incident.Version, incidentView(incident))
}

func (h *Handler) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	incidentID := r.PathValue("incident_id")

	incident, err := h.svc.GetIncident(r.Context(), tenantID, incidentID)
	if err != nil {
		if errors.Is(err, ErrIncidentNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "fleet incident not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, incident.ID, incident.Version, incidentView(incident))
}

func (h *Handler) handleListOpenIncidents(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	incidents, err := h.svc.ListOpenIncidents(r.Context(), tenantID, assetID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]fleetResource, 0, len(incidents))
	for i := range incidents {
		items = append(items, fleetResource{
			ID:      incidents[i].ID,
			Version: incidents[i].Version,
			Data:    incidentView(&incidents[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleDispatchEligibility(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	assetID := r.PathValue("asset_id")

	block, err := h.svc.DispatchEligibility(r.Context(), tenantID, assetID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "fleet asset not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

func (h *Handler) handleGetTrackingStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	workerID := r.PathValue("worker_id")

	status, err := h.svc.GetTrackingStatus(r.Context(), tenantID, workerID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *Handler) handleCollectLocation(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	workerID := r.PathValue("worker_id")

	var req struct {
		AssetID    string  `json:"asset_id"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		CapturedAt string  `json:"captured_at"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var capturedAt time.Time
	if req.CapturedAt != "" {
		capturedAt, err = time.Parse(time.RFC3339, req.CapturedAt)
		if err != nil {
			apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid captured_at, use RFC3339")
			return
		}
	}

	event, err := h.svc.CollectLocation(r.Context(), tenantID, workerID, LocationParams{
		AssetID:    req.AssetID,
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		CapturedAt: capturedAt,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrOffDutyTrackingDisabled) {
			status = http.StatusForbidden
			code = "OFF_DUTY"
		} else if errors.Is(err, ErrAssetNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(trackingEventView(event))
}

func assetView(a *FleetAsset) map[string]any {
	m := map[string]any{
		"id":                        a.ID,
		"tenant_id":                 a.TenantID,
		"model":                     a.Model,
		"serial_number":             a.SerialNumber,
		"rated_motor_power_watts":   a.RatedMotorPowerWatts,
		"maximum_design_speed_kmh":  a.MaximumDesignSpeedKmh,
		"design_speed_evidence_ref": a.DesignSpeedEvidenceRef,
		"compliance_document_ref":   a.ComplianceDocumentRef,
		"battery_serial":            a.BatterySerial,
		"charger":                   a.Charger,
		"purchase_date":             a.PurchaseDate.Format(time.RFC3339),
		"warranty_terms":            a.WarrantyTerms,
		"assigned_custodian_id":     a.AssignedCustodianID,
		"status":                    a.Status,
		"version":                   a.Version,
		"created_at":                a.CreatedAt.Format(time.RFC3339),
		"updated_at":                a.UpdatedAt.Format(time.RFC3339),
	}
	if a.WarrantyExpiresAt != nil {
		m["warranty_expires_at"] = a.WarrantyExpiresAt.Format(time.RFC3339)
	}
	return m
}

func batteryView(b *FleetBattery) map[string]any {
	m := map[string]any{
		"id":             b.ID,
		"tenant_id":      b.TenantID,
		"asset_id":       b.AssetID,
		"battery_serial": b.BatterySerial,
		"health_status":  b.HealthStatus,
		"cycle_count":    b.CycleCount,
		"status":         b.Status,
		"version":        b.Version,
		"created_at":     b.CreatedAt.Format(time.RFC3339),
		"updated_at":     b.UpdatedAt.Format(time.RFC3339),
	}
	if b.LastServiceAt != nil {
		m["last_service_at"] = b.LastServiceAt.Format(time.RFC3339)
	}
	if b.NextServiceDueAt != nil {
		m["next_service_due_at"] = b.NextServiceDueAt.Format(time.RFC3339)
	}
	return m
}

func safetyItemView(r *FleetMaintenanceRecord) map[string]any {
	m := map[string]any{
		"id":               r.ID,
		"tenant_id":        r.TenantID,
		"asset_id":         r.AssetID,
		"kind":             r.Kind,
		"title":            r.Title,
		"description":      r.Description,
		"status":           r.Status,
		"service_provider": r.ServiceProvider,
		"performed_by":     r.PerformedBy,
		"notes":            r.Notes,
		"version":          r.Version,
		"created_at":       r.CreatedAt.Format(time.RFC3339),
		"updated_at":       r.UpdatedAt.Format(time.RFC3339),
	}
	if r.DueAt != nil {
		m["due_at"] = r.DueAt.Format(time.RFC3339)
	}
	if r.CompletedAt != nil {
		m["completed_at"] = r.CompletedAt.Format(time.RFC3339)
	}
	return m
}

func inspectionView(i *FleetInspection) map[string]any {
	return map[string]any{
		"id":                 i.ID,
		"tenant_id":          i.TenantID,
		"asset_id":           i.AssetID,
		"worker_id":          i.WorkerID,
		"inspection_type":    i.InspectionType,
		"result":             i.Result,
		"damage_reported":    i.DamageReported,
		"damage_description": i.DamageDescription,
		"created_at":         i.CreatedAt.Format(time.RFC3339),
	}
}

func custodyEventView(e *FleetCustodyEvent) map[string]any {
	m := map[string]any{
		"id":              e.ID,
		"tenant_id":       e.TenantID,
		"asset_id":        e.AssetID,
		"event_type":      e.EventType,
		"from_worker_id":  e.FromWorkerID,
		"to_worker_id":    e.ToWorkerID,
		"condition":       e.Condition,
		"accessories":     e.Accessories,
		"acknowledged_by": e.AcknowledgedBy,
		"notes":           e.Notes,
		"created_at":      e.CreatedAt.Format(time.RFC3339),
	}
	if e.AcknowledgedAt != nil {
		m["acknowledged_at"] = e.AcknowledgedAt.Format(time.RFC3339)
	}
	return m
}

func incidentView(i *FleetIncident) map[string]any {
	m := map[string]any{
		"id":               i.ID,
		"tenant_id":        i.TenantID,
		"asset_id":         i.AssetID,
		"kind":             i.Kind,
		"severity":         i.Severity,
		"description":      i.Description,
		"reported_by":      i.ReportedBy,
		"safety_ticket_id": i.SafetyTicketID,
		"status":           i.Status,
		"reviewed_by":      i.ReviewedBy,
		"resolution":       i.Resolution,
		"version":          i.Version,
		"created_at":       i.CreatedAt.Format(time.RFC3339),
		"updated_at":       i.UpdatedAt.Format(time.RFC3339),
	}
	if i.ReviewedAt != nil {
		m["reviewed_at"] = i.ReviewedAt.Format(time.RFC3339)
	}
	return m
}

func trackingEventView(e *FleetTrackingEvent) map[string]any {
	return map[string]any{
		"id":               e.ID,
		"tenant_id":        e.TenantID,
		"asset_id":         e.AssetID,
		"worker_id":        e.WorkerID,
		"custody_event_id": e.CustodyEventID,
		"latitude":         e.Latitude,
		"longitude":        e.Longitude,
		"captured_at":      e.CapturedAt.Format(time.RFC3339),
		"created_at":       e.CreatedAt.Format(time.RFC3339),
	}
}

// ETag reads the current version of a resource for concurrency control.
func ETag(w http.ResponseWriter, version int) {
	w.Header().Set("ETag", strconv.Itoa(version))
}
