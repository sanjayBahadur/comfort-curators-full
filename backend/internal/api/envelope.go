package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/logging"
	"comfort-curators-backend/internal/platform/security"
)

type Resource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

type Collection struct {
	Items      []Resource `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type ErrorBody struct {
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

func writeResource(w http.ResponseWriter, status int, id string, version int, data any) {
	writeJSON(w, status, Resource{ID: id, Version: version, Data: data})
}

func writeCollection(w http.ResponseWriter, items []Resource, nextCursor *string) {
	writeJSON(w, http.StatusOK, Collection{Items: items, NextCursor: nextCursor})
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := correlationID(r)
	writeJSON(w, status, ErrorBody{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func writeErrorDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	requestID := correlationID(r)
	writeJSON(w, status, ErrorBody{
		RequestID: requestID,
		Code:      code,
		Message:   message,
		Details:   details,
	})
}

func correlationID(r *http.Request) string {
	if cid := r.Header.Get("X-Correlation-ID"); cid != "" {
		return cid
	}
	if cid := r.Header.Get("X-Request-ID"); cid != "" {
		return cid
	}
	// The platform middleware copies a generated correlation id onto the
	// request context; prefer it over synthesizing a fresh one so the error
	// envelope matches the id the caller can observe on the response header.
	if cid := logging.CorrelationIDFromCtx(r.Context()); cid != "" {
		return cid
	}
	if cid := logging.RequestIDFromCtx(r.Context()); cid != "" {
		return cid
	}
	return newCorrelationID()
}

// newCorrelationID generates a stable, schema-conforming correlation id when
// neither the request headers nor the middleware context supply one. This
// closes the residual conformance gap where a live handler without a client
// correlation id would otherwise emit an Error envelope with an empty
// request_id, which the contract forbids.
func newCorrelationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(b[:])
}

func writeETag(w http.ResponseWriter, version int) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, version))
}

func subjectFromRequest(r *http.Request) (security.Subject, error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return security.Subject{}, fmt.Errorf("unauthenticated")
	}
	return subject, nil
}
