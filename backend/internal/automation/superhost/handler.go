package superhost

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	store         *automation.AgentRunStore
	assembler     *ContextAssembler
	threadStore   *ThreadStore
	toolCallStore *ToolCallStore
}

func NewHandler(store *automation.AgentRunStore, assembler *ContextAssembler) *Handler {
	return &Handler{store: store, assembler: assembler}
}

func NewHandlerWithThreads(store *automation.AgentRunStore, assembler *ContextAssembler, threadStore *ThreadStore) *Handler {
	return &Handler{store: store, assembler: assembler, threadStore: threadStore}
}

func NewHandlerWithApprovals(store *automation.AgentRunStore, assembler *ContextAssembler, threadStore *ThreadStore, toolCallStore *ToolCallStore) *Handler {
	return &Handler{store: store, assembler: assembler, threadStore: threadStore, toolCallStore: toolCallStore}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/superhost/runs", h.handleCreateRun)
	mux.HandleFunc("POST /v1/superhost/threads", h.handleCreateThread)
	mux.HandleFunc("POST /v1/superhost/threads/{thread_id}/messages", h.handleSendMessage)
	mux.HandleFunc("POST /v1/superhost/approvals/{request_id}/decide", h.handleDecideApproval)

	mux.HandleFunc("POST /v1/jarvis/runs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v1/superhost/runs", http.StatusPermanentRedirect)
	})
}

func (h *Handler) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req automation.SubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if req.RunKind == "" {
		req.RunKind = AgentKindSuperhost
	}

	if req.TenantID == "" {
		req.TenantID = tenantID
	}
	if req.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "tenant mismatch: model arguments cannot select another tenant")
		return
	}

	// actorID always comes from the authenticated subject, never the
	// request body -- letting a client set an arbitrary actor_id would
	// let it impersonate any account on the tenant (and, downstream,
	// forge entries on another account's task ledger and bypass tool-
	// audience role gating). tenantID above has the same rule already;
	// this closes the same gap for actor identity.
	req.ActorID = actorID
	if req.Provider == "" {
		req.Provider = defaultSuperhostProvider()
	}
	if req.Model == "" {
		req.Model = defaultSuperhostModel()
	}

	propertyID := req.PropertyID
	if propertyID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "property_id is required for superhost runs")
		return
	}

	context, err := h.assembler.Assemble(r.Context(), tenantID, propertyID, actorID)
	if err != nil {
		if errors.Is(err, ErrCrossPropertyDenied) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "cross-property request denied")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "context assembly failed: "+err.Error())
		return
	}

	contextJSON, err := json.Marshal(context)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "context serialization failed")
		return
	}
	req.InputData = contextJSON
	req.RunKind = AgentKindSuperhost

	run, duplicate, err := h.store.Submit(r.Context(), req)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":         run.RunID,
		"state":          run.State,
		"duplicate":      duplicate,
		"property_id":    propertyID,
		"context_source": "jarvis-context-assembler",
	})
}

func (h *Handler) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		TenantID       string `json:"tenant_id"`
		PropertyID     string `json:"property_id"`
		Purpose        string `json:"purpose"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if req.IdempotencyKey == "" || len(req.IdempotencyKey) < 8 {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "idempotency_key is required (min 8 chars)")
		return
	}
	if len(req.IdempotencyKey) > 128 {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "idempotency_key too long (max 128 chars)")
		return
	}
	if req.TenantID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "tenant_id is required")
		return
	}
	if req.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "tenant mismatch: cannot create a thread for another tenant")
		return
	}
	// req.PropertyID == "" is allowed: a portfolio-scoped thread, not
	// locked to one property (see ThreadStore.CreateThread and
	// ContextAssembler.AssemblePortfolio) -- lets one thread reason about
	// and act across every property on the tenant instead of requiring a
	// human to pick exactly one before Superhost can do anything.
	if req.Purpose == "" || len(req.Purpose) > 500 {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "purpose is required (max 500 chars)")
		return
	}

	if h.threadStore == nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "thread store not configured")
		return
	}

	thread, duplicate, err := h.threadStore.CreateThread(r.Context(), req.TenantID, req.PropertyID, actorID, req.Purpose, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, ErrCrossPropertyDenied) {
			writeError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE", "invalid property or tenant reference")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	_ = duplicate

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      thread.ThreadID,
		"version": 1,
		"data": map[string]any{
			"thread_id":  thread.ThreadID,
			"run_id":     thread.RunID,
			"created_at": thread.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		},
	})
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	threadID := r.PathValue("thread_id")
	if threadID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "thread_id path parameter is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		IdempotencyKey string           `json:"idempotency_key"`
		Content        string           `json:"content"`
		UISurfaces     []UISurfaceInput `json:"ui_surfaces,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if req.IdempotencyKey == "" || len(req.IdempotencyKey) < 8 {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "idempotency_key is required (min 8 chars)")
		return
	}
	if len(req.IdempotencyKey) > 128 {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "idempotency_key too long (max 128 chars)")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || len(content) > 4000 {
		writeError(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE", "content must be 1-4000 characters")
		return
	}
	content = content + "\n\n" + renderUISurfaces(req.UISurfaces)

	if h.threadStore == nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "thread store not configured")
		return
	}

	thread, err := h.threadStore.GetThread(r.Context(), threadID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "thread not found")
		return
	}
	if thread.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "not authorized for this thread")
		return
	}
	// A thread belongs to the one account that created it (see
	// ThreadStore.CreateThread) -- this is what actually keeps one
	// account's Superhost conversation private from another's, not just
	// the idempotency-key scoping that decides which thread a *new*
	// message resolves to.
	if thread.ActorID != "" && thread.ActorID != actorID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "not authorized for this thread")
		return
	}

	// An empty thread.PropertyID means this is a portfolio-scoped thread
	// (see ThreadStore.CreateThread) -- assemble context across every
	// property on the tenant instead of one.
	var pcRaw []byte
	if thread.PropertyID == "" {
		pc, err := h.assembler.AssemblePortfolio(r.Context(), thread.TenantID, actorID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "context assembly failed: "+err.Error())
			return
		}
		pcRaw, _ = json.Marshal(pc)
	} else {
		pc, err := h.assembler.Assemble(r.Context(), thread.TenantID, thread.PropertyID, actorID)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "context assembly failed: "+err.Error())
			return
		}
		pcRaw, _ = json.Marshal(pc)
	}

	msgInput := map[string]any{
		"type":        "user_message",
		"content":     content,
		"ui_surfaces": req.UISurfaces,
	}
	msgRaw, _ := json.Marshal(msgInput)
	combined := fmt.Sprintf(`{"context":%s,"message":%s}`, string(pcRaw), string(msgRaw))
	inputData := json.RawMessage(combined)

	run, _, err := h.store.Submit(r.Context(), automation.SubmitRequest{
		RunKind:        AgentKindSuperhost,
		TenantID:       thread.TenantID,
		PropertyID:     thread.PropertyID,
		ActorID:        actorID,
		Provider:       defaultSuperhostProvider(),
		Model:          defaultSuperhostModel(),
		IdempotencyKey: req.IdempotencyKey,
		InputData:      inputData,
	})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if err := h.threadStore.UpdateThreadRun(r.Context(), threadID, run.RunID); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	requestID := make([]byte, 16)
	rand.Read(requestID)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"request_id":  "req_" + hex.EncodeToString(requestID[:8]),
		"status":      "accepted",
		"resource_id": run.RunID,
	})
}

func (h *Handler) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, roles, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if h.toolCallStore == nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "approval store not configured")
		return
	}

	requestID := r.PathValue("request_id")
	if requestID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "request_id path parameter is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		Decision string `json:"decision"`
		Evidence string `json:"evidence,omitempty"`
		Reason   string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if req.Decision != ApprovalStateApproved && req.Decision != ApprovalStateRejected {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "decision must be 'approved' or 'rejected'")
		return
	}

	ar, err := h.toolCallStore.GetApprovalRequest(r.Context(), requestID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "approval request not found")
		return
	}

	if ar.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "not authorized for this approval request")
		return
	}

	approverRole := ""
	if len(roles) > 0 {
		approverRole = roles[0]
	}

	if err := ar.Decide(actorID, approverRole, req.Decision, req.Evidence, req.Reason); err != nil {
		if errors.Is(err, ErrPolicySelfApproval) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "self-approval is not allowed: requester cannot approve their own request")
			return
		}
		writeError(w, r, http.StatusConflict, "CONFLICT", err.Error())
		return
	}

	run, err := h.store.Get(r.Context(), ar.RunID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load run: "+err.Error())
		return
	}
	if run.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "not authorized for this run")
		return
	}

	if err := h.toolCallStore.SaveApprovalDecision(r.Context(), *ar); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to persist approval decision: "+err.Error())
		return
	}

	if req.Decision == ApprovalStateApproved {
		if err := h.store.DecideRun(r.Context(), ar.RunID, automation.StateQueued, ""); err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to requeue run: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"request_id": requestID,
			"decision":   "approved",
		})
		return
	}

	if err := h.store.DecideRun(r.Context(), ar.RunID, automation.StateFailed,
		fmt.Sprintf("policy denied: human rejected `%s`", ar.ToolName),
	); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to mark run as failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"request_id": requestID,
		"decision":   "rejected",
	})
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

func renderUISurfaces(surfaces []UISurfaceInput) string {
	if len(surfaces) == 0 {
		return "Available UI surfaces: none registered on the current page."
	}
	var b strings.Builder
	b.WriteString("Available UI surfaces:\n")
	for _, s := range surfaces {
		actions := "[" + strings.Join(s.Actions, ", ") + "]"
		fmt.Fprintf(&b, "- id: %q, label: %q, actions: %s\n", s.ID, s.Label, actions)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
