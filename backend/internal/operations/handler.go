package operations

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/security"
)

type ticketResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

type ticketCollection struct {
	Items      []ticketResource `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

type ticketError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type TicketHandler struct {
	svc *TicketService
}

func NewTicketHandler(svc *TicketService) *TicketHandler {
	return &TicketHandler{svc: svc}
}

func (h *TicketHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/tickets", h.handleCreateTicket)
	mux.HandleFunc("GET /v1/tickets", h.handleListTickets)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}", h.handleGetTicket)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/transitions", h.handleTransitionTicket)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/state-events", h.handleListStateEvents)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/blockers", h.handleBlockTicket)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/unblock", h.handleUnblockTicket)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/cancel", h.handleCancelTicket)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/reopen", h.handleReopenTicket)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/checklist-syncs", h.handleSyncChecklist)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/checklist-items", h.handleListChecklistItems)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/evidence", h.handleRegisterEvidence)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/evidence", h.handleListEvidence)
	mux.HandleFunc("GET /v1/evidence/{evidence_id}", h.handleGetEvidence)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/classify", h.handleClassifyIncident)
	mux.HandleFunc("GET /v1/alerts/incident", h.handleListIncidentAlerts)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/alerts", h.handleListIncidentAlertsForTicket)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/recovery", h.handleStartServiceRecovery)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/recovery", h.handleListServiceRecoveries)
	mux.HandleFunc("GET /v1/recovery/{recovery_id}", h.handleGetServiceRecovery)
	mux.HandleFunc("POST /v1/recovery/{recovery_id}/close", h.handleCloseServiceRecovery)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/checklist-syncs/idempotent", h.handleIdempotentSyncChecklist)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/sync-conflicts", h.handleListSyncConflicts)
	mux.HandleFunc("POST /v1/sync-conflicts/{conflict_id}/resolve", h.handleResolveSyncConflict)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/offline-evidence", h.handleQueueOfflineEvidence)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/offline-evidence/sync", h.handleSyncOfflineEvidence)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/offline-evidence", h.handleListQueuedOfflineEvidence)
}

type createTicketRequest struct {
	TenantID           string          `json:"tenant_id"`
	PropertyID         string          `json:"property_id"`
	Type               string          `json:"type"`
	RequestedWindow    json.RawMessage `json:"requested_window"`
	ChecklistVersionID string          `json:"checklist_version_id,omitempty"`
	Reason             string          `json:"reason"`
}

type transitionTicketRequest struct {
	ToState     string   `json:"to_state"`
	Reason      string   `json:"reason"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type blockTicketRequest struct {
	Type             string `json:"type"`
	Reason           string `json:"reason"`
	ResponsibleParty string `json:"responsible_party,omitempty"`
	EscalationPolicy string `json:"escalation_policy,omitempty"`
}

type unblockTicketRequest struct {
	Reason string `json:"reason"`
}

type cancelTicketRequest struct {
	Reason string `json:"reason"`
}

type reopenTicketRequest struct {
	Reason string `json:"reason"`
}

type syncChecklistRequest struct {
	Items []struct {
		TemplateItemIndex int      `json:"template_item_index"`
		Label             string   `json:"label"`
		Status            string   `json:"status"`
		CompletedBy       string   `json:"completed_by,omitempty"`
		EvidenceIDs       []string `json:"evidence_ids,omitempty"`
		EvidenceRequired  bool     `json:"evidence_required,omitempty"`
		Notes             string   `json:"notes,omitempty"`
	} `json:"items"`
}

func (h *TicketHandler) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createTicketRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Type == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "type is required")
		return
	}
	if req.PropertyID == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id is required")
		return
	}
	if req.Reason == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "reason is required")
		return
	}

	params := CreateTicketParams{
		TenantID:           subject.TenantID,
		PropertyID:         req.PropertyID,
		Type:               req.Type,
		RequestedWindow:    req.RequestedWindow,
		ChecklistVersionID: req.ChecklistVersionID,
		Reason:             req.Reason,
	}

	t, err := h.svc.CreateTicket(r.Context(), params, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidTicketType) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusCreated, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleListTickets(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	status := r.URL.Query().Get("status")
	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, parseErr := strconv.Atoi(l); parseErr == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	tickets, nextCursor, err := h.svc.ListTickets(r.Context(), subject.TenantID, propertyID, status, cursor, limit)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(tickets))
	for _, t := range tickets {
		items = append(items, ticketResource{ID: t.ID, Version: t.Version, Data: ticketView(&t)})
	}

	var nc *string
	if nextCursor != "" {
		nc = &nextCursor
	}
	writeHTTPCollection(w, items, nc)
}

func (h *TicketHandler) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	t, err := h.svc.GetTicket(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusOK, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleTransitionTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req transitionTicketRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.ToState == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "to_state is required")
		return
	}

	t, err := h.svc.TransitionTicket(r.Context(), subject.TenantID, ticketID, TransitionParams{
		ToState:     req.ToState,
		Reason:      req.Reason,
		EvidenceIDs: req.EvidenceIDs,
	}, subject.ActorID)
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
		case errors.Is(err, ErrTicketTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case errors.Is(err, ErrSelfVerification):
			code = "SELF_VERIFICATION_DENIED"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrEvidenceRequired):
			code = "EVIDENCE_REQUIRED"
			status = http.StatusUnprocessableEntity
		}

		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusOK, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleListStateEvents(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	events, err := h.svc.ListStateEvents(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(events))
	for _, e := range events {
		items = append(items, ticketResource{
			ID:      e.ID,
			Version: 1,
			Data:    stateEventView(e),
		})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleBlockTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req blockTicketRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.BlockTicket(r.Context(), subject.TenantID, ticketID, TicketBlock{
		Type:             req.Type,
		Reason:           req.Reason,
		ResponsibleParty: req.ResponsibleParty,
		EscalationPolicy: req.EscalationPolicy,
	}, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrBlockerRequired):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrInvalidBlockerType):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrAlreadyBlocked):
			code = "ALREADY_BLOCKED"
			status = http.StatusConflict
		case errors.Is(err, ErrTicketTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusOK, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleUnblockTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req unblockTicketRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.UnblockTicket(r.Context(), subject.TenantID, ticketID, req.Reason, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrTicketNotBlocked):
			code = "NOT_BLOCKED"
			status = http.StatusConflict
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusOK, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleCancelTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req cancelTicketRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.CancelTicket(r.Context(), subject.TenantID, ticketID, req.Reason, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrTicketTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrInvalidTransition):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusOK, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleReopenTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req reopenTicketRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.ReopenTicket(r.Context(), subject.TenantID, ticketID, req.Reason, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrReopenRequiresReason):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusCreated, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleSyncChecklist(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req syncChecklistRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	items := make([]TicketChecklistItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = TicketChecklistItem{
			TemplateItemIndex: item.TemplateItemIndex,
			Label:             item.Label,
			Status:            item.Status,
			CompletedBy:       item.CompletedBy,
			EvidenceIDs:       item.EvidenceIDs,
			EvidenceRequired:  item.EvidenceRequired,
			Notes:             item.Notes,
		}
	}

	result, err := h.svc.SyncChecklist(r.Context(), subject.TenantID, ticketID, items, subject.ActorID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		if errors.Is(err, ErrEvidenceRequirementLocks) {
			writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	respItems := make([]ticketResource, 0, len(result))
	for _, item := range result {
		respItems = append(respItems, ticketResource{
			ID:      item.ID,
			Version: item.Version,
			Data:    checklistItemView(item),
		})
	}
	writeHTTPCollection(w, respItems, nil)
}

func (h *TicketHandler) handleListChecklistItems(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	items, err := h.svc.ListChecklistItems(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	respItems := make([]ticketResource, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, ticketResource{
			ID:      item.ID,
			Version: item.Version,
			Data:    checklistItemView(item),
		})
	}
	writeHTTPCollection(w, respItems, nil)
}

type registerEvidenceRequest struct {
	ChecklistItemID string `json:"checklist_item_id,omitempty"`
	ObjectID        string `json:"object_id,omitempty"`
	ContentHash     string `json:"content_hash"`
	FileName        string `json:"file_name,omitempty"`
	ContentType     string `json:"content_type,omitempty"`
	SizeBytes       int64  `json:"size_bytes"`
}

type classifyIncidentRequest struct {
	Severity string `json:"severity"`
}

type startRecoveryRequest struct {
	Reason          string `json:"reason"`
	Responsibility  string `json:"responsibility"`
	ReworkCostMinor int64  `json:"rework_cost_minor"`
	Currency        string `json:"currency,omitempty"`
}

func (h *TicketHandler) handleRegisterEvidence(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req registerEvidenceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.ContentHash == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "content_hash is required")
		return
	}

	rec, err := h.svc.RegisterEvidence(r.Context(), subject.TenantID, ticketID, RegisterEvidenceParams{
		ChecklistItemID: req.ChecklistItemID,
		ObjectID:        req.ObjectID,
		ContentHash:     req.ContentHash,
		FileName:        req.FileName,
		ContentType:     req.ContentType,
		SizeBytes:       req.SizeBytes,
	}, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrInvalidEvidenceHash):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrChecklistItemNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrTicketTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPResource(w, http.StatusCreated, rec.ID, rec.Version, evidenceView(rec))
}

func (h *TicketHandler) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	evidenceID := r.PathValue("evidence_id")

	rec, err := h.svc.GetEvidence(r.Context(), subject.TenantID, evidenceID)
	if err != nil {
		if errors.Is(err, ErrEvidenceNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "evidence not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeHTTPResource(w, http.StatusOK, rec.ID, rec.Version, evidenceView(rec))
}

func (h *TicketHandler) handleListEvidence(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	records, err := h.svc.ListEvidence(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(records))
	for _, rec := range records {
		items = append(items, ticketResource{ID: rec.ID, Version: rec.Version, Data: evidenceView(&rec)})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleClassifyIncident(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req classifyIncidentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.ClassifyIncident(r.Context(), subject.TenantID, ticketID, req.Severity, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSeverityRequired), errors.Is(err, ErrInvalidSeverity):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrNotIncident):
			code = "NOT_AN_INCIDENT"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPETag(w, t.Version)
	writeHTTPResource(w, http.StatusOK, t.ID, t.Version, ticketView(t))
}

func (h *TicketHandler) handleListIncidentAlerts(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	status := r.URL.Query().Get("status")

	if propertyID == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id is required")
		return
	}

	alerts, err := h.svc.ListIncidentAlerts(r.Context(), subject.TenantID, propertyID, status)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(alerts))
	for _, a := range alerts {
		items = append(items, ticketResource{ID: a.ID, Version: 1, Data: incidentAlertView(a)})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleListIncidentAlertsForTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	alerts, err := h.svc.ListIncidentAlertsForTicket(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(alerts))
	for _, a := range alerts {
		items = append(items, ticketResource{ID: a.ID, Version: 1, Data: incidentAlertView(a)})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleStartServiceRecovery(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req startRecoveryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Reason == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "reason is required")
		return
	}

	rec, err := h.svc.StartServiceRecovery(r.Context(), subject.TenantID, ticketID, RecoveryParams{
		Reason:          req.Reason,
		Responsibility:  req.Responsibility,
		ReworkCostMinor: req.ReworkCostMinor,
		Currency:        req.Currency,
	}, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrResponsibilityRequired):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrInvalidReworkCost), errors.Is(err, ErrCurrencyRequired):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrNotIncident):
			code = "NOT_AN_INCIDENT"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketTerminal):
			code = "INVALID_TRANSITION"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPResource(w, http.StatusCreated, rec.ID, 1, serviceRecoveryView(rec))
}

func (h *TicketHandler) handleGetServiceRecovery(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	recoveryID := r.PathValue("recovery_id")

	rec, err := h.svc.GetServiceRecovery(r.Context(), subject.TenantID, recoveryID)
	if err != nil {
		if errors.Is(err, ErrRecoveryNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "service recovery not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeHTTPResource(w, http.StatusOK, rec.ID, 1, serviceRecoveryView(rec))
}

func (h *TicketHandler) handleListServiceRecoveries(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	recoveries, err := h.svc.ListServiceRecoveries(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(recoveries))
	for _, rec := range recoveries {
		items = append(items, ticketResource{ID: rec.ID, Version: 1, Data: serviceRecoveryView(&rec)})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleCloseServiceRecovery(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	recoveryID := r.PathValue("recovery_id")

	rec, err := h.svc.CloseServiceRecovery(r.Context(), subject.TenantID, recoveryID, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrRecoveryNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrRecoveryInactive):
			code = "RECOVERY_INACTIVE"
			status = http.StatusConflict
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPResource(w, http.StatusOK, rec.ID, 1, serviceRecoveryView(rec))
}

func ticketView(t *Ticket) map[string]any {
	v := map[string]any{
		"id":          t.ID,
		"tenant_id":   t.TenantID,
		"property_id": t.PropertyID,
		"type":        t.Type,
		"status":      t.Status,
		"reason":      t.Reason,
		"created_by":  t.CreatedBy,
		"version":     t.Version,
		"created_at":  t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":  t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if t.RequestedWindow != nil {
		var window any
		if err := json.Unmarshal(t.RequestedWindow, &window); err == nil {
			v["requested_window"] = window
		}
	}
	if t.ChecklistVersionID != "" {
		v["checklist_version_id"] = t.ChecklistVersionID
	}
	if t.AssignedTo != "" {
		v["assigned_to"] = t.AssignedTo
	}
	if t.VerifiedBy != "" {
		v["verified_by"] = t.VerifiedBy
	}
	if t.VerifierNote != "" {
		v["verifier_note"] = t.VerifierNote
	}
	if t.Blocker != nil {
		v["blocker"] = t.Blocker
	}
	if t.FollowUpTicketID != "" {
		v["follow_up_ticket_id"] = t.FollowUpTicketID
	}
	if t.ReopenReason != "" {
		v["reopen_reason"] = t.ReopenReason
	}
	if t.NotificationIntent != "" {
		v["notification_intent"] = t.NotificationIntent
	}
	if t.Severity != "" {
		v["severity"] = t.Severity
	}
	return v
}

func evidenceView(e *EvidenceRecord) map[string]any {
	v := map[string]any{
		"id":           e.ID,
		"tenant_id":    e.TenantID,
		"ticket_id":    e.TicketID,
		"content_hash": e.ContentHash,
		"size_bytes":   e.SizeBytes,
		"status":       e.Status,
		"captured_by":  e.CapturedBy,
		"captured_at":  e.CapturedAt.Format("2006-01-02T15:04:05Z"),
		"version":      e.Version,
	}
	if e.ChecklistItemID != "" {
		v["checklist_item_id"] = e.ChecklistItemID
	}
	if e.ObjectID != "" {
		v["object_id"] = e.ObjectID
	}
	if e.FileName != "" {
		v["file_name"] = e.FileName
	}
	if e.ContentType != "" {
		v["content_type"] = e.ContentType
	}
	return v
}

func incidentAlertView(a IncidentAlert) map[string]any {
	return map[string]any{
		"id":          a.ID,
		"tenant_id":   a.TenantID,
		"property_id": a.PropertyID,
		"ticket_id":   a.TicketID,
		"severity":    a.Severity,
		"target":      a.Target,
		"policy":      a.Policy,
		"status":      a.Status,
		"created_at":  a.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func serviceRecoveryView(r *ServiceRecovery) map[string]any {
	v := map[string]any{
		"id":                 r.ID,
		"tenant_id":          r.TenantID,
		"property_id":        r.PropertyID,
		"incident_ticket_id": r.IncidentTicketID,
		"severity":           r.Severity,
		"original_reason":    r.OriginalReason,
		"responsibility":     r.Responsibility,
		"rework_cost_minor":  r.ReworkCostMinor,
		"currency":           r.Currency,
		"status":             r.Status,
		"created_by":         r.CreatedBy,
		"created_at":         r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":         r.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.FollowUpTicketID != "" {
		v["follow_up_ticket_id"] = r.FollowUpTicketID
	}
	if len(r.OriginalEvidenceHashes) > 0 {
		v["original_evidence_hashes"] = r.OriginalEvidenceHashes
	}
	return v
}

func stateEventView(e TicketStateEvent) map[string]any {
	v := map[string]any{
		"id":         e.ID,
		"ticket_id":  e.TicketID,
		"from_state": e.FromState,
		"to_state":   e.ToState,
		"actor_id":   e.ActorID,
		"reason":     e.Reason,
		"version":    e.Version,
		"created_at": e.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if len(e.Evidence) > 0 {
		v["evidence"] = e.Evidence
	}
	return v
}

func checklistItemView(item TicketChecklistItem) map[string]any {
	v := map[string]any{
		"id":                  item.ID,
		"ticket_id":           item.TicketID,
		"template_item_index": item.TemplateItemIndex,
		"label":               item.Label,
		"status":              item.Status,
	}
	if item.EvidenceRequired {
		v["evidence_required"] = true
	}
	if item.CompletedBy != "" {
		v["completed_by"] = item.CompletedBy
	}
	if item.CompletedAt != nil {
		v["completed_at"] = item.CompletedAt.Format("2006-01-02T15:04:05Z")
	}
	if len(item.EvidenceIDs) > 0 {
		v["evidence_ids"] = item.EvidenceIDs
	}
	if item.Notes != "" {
		v["notes"] = item.Notes
	}
	return v
}

type idempotentSyncRequest struct {
	SyncKey string `json:"sync_key"`
	Items   []struct {
		TemplateItemIndex int      `json:"template_item_index"`
		Label             string   `json:"label"`
		Status            string   `json:"status"`
		CompletedBy       string   `json:"completed_by,omitempty"`
		EvidenceIDs       []string `json:"evidence_ids,omitempty"`
		EvidenceRequired  bool     `json:"evidence_required,omitempty"`
		Notes             string   `json:"notes,omitempty"`
	} `json:"items"`
}

type resolveSyncConflictRequest struct {
	Resolution string `json:"resolution"`
}

type offlineEvidenceRequest struct {
	ContentHash  string `json:"content_hash"`
	FileName     string `json:"file_name,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	Language     string `json:"language,omitempty"`
	CapturedNote string `json:"captured_note,omitempty"`
}

type syncOfflineEvidenceRequest struct {
	ContentHashes []string `json:"content_hashes"`
}

func (h *TicketHandler) handleIdempotentSyncChecklist(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req idempotentSyncRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.SyncKey == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "sync_key is required")
		return
	}

	items := make([]TicketChecklistItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = TicketChecklistItem{
			TemplateItemIndex: item.TemplateItemIndex,
			Label:             item.Label,
			Status:            item.Status,
			CompletedBy:       item.CompletedBy,
			EvidenceIDs:       item.EvidenceIDs,
			EvidenceRequired:  item.EvidenceRequired,
			Notes:             item.Notes,
		}
	}

	result, err := h.svc.IdempotentSyncChecklist(r.Context(), subject.TenantID, ticketID, req.SyncKey, items, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrSyncKeyConflict):
			code = "SYNC_KEY_CONFLICT"
			status = http.StatusConflict
		case errors.Is(err, ErrEvidenceRequirementLocks):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, map[string]any{
		"items":     checklistItemsView(result.Items),
		"conflicts": syncConflictsView(result.Conflicts),
		"replay":    result.Replay,
	})
}

func (h *TicketHandler) handleListSyncConflicts(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	resolvedOnly := false
	if r.URL.Query().Get("resolved") == "true" {
		resolvedOnly = true
	}

	conflicts, err := h.svc.ListSyncConflicts(r.Context(), subject.TenantID, ticketID, resolvedOnly)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(conflicts))
	for _, c := range conflicts {
		items = append(items, ticketResource{ID: c.ID, Version: 1, Data: syncConflictView(c)})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleResolveSyncConflict(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	conflictID := r.PathValue("conflict_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req resolveSyncConflictRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Resolution == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "resolution is required")
		return
	}

	c, err := h.svc.ResolveSyncConflict(r.Context(), subject.TenantID, conflictID, req.Resolution, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSyncConflictNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrSyncConflictNotOpen):
			code = "CONFLICT_RESOLVED"
			status = http.StatusConflict
		case errors.Is(err, ErrCrossTenantDenied):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPResource(w, http.StatusOK, c.ID, 1, syncConflictView(*c))
}

func (h *TicketHandler) handleQueueOfflineEvidence(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req offlineEvidenceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	payload := OfflineEvidencePayload{
		ContentHash:  req.ContentHash,
		FileName:     req.FileName,
		ContentType:  req.ContentType,
		SizeBytes:    req.SizeBytes,
		Language:     req.Language,
		CapturedNote: req.CapturedNote,
	}

	e, err := h.svc.QueueOfflineEvidence(r.Context(), subject.TenantID, ticketID, payload, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrInvalidEvidenceHash):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		case errors.Is(err, ErrTicketNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPResource(w, http.StatusCreated, e.ID, 1, offlineEvidenceView(e))
}

func (h *TicketHandler) handleSyncOfflineEvidence(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req syncOfflineEvidenceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if len(req.ContentHashes) == 0 {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "content_hashes is required")
		return
	}

	records, err := h.svc.SyncOfflineEvidence(r.Context(), subject.TenantID, ticketID, req.ContentHashes, subject.ActorID)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(records))
	for _, rec := range records {
		items = append(items, ticketResource{ID: rec.ID, Version: rec.Version, Data: evidenceView(&rec)})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *TicketHandler) handleListQueuedOfflineEvidence(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")
	status := r.URL.Query().Get("status")

	records, err := h.svc.ListQueuedOfflineEvidence(r.Context(), subject.TenantID, ticketID, status)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "ticket not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]ticketResource, 0, len(records))
	for _, rec := range records {
		items = append(items, ticketResource{ID: rec.ID, Version: 1, Data: offlineEvidenceView(&rec)})
	}
	writeHTTPCollection(w, items, nil)
}

func syncConflictView(c SyncConflict) map[string]any {
	v := map[string]any{
		"id":                  c.ID,
		"ticket_id":           c.TicketID,
		"template_item_index": c.TemplateItemIndex,
		"server_label":        c.ServerLabel,
		"server_status":       c.ServerStatus,
		"server_version":      c.ServerVersion,
		"client_label":        c.ClientLabel,
		"client_status":       c.ClientStatus,
		"client_version":      c.ClientVersion,
		"resolved":            c.Resolved,
	}
	if c.ChecklistItemID != "" {
		v["checklist_item_id"] = c.ChecklistItemID
	}
	if c.Resolution != "" {
		v["resolution"] = c.Resolution
	}
	if c.ResolvedBy != "" {
		v["resolved_by"] = c.ResolvedBy
	}
	if c.ResolvedAt != nil {
		v["resolved_at"] = c.ResolvedAt.Format("2006-01-02T15:04:05Z")
	}
	return v
}

func syncConflictsView(conflicts []SyncConflict) []map[string]any {
	items := make([]map[string]any, len(conflicts))
	for i, c := range conflicts {
		items[i] = syncConflictView(c)
	}
	return items
}

func checklistItemsView(items []TicketChecklistItem) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		result[i] = checklistItemView(item)
	}
	return result
}

func offlineEvidenceView(e *QueuedOfflineEvidence) map[string]any {
	v := map[string]any{
		"id":           e.ID,
		"tenant_id":    e.TenantID,
		"ticket_id":    e.TicketID,
		"content_hash": e.ContentHash,
		"size_bytes":   e.SizeBytes,
		"status":       e.Status,
		"captured_by":  e.CapturedBy,
		"captured_at":  e.CapturedAt.Format("2006-01-02T15:04:05Z"),
	}
	if e.ChecklistItemID != "" {
		v["checklist_item_id"] = e.ChecklistItemID
	}
	if e.FileName != "" {
		v["file_name"] = e.FileName
	}
	if e.ContentType != "" {
		v["content_type"] = e.ContentType
	}
	return v
}

func getSubject(r *http.Request) (security.Subject, error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return security.Subject{}, errors.New("unauthenticated")
	}
	return subject, nil
}

func writeHTTPJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeHTTPResource(w http.ResponseWriter, status int, id string, version int, data any) {
	writeHTTPJSON(w, status, ticketResource{ID: id, Version: version, Data: data})
}

func writeHTTPCollection(w http.ResponseWriter, items []ticketResource, nextCursor *string) {
	writeHTTPJSON(w, http.StatusOK, ticketCollection{Items: items, NextCursor: nextCursor})
}

func writeHTTPETag(w http.ResponseWriter, version int) {
	w.Header().Set("ETag", strconv.Itoa(version))
}

func writeHTTPError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := ""
	if cid := r.Header.Get("X-Correlation-ID"); cid != "" {
		requestID = cid
	} else if cid := r.Header.Get("X-Request-ID"); cid != "" {
		requestID = cid
	}
	writeHTTPJSON(w, status, ticketError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}
