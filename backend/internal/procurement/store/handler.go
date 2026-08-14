package store

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"comfort-curators-backend/internal/iam"
)

// Handler exposes the guest-facing store flow. Ordering remains deliberately
// outside the Superhost tool registry; the caller must be an authenticated
// human using these HTTP routes.
type Handler struct {
	provider StoreProvider
}

func NewHandler(provider StoreProvider) *Handler {
	return &Handler{provider: provider}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/store/catalog", h.handleSearch)
	mux.HandleFunc("POST /v1/store/quotes", h.handleQuote)
	mux.HandleFunc("POST /v1/store/orders", h.handlePlaceOrder)
}

type errorBody struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorBody{
		RequestID: r.Header.Get("X-Correlation-ID"),
		Code:      code,
		Message:   message,
	})
}

func subjectFromRequest(r *http.Request) (string, string, error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || strings.TrimSpace(subject.TenantID) == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func decodeBody(r *http.Request, value any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(value); err != nil {
		return err
	}
	return nil
}

func providerError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	if errors.Is(err, ErrInvalidRequest) {
		status, code = http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	} else if errors.Is(err, ErrItemNotFound) {
		status, code = http.StatusNotFound, "NOT_FOUND"
	}
	writeError(w, r, status, code, err.Error())
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if _, _, err := subjectFromRequest(r); err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	// Catalog search is guest/property scoped at the HTTP boundary. The
	// provider interface is intentionally only a catalog boundary, so it does
	// not receive scope values and cannot persist or broaden them.
	if strings.TrimSpace(r.URL.Query().Get("property_id")) == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id is required")
		return
	}
	items, err := h.provider.Search(r.Context(), r.URL.Query().Get("query"), r.URL.Query().Get("provider"))
	if err != nil {
		providerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) handleQuote(w http.ResponseWriter, r *http.Request) {
	if _, _, err := subjectFromRequest(r); err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	var request QuoteRequest
	if err := decodeBody(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	quote, err := h.provider.Quote(r.Context(), request)
	if err != nil {
		providerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, quote)
}

func (h *Handler) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	var request PlaceOrderRequest
	if err := decodeBody(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if request.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "tenant scope mismatch")
		return
	}
	if strings.TrimSpace(request.PropertyID) == "" {
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "property_id is required")
		return
	}
	if request.GuestID == "" {
		request.GuestID = actorID
	}
	confirmation, err := h.provider.PlaceOrder(r.Context(), request)
	if err != nil {
		providerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, confirmation)
}
