package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *ReportingService
}

func NewHandler(svc *ReportingService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/reporting/snapshots", h.handleListSnapshots)
	mux.HandleFunc("POST /v1/reporting/snapshots/rebuild", h.handleRebuildSnapshot)
	mux.HandleFunc("POST /v1/reporting/snapshots/verify", h.handleVerifySnapshot)
	mux.HandleFunc("GET /v1/reporting/property-contribution", h.handleGetPropertyContribution)
	mux.HandleFunc("GET /v1/reporting/owner-exceptions", h.handleListOwnerExceptions)
	mux.HandleFunc("POST /v1/reporting/worker-metrics", h.handleRecordWorkerMetric)
	mux.HandleFunc("GET /v1/reporting/worker-metrics", h.handleListWorkerMetrics)
	mux.HandleFunc("GET /v1/reporting/worker-metrics/summary", h.handleWorkerMetricSummary)
	mux.HandleFunc("GET /v1/reporting/readiness", h.handleGetReadiness)
	mux.HandleFunc("GET /v1/reporting/service-level-summary", h.handleGetServiceLevelSummary)
	mux.HandleFunc("GET /v1/reporting/inventory-summary", h.handleGetInventorySummary)
	mux.HandleFunc("GET /v1/reporting/approval-pipeline", h.handleGetApprovalPipeline)
}

type reportResource struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Data    any    `json:"data"`
}

type reportCollection struct {
	Items []reportResource `json:"items"`
}

type reportError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

func subjectFromReportRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func apiReportError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(reportError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiReportResource(w http.ResponseWriter, status int, id string, version int64, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(reportResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiReportCollection(w http.ResponseWriter, items []reportResource) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reportCollection{Items: items})
}

func reportMapError(err error) (status int, code string) {
	switch {
	case errors.Is(err, ErrSnapshotNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrInvalidSnapshot),
		errors.Is(err, ErrUnknownProjection),
		errors.Is(err, ErrInvalidPeriod),
		errors.Is(err, ErrInvalidMetric):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	case errors.Is(err, ErrMetricRankingDenied),
		errors.Is(err, ErrMetricDisciplineDenied):
		return http.StatusForbidden, "FORBIDDEN"
	default:
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	}
}

// GET /v1/reporting/snapshots
func (h *Handler) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	snapshots, err := h.svc.ListSnapshots(r.Context(), tenantID)
	if err != nil {
		apiReportError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var items []reportResource
	for _, snap := range snapshots {
		items = append(items, reportResource{
			ID:      snap.ID,
			Version: snap.Version,
			Data:    snapshotView(snap),
		})
	}
	apiReportCollection(w, items)
}

// POST /v1/reporting/snapshots/rebuild
func (h *Handler) handleRebuildSnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Kind       string `json:"kind"`
		PropertyID string `json:"property_id"`
		Period     struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"period"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiReportError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var period *Period
	if req.Period.Start != "" || req.Period.End != "" {
		period = &Period{}
		if req.Period.Start != "" {
			t, err := time.Parse(time.RFC3339, req.Period.Start)
			if err != nil {
				apiReportError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid period start: "+err.Error())
				return
			}
			period.Start = t
		}
		if req.Period.End != "" {
			t, err := time.Parse(time.RFC3339, req.Period.End)
			if err != nil {
				apiReportError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid period end: "+err.Error())
				return
			}
			period.End = t
		}
	}

	snap, err := h.svc.RebuildSnapshot(r.Context(), tenantID, RebuildParams{
		Kind:       req.Kind,
		PropertyID: req.PropertyID,
		Period:     period,
	})
	if err != nil {
		status, code := reportMapError(err)
		apiReportError(w, r, status, code, err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, snap.ID, snap.Version, snapshotView(*snap))
}

// POST /v1/reporting/snapshots/verify
func (h *Handler) handleVerifySnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		SnapshotID string `json:"snapshot_id"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiReportError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	verification, err := h.svc.VerifySnapshot(r.Context(), tenantID, req.SnapshotID)
	if err != nil {
		status, code := reportMapError(err)
		apiReportError(w, r, status, code, err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, verification.SnapshotID, 0, verification)
}

// GET /v1/reporting/property-contribution
func (h *Handler) handleGetPropertyContribution(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	var period *Period
	if ps := r.URL.Query().Get("period_start"); ps != "" {
		t, err := time.Parse(time.RFC3339, ps)
		if err == nil {
			if period == nil {
				period = &Period{}
			}
			period.Start = t
		}
	}
	if pe := r.URL.Query().Get("period_end"); pe != "" {
		t, err := time.Parse(time.RFC3339, pe)
		if err == nil {
			if period == nil {
				period = &Period{}
			}
			period.End = t
		}
	}

	pc, err := h.svc.PropertyContribution(r.Context(), tenantID, propertyID, period)
	if err != nil {
		status, code := reportMapError(err)
		apiReportError(w, r, status, code, err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, propertyID, 0, pc)
}

// GET /v1/reporting/owner-exceptions
func (h *Handler) handleListOwnerExceptions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	exceptions, err := h.svc.ListOwnerExceptions(r.Context(), tenantID, propertyID)
	if err != nil {
		apiReportError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var items []reportResource
	for _, ex := range exceptions {
		items = append(items, reportResource{
			ID:      ex.SourceID,
			Version: 0,
			Data:    ex,
		})
	}
	apiReportCollection(w, items)
}

// POST /v1/reporting/worker-metrics
func (h *Handler) handleRecordWorkerMetric(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		WorkerID    string `json:"worker_id"`
		PropertyID  string `json:"property_id"`
		MetricKind  string `json:"metric_kind"`
		Value       int64  `json:"value"`
		Unit        string `json:"unit"`
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
		SourceRef   string `json:"source_ref"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiReportError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	params := MetricObservationParams{
		WorkerID:   req.WorkerID,
		PropertyID: req.PropertyID,
		MetricKind: req.MetricKind,
		Value:      req.Value,
		Unit:       req.Unit,
		SourceRef:  req.SourceRef,
	}
	if req.PeriodStart != "" {
		t, err := time.Parse(time.RFC3339, req.PeriodStart)
		if err == nil {
			params.PeriodStart = &t
		}
	}
	if req.PeriodEnd != "" {
		t, err := time.Parse(time.RFC3339, req.PeriodEnd)
		if err == nil {
			params.PeriodEnd = &t
		}
	}

	obs, err := h.svc.RecordWorkerMetric(r.Context(), tenantID, params, actorID)
	if err != nil {
		status, code := reportMapError(err)
		apiReportError(w, r, status, code, err.Error())
		return
	}

	apiReportResource(w, http.StatusCreated, obs.ID, 0, obs)
}

// GET /v1/reporting/worker-metrics
func (h *Handler) handleListWorkerMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	workerID := r.URL.Query().Get("worker_id")
	metricKind := r.URL.Query().Get("metric_kind")

	observations, err := h.svc.ListWorkerMetrics(r.Context(), tenantID, propertyID, workerID, metricKind)
	if err != nil {
		status, code := reportMapError(err)
		apiReportError(w, r, status, code, err.Error())
		return
	}

	var items []reportResource
	for _, o := range observations {
		items = append(items, reportResource{
			ID:      o.ID,
			Version: 0,
			Data:    o,
		})
	}
	apiReportCollection(w, items)
}

// GET /v1/reporting/worker-metrics/summary
func (h *Handler) handleWorkerMetricSummary(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.URL.Query().Get("worker_id")
	propertyID := r.URL.Query().Get("property_id")
	metricKind := r.URL.Query().Get("metric_kind")

	summary, err := h.svc.WorkerMetricSummary(r.Context(), tenantID, propertyID, workerID, metricKind)
	if err != nil {
		status, code := reportMapError(err)
		apiReportError(w, r, status, code, err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, workerID, 0, summary)
}

// GET /v1/reporting/readiness
func (h *Handler) handleGetReadiness(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	readiness, err := h.svc.GetReadiness(ctx(r), tenantID, propertyID)
	if err != nil {
		apiReportError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, propertyID, 0, readiness)
}

// GET /v1/reporting/service-level-summary
func (h *Handler) handleGetServiceLevelSummary(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	var period *Period
	if ps := r.URL.Query().Get("period_start"); ps != "" {
		t, err := time.Parse(time.RFC3339, ps)
		if err == nil {
			if period == nil {
				period = &Period{}
			}
			period.Start = t
		}
	}
	if pe := r.URL.Query().Get("period_end"); pe != "" {
		t, err := time.Parse(time.RFC3339, pe)
		if err == nil {
			if period == nil {
				period = &Period{}
			}
			period.End = t
		}
	}

	summary, err := h.svc.GetServiceLevelSummary(r.Context(), tenantID, propertyID, period)
	if err != nil {
		apiReportError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, propertyID, 0, summary)
}

// GET /v1/reporting/inventory-summary
func (h *Handler) handleGetInventorySummary(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	summary, err := h.svc.GetInventorySummary(ctx(r), tenantID, propertyID)
	if err != nil {
		apiReportError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, propertyID, 0, summary)
}

// GET /v1/reporting/approval-pipeline
func (h *Handler) handleGetApprovalPipeline(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromReportRequest(r)
	if err != nil {
		apiReportError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")

	pipeline, err := h.svc.GetApprovalPipeline(ctx(r), tenantID, propertyID)
	if err != nil {
		apiReportError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiReportResource(w, http.StatusOK, propertyID, 0, pipeline)
}

// ctx is shorthand that extracts context from http.Request when the handler
// signature doesn't already carry it. Used only for reporting-specific read
// model handlers.
func ctx(r *http.Request) context.Context { return r.Context() }

func snapshotView(snap ReportSnapshot) map[string]any {
	v := map[string]any{
		"id":           snap.ID,
		"tenant_id":    snap.TenantID,
		"property_id":  snap.PropertyID,
		"kind":         snap.Kind,
		"source_count": snap.SourceCount,
		"source_hash":  snap.SourceHash,
		"built_at":     snap.BuiltAt.Format(time.RFC3339),
		"version":      snap.Version,
		"created_at":   snap.CreatedAt.Format(time.RFC3339),
	}
	if snap.PeriodStart != nil {
		v["period_start"] = snap.PeriodStart.Format(time.RFC3339)
	}
	if snap.PeriodEnd != nil {
		v["period_end"] = snap.PeriodEnd.Format(time.RFC3339)
	}
	return v
}
