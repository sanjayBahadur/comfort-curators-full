package automation

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"comfort-curators-backend/internal/iam"
)

type AgentRunHandler struct {
	store *AgentRunStore
}

func NewAgentRunHandler(store *AgentRunStore) *AgentRunHandler {
	return &AgentRunHandler{store: store}
}

func (h *AgentRunHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/agent-runs", h.handleSubmit)
	mux.HandleFunc("GET /v1/agent-runs/{run_id}", h.handleGet)
	mux.HandleFunc("GET /v1/agent-runs/{run_id}/events", h.handleListEvents)
	mux.HandleFunc("POST /v1/agent-runs/{run_id}/cancel", h.handleCancel)
	mux.HandleFunc("POST /v1/agent-runs/{run_id}/retry", h.handleRetry)
	mux.HandleFunc("GET /v1/superhost/threads/{thread_id}/stream", h.handleStream)
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

func (h *AgentRunHandler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req SubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if req.RunKind == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "run_kind is required")
		return
	}

	if req.TenantID == "" {
		req.TenantID = tenantID
	}
	if req.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "tenant mismatch")
		return
	}

	if req.ActorID == "" {
		req.ActorID = tenantID
	}
	if req.Provider == "" {
		req.Provider = "model-stub"
	}
	if req.Model == "" {
		req.Model = "stub-v1"
	}

	run, duplicate, err := h.store.Submit(r.Context(), req)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":    run.RunID,
		"state":     run.State,
		"duplicate": duplicate,
	})
}

func (h *AgentRunHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	runID := r.PathValue("run_id")
	run, err := h.store.Get(r.Context(), runID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (h *AgentRunHandler) handleListEvents(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	runID := r.PathValue("run_id")
	events, err := ListEvents(r.Context(), h.store.pool, runID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if events == nil {
		events = []AgentRunEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"events": events,
	})
}

func (h *AgentRunHandler) handleCancel(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	runID := r.PathValue("run_id")
	if err := h.store.Cancel(r.Context(), runID); err != nil {
		if errors.Is(err, ErrRunNotCancellable) {
			writeError(w, r, http.StatusConflict, "NOT_CANCELLABLE", "run is not in a cancellable state")
			return
		}
		if errors.Is(err, ErrRunNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"run_id": runID,
		"state":  StateCancelled,
	})
}

func (h *AgentRunHandler) handleRetry(w http.ResponseWriter, r *http.Request) {
	_, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	runID := r.PathValue("run_id")
	if err := h.store.Retry(r.Context(), runID); err != nil {
		if errors.Is(err, ErrRunNotRetryable) {
			writeError(w, r, http.StatusConflict, "NOT_RETRYABLE", "run is not in a retryable state")
			return
		}
		if errors.Is(err, ErrRunNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"run_id": runID,
		"state":  StateQueued,
	})
}
