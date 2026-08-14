package observability

import (
	"encoding/json"
	"net/http"
	"strings"

	"comfort-curators-backend/internal/iam"
)

// Handler exposes observability data over HTTP for dashboards and operators.
type Handler struct {
	Metrics *Metrics
	Tracer  *Tracer
	Alerts  *AlertService
}

// NewHandler returns an observability HTTP handler wired to the given
// in-process registries.
func NewHandler(m *Metrics, tr *Tracer, a *AlertService) *Handler {
	return &Handler{Metrics: m, Tracer: tr, Alerts: a}
}

// RegisterRoutes attaches observability endpoints to the provided mux,
// gated to staff. These disclose operational internals (metrics, alerts,
// distributed traces) that RequireAuthByDefault alone -- which only checks
// "is there any authenticated subject at all" -- does not restrict from
// any logged-in owner or guest.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	staffOnly := iam.RequireRole(iam.RoleStaff)
	mux.Handle("GET /metrics", staffOnly(http.HandlerFunc(h.handleMetrics)))
	mux.Handle("GET /alerts", staffOnly(http.HandlerFunc(h.handleAlerts)))
	mux.Handle("GET /alerts/unresolved", staffOnly(http.HandlerFunc(h.handleUnresolvedAlerts)))
	mux.Handle("GET /traces", staffOnly(http.HandlerFunc(h.handleTraces)))
	mux.Handle("GET /traces/{traceID}", staffOnly(http.HandlerFunc(h.handleTrace)))
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": h.Metrics.Snapshot(),
	})
}

func (h *Handler) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := h.Alerts.Alerts()
	if alerts == nil {
		alerts = []Alert{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

func (h *Handler) handleUnresolvedAlerts(w http.ResponseWriter, r *http.Request) {
	kinds := parseAlertKinds(r.URL.Query().Get("kind"))
	alerts := h.Alerts.Unresolved(kinds...)
	if alerts == nil {
		alerts = []Alert{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

func (h *Handler) handleTraces(w http.ResponseWriter, r *http.Request) {
	spans := h.Tracer.Spans()
	if spans == nil {
		spans = []Span{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"traces": spans,
		"count":  len(spans),
	})
}

func (h *Handler) handleTrace(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceID")
	if traceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "traceID is required",
		})
		return
	}
	spans := h.Tracer.Trace(traceID)
	if spans == nil {
		spans = []Span{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trace_id": traceID,
		"spans":    spans,
		"count":    len(spans),
	})
}

func parseAlertKinds(raw string) []AlertKind {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	kinds := make([]AlertKind, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kinds = append(kinds, AlertKind(p))
	}
	return kinds
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
