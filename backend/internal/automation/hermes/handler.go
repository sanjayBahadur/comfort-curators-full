package hermes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *HermesService
}

func NewHandler(svc *HermesService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/hermes/drafts", h.handleCreateDraft)
	mux.HandleFunc("POST /v1/hermes/drafts/{draft_id}/review", h.handleReviewDraft)
	mux.HandleFunc("POST /v1/hermes/drafts/{draft_id}/deliver", h.handleDeliverDraft)
	mux.HandleFunc("GET /v1/hermes/deliveries", h.handleListDeliveries)
	mux.HandleFunc("GET /v1/hermes/deliveries/{delivery_id}", h.handleGetDelivery)
}

func (h *Handler) handleCreateDraft(w http.ResponseWriter, r *http.Request) {
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

	var params DraftParams
	if err := json.Unmarshal(body, &params); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	if params.TenantID == "" {
		params.TenantID = tenantID
	}
	if params.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "tenant mismatch: draft arguments cannot select another tenant")
		return
	}
	if params.ActorID == "" {
		params.ActorID = actorID
	}

	draft, err := h.svc.Draft(r.Context(), params)
	if err != nil {
		if errors.Is(err, ErrInvalidAudience) || errors.Is(err, ErrPurposeRequired) ||
			errors.Is(err, ErrFactsRequired) || errors.Is(err, ErrUnapprovedFact) ||
			errors.Is(err, ErrAudienceMismatch) || errors.Is(err, ErrTemplateKeyRequired) ||
			errors.Is(err, ErrFreeFormContentRequired) || errors.Is(err, ErrInvalidFactAudience) {
			writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, draft)
}

func (h *Handler) handleReviewDraft(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")
	if draftID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "draft_id is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var params ReviewParams
	if err := json.Unmarshal(body, &params); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	draft, err := h.svc.Review(r.Context(), tenantID, draftID, params)
	if err != nil {
		if errors.Is(err, ErrReviewerRequired) || errors.Is(err, ErrReviewDecisionRequired) ||
			errors.Is(err, ErrReviewerIsRequester) || errors.Is(err, ErrReviewNotRequired) ||
			errors.Is(err, ErrDraftNotFound) {
			writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, draft)
}

func (h *Handler) handleDeliverDraft(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")
	if draftID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "draft_id is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var params DeliveryParams
	if err := json.Unmarshal(body, &params); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON: "+err.Error())
		return
	}

	params.TenantID = tenantID
	params.DraftID = draftID

	delivery, err := h.svc.Deliver(r.Context(), params)
	if err != nil {
		if errors.Is(err, ErrDraftNotFound) || errors.Is(err, ErrDraftNotApproved) ||
			errors.Is(err, ErrDraftRequiresReview) {
			writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, delivery)
}

func (h *Handler) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	deliveries, err := h.svc.ListDeliveries(r.Context(), tenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if deliveries == nil {
		deliveries = []HermesDelivery{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deliveries": deliveries,
		"count":      len(deliveries),
	})
}

func (h *Handler) handleGetDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	deliveryID := r.PathValue("delivery_id")
	if deliveryID == "" {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "delivery_id is required")
		return
	}

	delivery, err := h.svc.GetDelivery(r.Context(), tenantID, deliveryID)
	if err != nil {
		if errors.Is(err, ErrDeliveryNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, delivery)
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, roles []string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", nil, errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, subject.Roles, nil
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
