package reservations

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type CalendarHandler struct {
	svc *CalendarService
}

func NewCalendarHandler(svc *CalendarService) *CalendarHandler {
	return &CalendarHandler{svc: svc}
}

func (h *CalendarHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/properties/{property_id}/calendar-feeds", h.handleCreateFeed)
	mux.HandleFunc("GET /v1/properties/{property_id}/calendar-feeds", h.handleListFeeds)
	mux.HandleFunc("GET /v1/properties/{property_id}/calendar-feeds/{feed_id}", h.handleGetFeed)
	mux.HandleFunc("PUT /v1/calendar-feeds/{feed_id}/status", h.handleSetFeedStatus)
	mux.HandleFunc("POST /v1/calendar-feeds/{feed_id}/polls", h.handlePollFeed)
	mux.HandleFunc("GET /v1/properties/{property_id}/calendar-events", h.handleListEvents)
	mux.HandleFunc("GET /v1/properties/{property_id}/calendar-exceptions", h.handleListExceptions)
	mux.HandleFunc("POST /v1/calendar-exceptions/{exception_id}/resolve", h.handleResolveException)
	mux.HandleFunc("GET /v1/properties/{property_id}/calendar-health", h.handleFeedHealth)
	mux.HandleFunc("GET /v1/properties/{property_id}/reservations", h.handleListReservations)
	mux.HandleFunc("GET /v1/properties/{property_id}/reservation-conflicts", h.handleListConflicts)
	mux.HandleFunc("POST /v1/reservation-conflicts/{conflict_id}/resolve", h.handleResolveConflict)
	mux.HandleFunc("GET /v1/properties/{property_id}/turnover-proposals", h.handleListProposals)
	mux.HandleFunc("POST /v1/properties/{property_id}/turnover-proposals/generate", h.handleGenerateProposals)
}

type calendarError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type calendarResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

func writeHTTPJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeHTTPError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	writeHTTPJSON(w, status, calendarError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

type calendarSubject struct {
	TenantID string
	ActorID  string
	Roles    []string
}

func subjectFromHTTPRequest(r *http.Request) (calendarSubject, error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return calendarSubject{}, fmt.Errorf("unauthenticated")
	}
	return calendarSubject{
		TenantID: subject.TenantID,
		ActorID:  subject.ActorID,
		Roles:    subject.Roles,
	}, nil
}

type createFeedRequest struct {
	Source                   string `json:"source"`
	URL                      string `json:"url"`
	PropertyTimezone         string `json:"property_timezone,omitempty"`
	StaleAfterMinutes        int    `json:"stale_after_minutes,omitempty"`
	MinimumTurnaroundMinutes int    `json:"minimum_turnaround_minutes,omitempty"`
}

func (h *CalendarHandler) handleCreateFeed(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req createFeedRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Source == "" || req.URL == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "source and url are required")
		return
	}

	feed, err := h.svc.CreateFeed(r.Context(), FeedParams{
		TenantID:                 subj.TenantID,
		PropertyID:               propertyID,
		Source:                   req.Source,
		URL:                      req.URL,
		PropertyTimezone:         req.PropertyTimezone,
		StaleAfterMinutes:        req.StaleAfterMinutes,
		MinimumTurnaroundMinutes: req.MinimumTurnaroundMinutes,
	}, subj.Roles)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSuperhostCannotMutate):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case errors.Is(err, ErrInvalidFeed):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusCreated, calendarResource{
		ID:      feed.ID,
		Version: feed.Version,
		Data:    feed,
	})
}

func (h *CalendarHandler) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	feeds, err := h.svc.ListFeeds(r.Context(), subj.TenantID, propertyID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if feeds == nil {
		feeds = []CalendarFeed{}
	}

	resources := make([]calendarResource, 0, len(feeds))
	for _, f := range feeds {
		resources = append(resources, calendarResource{
			ID:      f.ID,
			Version: f.Version,
			Data:    f,
		})
	}

	writeHTTPJSON(w, http.StatusOK, map[string]any{
		"items": resources,
	})
}

func (h *CalendarHandler) handleGetFeed(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	feedID := r.PathValue("feed_id")

	feeds, err := h.svc.ListFeeds(r.Context(), subj.TenantID, r.PathValue("property_id"))
	if err != nil {
		if errors.Is(err, ErrFeedNotFound) {
			writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "calendar feed not found")
			return
		}
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	for _, f := range feeds {
		if f.ID == feedID {
			writeHTTPJSON(w, http.StatusOK, calendarResource{
				ID:      f.ID,
				Version: f.Version,
				Data:    f,
			})
			return
		}
	}
	writeHTTPError(w, r, http.StatusNotFound, "NOT_FOUND", "calendar feed not found")
}

func (h *CalendarHandler) handleSetFeedStatus(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	feedID := r.PathValue("feed_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Status == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "status is required")
		return
	}

	feed, err := h.svc.SetFeedStatus(r.Context(), subj.TenantID, feedID, req.Status, subj.Roles)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSuperhostCannotMutate):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case errors.Is(err, ErrFeedNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrInvalidFeed):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, calendarResource{
		ID:      feed.ID,
		Version: feed.Version,
		Data:    feed,
	})
}

func (h *CalendarHandler) handlePollFeed(w http.ResponseWriter, r *http.Request) {
	_, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	feedID := r.PathValue("feed_id")

	result, err := h.svc.PollFeed(r.Context(), feedID)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		if errors.Is(err, ErrFeedNotFound) {
			code = "NOT_FOUND"
			status = http.StatusNotFound
		} else if errors.Is(err, ErrFeedNotActive) {
			code = "INVALID_STATE"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, map[string]any{
		"status": "accepted",
		"result": result,
	})
}

func (h *CalendarHandler) handleListEvents(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	events, err := h.svc.ListEvents(r.Context(), subj.TenantID, propertyID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if events == nil {
		events = []ExternalCalendarEvent{}
	}

	resources := make([]calendarResource, 0, len(events))
	for _, ev := range events {
		resources = append(resources, calendarResource{
			ID:      ev.ID,
			Version: 1,
			Data:    ev,
		})
	}

	writeHTTPJSON(w, http.StatusOK, map[string]any{
		"items": resources,
	})
}

func (h *CalendarHandler) handleListExceptions(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	exceptions, err := h.svc.ListExceptions(r.Context(), subj.TenantID, propertyID)
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if exceptions == nil {
		exceptions = []CalendarException{}
	}

	resources := make([]calendarResource, 0, len(exceptions))
	for _, exc := range exceptions {
		resources = append(resources, calendarResource{
			ID:      exc.ID,
			Version: 1,
			Data:    exc,
		})
	}

	writeHTTPJSON(w, http.StatusOK, map[string]any{
		"items": resources,
	})
}

func (h *CalendarHandler) handleResolveException(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	exceptionID := r.PathValue("exception_id")

	resolved, err := h.svc.ResolveException(r.Context(), subj.TenantID, exceptionID, subj.Roles)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSuperhostCannotMutate):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case errors.Is(err, ErrExceptionNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, calendarResource{
		ID:      resolved.ID,
		Version: 1,
		Data:    resolved,
	})
}

func (h *CalendarHandler) handleFeedHealth(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	health, err := h.svc.FeedHealth(r.Context(), subj.TenantID, propertyID, time.Now().UTC())
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if health == nil {
		health = []FeedHealth{}
	}

	writeHTTPJSON(w, http.StatusOK, map[string]any{
		"feeds": health,
	})
}

func (h *CalendarHandler) handleListReservations(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	reservations, err := h.svc.ListReservations(r.Context(), subj.TenantID, r.PathValue("property_id"))
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if reservations == nil {
		reservations = []Reservation{}
	}

	items := make([]calendarResource, 0, len(reservations))
	for _, rsv := range reservations {
		items = append(items, calendarResource{ID: rsv.ID, Version: rsv.Version, Data: rsv})
	}
	writeHTTPJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *CalendarHandler) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	conflicts, err := h.svc.ListConflicts(r.Context(), subj.TenantID, r.PathValue("property_id"))
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if conflicts == nil {
		conflicts = []ReservationConflict{}
	}

	items := make([]calendarResource, 0, len(conflicts))
	for _, c := range conflicts {
		items = append(items, calendarResource{ID: c.ID, Version: 1, Data: c})
	}
	writeHTTPJSON(w, http.StatusOK, map[string]any{"items": items})
}

type resolveConflictRequest struct {
	Outcome string `json:"outcome"`
	Note    string `json:"note,omitempty"`
}

func (h *CalendarHandler) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
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
	var req resolveConflictRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeHTTPError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.Outcome == "" {
		writeHTTPError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "outcome is required")
		return
	}

	resolved, err := h.svc.ResolveConflict(r.Context(), subj.TenantID, conflictID, req.Outcome, req.Note, subj.ActorID, "user", subj.Roles)
	if err != nil {
		code := "INTERNAL_ERROR"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSuperhostCannotMutate):
			code = "FORBIDDEN"
			status = http.StatusForbidden
		case errors.Is(err, ErrConflictNotFound):
			code = "NOT_FOUND"
			status = http.StatusNotFound
		case errors.Is(err, ErrConflictAlreadyResolved):
			code = "INVALID_STATE"
			status = http.StatusConflict
		case errors.Is(err, ErrInvalidResolution):
			code = "VALIDATION_ERROR"
			status = http.StatusUnprocessableEntity
		}
		writeHTTPError(w, r, status, code, err.Error())
		return
	}

	writeHTTPJSON(w, http.StatusOK, calendarResource{ID: resolved.ID, Version: 1, Data: resolved})
}

func (h *CalendarHandler) handleListProposals(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	proposals, err := h.svc.ListProposals(r.Context(), subj.TenantID, r.PathValue("property_id"))
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if proposals == nil {
		proposals = []TurnoverProposal{}
	}

	items := make([]calendarResource, 0, len(proposals))
	for _, p := range proposals {
		items = append(items, calendarResource{ID: p.ID, Version: p.Version, Data: p})
	}
	writeHTTPJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *CalendarHandler) handleGenerateProposals(w http.ResponseWriter, r *http.Request) {
	subj, err := subjectFromHTTPRequest(r)
	if err != nil {
		writeHTTPError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	result, err := h.svc.GenerateTurnoverProposals(r.Context(), subj.TenantID, r.PathValue("property_id"), time.Now().UTC())
	if err != nil {
		writeHTTPError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeHTTPJSON(w, http.StatusOK, map[string]any{"result": result})
}
