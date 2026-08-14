package operations

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type DispatchHandler struct {
	dispatchSvc *DispatchService
}

func NewDispatchHandler(dispatchSvc *DispatchService) *DispatchHandler {
	return &DispatchHandler{dispatchSvc: dispatchSvc}
}

func (h *DispatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/dispatch/candidates", h.handleEvaluateCandidates)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/dispatch/assign", h.handleAssignWorker)
	mux.HandleFunc("POST /v1/tickets/{ticket_id}/dispatch/override", h.handleOverrideAssignment)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/dispatch/assignments", h.handleListAssignmentsForTicket)
	mux.HandleFunc("GET /v1/tickets/{ticket_id}/dispatch/overrides", h.handleListOverridesForTicket)
	mux.HandleFunc("GET /v1/dispatch/assignments/{assignment_id}", h.handleGetAssignment)
	mux.HandleFunc("POST /v1/dispatch/assignments/{assignment_id}/accept", h.handleAcceptAssignment)
	mux.HandleFunc("POST /v1/dispatch/assignments/{assignment_id}/decline", h.handleDeclineAssignment)
	mux.HandleFunc("GET /v1/dispatch/workers/{worker_id}/assignments", h.handleListAssignmentsForWorker)
	mux.HandleFunc("GET /v1/dispatch/workers/{worker_id}/treatment", h.handleGetPayTreatment)
}

func (h *DispatchHandler) handleEvaluateCandidates(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	var req DispatchCandidatesRequest
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		json.Unmarshal(body, &req)
	}

	workType := req.WorkType
	if workType == "" {
		ticket, err := h.dispatchSvc.ticketStore.GetTicket(r.Context(), subject.TenantID, ticketID)
		if err == nil {
			workType = ticket.Type
		}
	}

	resp, err := h.dispatchSvc.EvaluateCandidates(r.Context(), subject.TenantID, ticketID, workType)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, dispatchCandidatesResource{Data: resp})
}

func (h *DispatchHandler) handleAssignWorker(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	var req DispatchAssignRequest
	body, bodyErr := io.ReadAll(r.Body)
	if bodyErr != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil || req.WorkerID == "" {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "worker_id is required")
		return
	}

	ticket, err := h.dispatchSvc.ticketStore.GetTicket(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		code := "NOT_FOUND"
		status := http.StatusNotFound
		if errors.Is(err, ErrTicketNotFound) {
			code = "NOT_FOUND"
		} else {
			code = "INTERNAL_ERROR"
			status = http.StatusInternalServerError
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	assignment, payTreatment, err := h.dispatchSvc.AssignWorker(r.Context(), subject.TenantID, ticketID, req.WorkerID, ticket.Type, subject.ActorID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrDispatchWorkerNotEligible) ||
			errors.Is(err, ErrDispatchTicketNotAssignable) ||
			errors.Is(err, ErrDispatchTwoPersonRequired) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	data := map[string]any{
		"assignment":    assignment,
		"pay_treatment": payTreatment,
	}
	writeHTTPResource(w, http.StatusCreated, assignment.ID, assignment.Version, data)
}

func (h *DispatchHandler) handleOverrideAssignment(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")

	var req DispatchOverrideRequest
	body, bodyErr := io.ReadAll(r.Body)
	if bodyErr != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	ticket, ticketErr := h.dispatchSvc.ticketStore.GetTicket(r.Context(), subject.TenantID, ticketID)
	if ticketErr != nil {
		writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", ticketErr.Error())
		return
	}

	assignment, payTreatment, override, err := h.dispatchSvc.OverrideAssignment(
		r.Context(), subject.TenantID, ticketID, req.WorkerID, ticket.Type, req, subject.ActorID,
	)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrDispatchOverrideRequiresReason) ||
			errors.Is(err, ErrDispatchOverrideRequiresConstraint) ||
			errors.Is(err, ErrDispatchOverrideNotAttributed) ||
			errors.Is(err, ErrDispatchTicketNotAssignable) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	data := map[string]any{
		"assignment":    assignment,
		"pay_treatment": payTreatment,
		"override":      override,
	}
	writeHTTPResource(w, http.StatusCreated, assignment.ID, assignment.Version, data)
}

func (h *DispatchHandler) handleListAssignmentsForTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")
	assignments, err := h.dispatchSvc.ListAssignmentsForTicket(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if assignments == nil {
		assignments = []TicketAssignment{}
	}

	var items []ticketResource
	for _, a := range assignments {
		items = append(items, ticketResource{ID: a.ID, Version: a.Version, Data: a})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *DispatchHandler) handleListOverridesForTicket(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticketID := r.PathValue("ticket_id")
	overrides, err := h.dispatchSvc.ListOverridesForTicket(r.Context(), subject.TenantID, ticketID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if overrides == nil {
		overrides = []DispatchOverride{}
	}

	var items []ticketResource
	for _, o := range overrides {
		items = append(items, ticketResource{ID: o.ID, Version: 1, Data: o})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *DispatchHandler) handleGetAssignment(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	assignmentID := r.PathValue("assignment_id")
	assignment, payTreatment, err := h.dispatchSvc.GetAssignment(r.Context(), subject.TenantID, assignmentID)
	if err != nil {
		code := "NOT_FOUND"
		status := http.StatusNotFound
		if errors.Is(err, ErrDispatchAssignmentNotFound) {
			code = "NOT_FOUND"
		} else {
			code = "INTERNAL_ERROR"
			status = http.StatusInternalServerError
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	data := map[string]any{
		"assignment":    assignment,
		"pay_treatment": payTreatment,
	}
	writeHTTPResource(w, http.StatusOK, assignment.ID, assignment.Version, data)
}

func (h *DispatchHandler) handleAcceptAssignment(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	assignmentID := r.PathValue("assignment_id")

	var req DispatchAcceptRequest
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		json.Unmarshal(body, &req)
	}

	workerID := req.WorkerID
	if workerID == "" {
		workerID = subject.ActorID
	}

	assignment, payTreatment, err := h.dispatchSvc.AcceptAssignment(r.Context(), subject.TenantID, assignmentID, workerID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrDispatchAssignmentNotOffered) ||
			errors.Is(err, ErrDispatchNotWorker) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	data := map[string]any{
		"assignment":    assignment,
		"pay_treatment": payTreatment,
	}
	writeHTTPResource(w, http.StatusOK, assignment.ID, assignment.Version, data)
}

func (h *DispatchHandler) handleDeclineAssignment(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	assignmentID := r.PathValue("assignment_id")

	var req DispatchAcceptRequest
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		json.Unmarshal(body, &req)
	}

	workerID := req.WorkerID
	if workerID == "" {
		workerID = subject.ActorID
	}

	assignment, err := h.dispatchSvc.DeclineAssignment(r.Context(), subject.TenantID, assignmentID, workerID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrDispatchAssignmentNotOffered) ||
			errors.Is(err, ErrDispatchNotWorker) {
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPResource(w, http.StatusOK, assignment.ID, assignment.Version, assignment)
}

func (h *DispatchHandler) handleListAssignmentsForWorker(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	assignments, err := h.dispatchSvc.ListAssignmentsForWorker(r.Context(), subject.TenantID, workerID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if assignments == nil {
		assignments = []TicketAssignment{}
	}

	var items []ticketResource
	for _, a := range assignments {
		items = append(items, ticketResource{ID: a.ID, Version: a.Version, Data: a})
	}
	writeHTTPCollection(w, items, nil)
}

func (h *DispatchHandler) handleGetPayTreatment(w http.ResponseWriter, r *http.Request) {
	subject, err := getSubject(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	workerID := r.PathValue("worker_id")
	treatment, err := h.dispatchSvc.GetPayTreatment(r.Context(), subject.TenantID, workerID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, dispatchCandidatesResource{Data: treatment})
}
