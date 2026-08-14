package documents

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/documents", h.handleCreateDocument)
	mux.HandleFunc("POST /v1/documents/{document_id}/versions", h.handleCreateVersion)
	mux.HandleFunc("POST /v1/documents/{document_id}/reviews", h.handleReviewDocument)
	mux.HandleFunc("POST /v1/submission-packets/{packet_id}/confirmations", h.handleConfirmSubmissionPacket)

	mux.HandleFunc("GET /v1/documents/{document_id}", h.handleGetDocument)
	mux.HandleFunc("GET /v1/properties/{property_id}/documents", h.handleListDocuments)
	mux.HandleFunc("GET /v1/documents/{document_id}/versions", h.handleListVersions)
	mux.HandleFunc("GET /v1/document-versions/{version_id}/extractions", h.handleListExtractions)
	mux.HandleFunc("GET /v1/documents/{document_id}/reviews", h.handleListReviews)
	mux.HandleFunc("POST /v1/document-versions/{version_id}/extractions", h.handleCreateExtraction)
	mux.HandleFunc("POST /v1/properties/{property_id}/submission-packets", h.handleCreateSubmissionPacket)
	mux.HandleFunc("GET /v1/submission-packets/{packet_id}", h.handleGetSubmissionPacket)
	mux.HandleFunc("GET /v1/submission-packets/{packet_id}/receipt", h.handleGetReceipt)
	mux.HandleFunc("POST /v1/properties/{property_id}/documents/expiry-check", h.handleCheckExpiry)
}

type apiResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
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

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	writeJSON(w, status, apiError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func documentView(d *Document) map[string]any {
	m := map[string]any{
		"id":              d.ID,
		"tenant_id":       d.TenantID,
		"property_id":     d.PropertyID,
		"title":           d.Title,
		"document_type":   d.DocumentType,
		"status":          d.Status,
		"current_version": d.CurrentVersion,
		"version":         d.Version,
		"created_at":      d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":      d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if d.ExpiresAt != nil {
		m["expires_at"] = d.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return m
}

func versionView(v *DocumentVersion) map[string]any {
	return map[string]any{
		"id":             v.ID,
		"document_id":    v.DocumentID,
		"tenant_id":      v.TenantID,
		"version_number": v.VersionNumber,
		"content_hash":   v.ContentHash,
		"object_key":     v.ObjectKey,
		"filename":       v.Filename,
		"content_type":   v.ContentType,
		"size_bytes":     v.SizeBytes,
		"uploaded_by":    v.UploadedBy,
		"metadata":       v.Metadata,
		"created_at":     v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func extractionView(e *Extraction) map[string]any {
	return map[string]any{
		"id":                  e.ID,
		"document_version_id": e.DocumentVersionID,
		"tenant_id":           e.TenantID,
		"field_name":          e.FieldName,
		"field_value":         e.FieldValue,
		"field_category":      e.FieldCategory,
		"source_location":     e.SourceLocation,
		"confidence":          e.Confidence,
		"confidence_score":    e.ConfidenceScore,
		"extracted_by":        e.ExtractedBy,
		"extracted_at":        e.ExtractedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func reviewView(r *Review) map[string]any {
	return map[string]any{
		"id":                  r.ID,
		"document_id":         r.DocumentID,
		"document_version_id": r.DocumentVersionID,
		"tenant_id":           r.TenantID,
		"reviewer_id":         r.ReviewerID,
		"status":              r.Status,
		"decision":            r.Decision,
		"comments":            r.Comments,
		"reviewed_at":         r.ReviewedAt.Format("2006-01-02T15:04:05Z07:00"),
		"created_at":          r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func packetView(p *SubmissionPacket) map[string]any {
	m := map[string]any{
		"id":           p.ID,
		"tenant_id":    p.TenantID,
		"property_id":  p.PropertyID,
		"status":       p.Status,
		"document_ids": p.DocumentIDs,
		"created_by":   p.CreatedBy,
		"version":      p.Version,
		"created_at":   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.SubmittedAt != nil {
		m["submitted_at"] = p.SubmittedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return m
}

func receiptView(r *SubmissionReceipt) map[string]any {
	refs := make([]map[string]any, 0, len(r.DocumentVersionRefs))
	for _, ref := range r.DocumentVersionRefs {
		refs = append(refs, map[string]any{
			"document_id":         ref.DocumentID,
			"document_version_id": ref.DocumentVersionID,
			"version_number":      ref.VersionNumber,
			"content_hash":        ref.ContentHash,
		})
	}
	return map[string]any{
		"id":                    r.ID,
		"packet_id":             r.PacketID,
		"tenant_id":             r.TenantID,
		"confirmed_by":          r.ConfirmedBy,
		"receipt_hash":          r.ReceiptHash,
		"document_version_refs": refs,
		"confirmed_at":          r.ConfirmedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// POST /v1/documents
func (h *Handler) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		Title        string `json:"title"`
		DocumentType string `json:"document_type"`
		PropertyID   string `json:"property_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	doc, err := h.svc.CreateDocument(r.Context(), tenantID, CreateDocumentParams{
		Title:        req.Title,
		DocumentType: req.DocumentType,
		PropertyID:   req.PropertyID,
	}, actorID)
	if err != nil {
		if errors.Is(err, ErrInvalidDocument) {
			writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      doc.ID,
		Version: doc.Version,
		Data:    documentView(doc),
	})
}

// POST /v1/documents/{document_id}/versions
func (h *Handler) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		ContentHash string `json:"content_hash"`
		ObjectKey   string `json:"object_key"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		SizeBytes   int64  `json:"size_bytes"`
		Metadata    string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	ver, doc, err := h.svc.CreateVersion(r.Context(), tenantID, documentID, CreateVersionParams{
		ContentHash: req.ContentHash,
		ObjectKey:   req.ObjectKey,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		Metadata:    req.Metadata,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrDocumentNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		case errors.Is(err, ErrInvalidVersion):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrDocumentExpired):
			status = http.StatusUnprocessableEntity
			code = "INVALID_STATE"
		case errors.Is(err, ErrDuplicateVersion):
			status = http.StatusConflict
			code = "DUPLICATE"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      ver.ID,
		Version: ver.VersionNumber,
		Data: map[string]any{
			"version":  versionView(ver),
			"document": documentView(doc),
		},
	})
}

// POST /v1/documents/{document_id}/reviews
func (h *Handler) handleReviewDocument(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		Status   string `json:"status"`
		Decision string `json:"decision"`
		Comments string `json:"comments"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	review, err := h.svc.ReviewDocument(r.Context(), tenantID, documentID, CreateReviewParams{
		Status:   req.Status,
		Decision: req.Decision,
		Comments: req.Comments,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrDocumentNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		case errors.Is(err, ErrInvalidReview):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      review.ID,
		Version: 1,
		Data:    reviewView(review),
	})
}

// POST /v1/submission-packets/{packet_id}/confirmations
func (h *Handler) handleConfirmSubmissionPacket(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	packetID := r.PathValue("packet_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		ReviewerAuth string `json:"reviewer_auth"`
	}
	json.Unmarshal(body, &req)

	receipt, packet, err := h.svc.ConfirmSubmission(r.Context(), tenantID, packetID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrSubmissionPacketNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		case errors.Is(err, ErrPacketAlreadySubmitted):
			status = http.StatusConflict
			code = "ALREADY_SUBMITTED"
		case errors.Is(err, ErrHumanReviewRequired):
			status = http.StatusUnprocessableEntity
			code = "HUMAN_REVIEW_REQUIRED"
		case errors.Is(err, ErrAICannotCertify):
			status = http.StatusForbidden
			code = "AI_NOT_AUTHORIZED"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"packet":  packetView(packet),
		"receipt": receiptView(receipt),
	})
}

// GET /v1/documents/{document_id}
func (h *Handler) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")
	doc, err := h.svc.GetDocument(r.Context(), tenantID, documentID)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "document not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      doc.ID,
		Version: doc.Version,
		Data:    documentView(doc),
	})
}

// GET /v1/properties/{property_id}/documents
func (h *Handler) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	docs, err := h.svc.ListDocuments(r.Context(), tenantID, propertyID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if docs == nil {
		docs = []Document{}
	}

	resources := make([]apiResource, 0, len(docs))
	for i := range docs {
		resources = append(resources, apiResource{
			ID:      docs[i].ID,
			Version: docs[i].Version,
			Data:    documentView(&docs[i]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

// GET /v1/documents/{document_id}/versions
func (h *Handler) handleListVersions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")
	versions, err := h.svc.ListVersions(r.Context(), tenantID, documentID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if versions == nil {
		versions = []DocumentVersion{}
	}

	items := make([]map[string]any, 0, len(versions))
	for i := range versions {
		items = append(items, versionView(&versions[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// GET /v1/document-versions/{version_id}/extractions
func (h *Handler) handleListExtractions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	versionID := r.PathValue("version_id")
	extractions, err := h.svc.ListExtractions(r.Context(), tenantID, versionID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if extractions == nil {
		extractions = []Extraction{}
	}

	items := make([]map[string]any, 0, len(extractions))
	for i := range extractions {
		items = append(items, extractionView(&extractions[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// GET /v1/documents/{document_id}/reviews
func (h *Handler) handleListReviews(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	documentID := r.PathValue("document_id")
	reviews, err := h.svc.ListReviews(r.Context(), tenantID, documentID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if reviews == nil {
		reviews = []Review{}
	}

	items := make([]map[string]any, 0, len(reviews))
	for i := range reviews {
		items = append(items, reviewView(&reviews[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// POST /v1/document-versions/{version_id}/extractions
func (h *Handler) handleCreateExtraction(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	versionID := r.PathValue("version_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		FieldName       string  `json:"field_name"`
		FieldValue      string  `json:"field_value"`
		FieldCategory   string  `json:"field_category"`
		SourceLocation  string  `json:"source_location"`
		Confidence      string  `json:"confidence"`
		ConfidenceScore float64 `json:"confidence_score"`
		ExtractedBy     string  `json:"extracted_by"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	ext, err := h.svc.CreateExtraction(r.Context(), tenantID, versionID, CreateExtractionParams{
		FieldName:       req.FieldName,
		FieldValue:      req.FieldValue,
		FieldCategory:   req.FieldCategory,
		SourceLocation:  req.SourceLocation,
		Confidence:      req.Confidence,
		ConfidenceScore: req.ConfidenceScore,
		ExtractedBy:     req.ExtractedBy,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrDocumentVersionNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrInvalidExtraction) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      ext.ID,
		Version: 1,
		Data:    extractionView(ext),
	})
}

// POST /v1/properties/{property_id}/submission-packets
func (h *Handler) handleCreateSubmissionPacket(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req struct {
		DocumentIDs []string `json:"document_ids"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	packet, err := h.svc.CreateSubmissionPacket(r.Context(), tenantID, CreateSubmissionPacketParams{
		PropertyID:  propertyID,
		DocumentIDs: req.DocumentIDs,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidSubmissionPacket) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      packet.ID,
		Version: packet.Version,
		Data:    packetView(packet),
	})
}

// GET /v1/submission-packets/{packet_id}
func (h *Handler) handleGetSubmissionPacket(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	packetID := r.PathValue("packet_id")
	packet, err := h.svc.GetSubmissionPacket(r.Context(), tenantID, packetID)
	if err != nil {
		if errors.Is(err, ErrSubmissionPacketNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "submission packet not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      packet.ID,
		Version: packet.Version,
		Data:    packetView(packet),
	})
}

// GET /v1/submission-packets/{packet_id}/receipt
func (h *Handler) handleGetReceipt(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	packetID := r.PathValue("packet_id")
	receipt, err := h.svc.GetReceipt(r.Context(), tenantID, packetID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if receipt == nil {
		writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "no receipt found for this packet")
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      receipt.ID,
		Version: 1,
		Data:    receiptView(receipt),
	})
}

// POST /v1/properties/{property_id}/documents/expiry-check
func (h *Handler) handleCheckExpiry(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	expired, err := h.svc.DetectExpiry(r.Context(), tenantID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	nearing, err := h.svc.FindNearingExpiry(r.Context(), tenantID, 30)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	expiredItems := make([]map[string]any, 0, len(expired))
	for i := range expired {
		expiredItems = append(expiredItems, documentView(&expired[i]))
	}

	nearingItems := make([]map[string]any, 0, len(nearing))
	for i := range nearing {
		nearingItems = append(nearingItems, documentView(&nearing[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"expired":        expiredItems,
		"expired_count":  len(expiredItems),
		"nearing_expiry": nearingItems,
		"nearing_count":  len(nearingItems),
	})
}
