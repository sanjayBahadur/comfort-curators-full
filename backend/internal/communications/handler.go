package communications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type CommunicationsHandler struct {
	svc *CommunicationsService
}

func NewCommunicationsHandler(svc *CommunicationsService) *CommunicationsHandler {
	return &CommunicationsHandler{svc: svc}
}

func (h *CommunicationsHandler) RegisterRoutes(mux *http.ServeMux) {
	// Templates
	mux.HandleFunc("POST /v1/communications/templates", h.handleCreateTemplate)
	mux.HandleFunc("GET /v1/communications/templates", h.handleListTemplates)
	mux.HandleFunc("GET /v1/communications/templates/{template_key}", h.handleGetTemplate)
	mux.HandleFunc("POST /v1/communications/templates/{template_key}/versions", h.handleAddTemplateVersion)
	mux.HandleFunc("GET /v1/communications/templates/{template_key}/resolve", h.handleResolveTemplate)
	mux.HandleFunc("GET /v1/communications/templates/{template_key}/preview", h.handlePreviewTemplate)

	// Preferences
	mux.HandleFunc("PUT /v1/communications/preferences", h.handleSetPreferences)
	mux.HandleFunc("GET /v1/communications/preferences", h.handleGetPreferences)

	// Drafts
	mux.HandleFunc("POST /v1/communications/drafts", h.handleCreateDraft)
	mux.HandleFunc("GET /v1/communications/drafts/{draft_id}", h.handleGetDraft)
	mux.HandleFunc("POST /v1/communications/drafts/{draft_id}/review", h.handleReviewDraft)
	mux.HandleFunc("GET /v1/communications/drafts/{draft_id}/reviews", h.handleListReviews)
	mux.HandleFunc("POST /v1/communications/drafts/{draft_id}/deliver", h.handleDeliver)
	mux.HandleFunc("GET /v1/communications/drafts/{draft_id}/preview", h.handlePreviewDraft)

	// Deliveries
	mux.HandleFunc("GET /v1/communications/deliveries", h.handleListDeliveries)
	mux.HandleFunc("POST /v1/communications/deliveries/{delivery_id}/result", h.handleRecordDeliveryResult)

	// Secure links
	mux.HandleFunc("POST /v1/communications/secure-links", h.handleCreateSecureLink)
	mux.HandleFunc("POST /v1/communications/secure-links/redeem", h.handleRedeemSecureLink)
	mux.HandleFunc("POST /v1/communications/secure-links/{link_id}/revoke", h.handleRevokeSecureLink)
	mux.HandleFunc("GET /v1/communications/secure-links", h.handleListSecureLinks)
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

var ErrNoGetDraft = errors.New("draft retrieval not supported")

func (s *CommunicationsService) GetDraft(ctx context.Context, tenantID, draftID string) (*CommunicationDraft, error) {
	return s.store.GetDraft(ctx, tenantID, draftID)
}

// --- request types ---

type createTemplateRequest struct {
	TemplateKey  string `json:"template_key"`
	Audience     string `json:"audience"`
	ConsentClass string `json:"consent_class"`
	Channel      string `json:"channel,omitempty"`
	Severity     string `json:"severity,omitempty"`
}

type addTemplateVersionRequest struct {
	Language string `json:"language"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

type setPreferencesRequest struct {
	RecipientID           string   `json:"recipient_id"`
	Audience              string   `json:"audience"`
	ConsentTransactional  bool     `json:"consent_transactional"`
	ConsentUrgent         bool     `json:"consent_urgent"`
	ConsentMarketing      bool     `json:"consent_marketing"`
	ConsentSponsored      bool     `json:"consent_sponsored"`
	Channel               string   `json:"channel,omitempty"`
	Severity              string   `json:"severity,omitempty"`
	QuietHoursStartMinute int      `json:"quiet_hours_start_minute"`
	QuietHoursEndMinute   int      `json:"quiet_hours_end_minute"`
	EscalationContacts    []string `json:"escalation_contacts,omitempty"`
}

type createDraftRequest struct {
	Audience     string `json:"audience"`
	RecipientID  string `json:"recipient_id"`
	Source       string `json:"source"`
	TemplateKey  string `json:"template_key,omitempty"`
	ConsentClass string `json:"consent_class,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Language     string `json:"language,omitempty"`
	Subject      string `json:"subject,omitempty"`
	Body         string `json:"body,omitempty"`
}

type reviewDraftRequest struct {
	ReviewerID string `json:"reviewer_id"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
}

type recordDeliveryResultRequest struct {
	Status  string `json:"status"`
	Failure string `json:"failure,omitempty"`
}

type createSecureLinkRequest struct {
	PropertyID  string `json:"property_id"`
	Audience    string `json:"audience"`
	RecipientID string `json:"recipient_id"`
	Purpose     string `json:"purpose,omitempty"`
	ExpiresAt   string `json:"expires_at"`
}

type redeemSecureLinkRequest struct {
	Token string `json:"token"`
}

// --- helpers ---

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

func parseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func templateView(t *MessageTemplate) map[string]any {
	return map[string]any{
		"id":            t.ID,
		"tenant_id":     t.TenantID,
		"template_key":  t.TemplateKey,
		"audience":      t.Audience,
		"consent_class": t.ConsentClass,
		"channel":       t.Channel,
		"severity":      t.Severity,
		"status":        t.Status,
		"created_at":    t.CreatedAt.Format(time.RFC3339),
		"updated_at":    t.UpdatedAt.Format(time.RFC3339),
	}
}

func templateVersionView(v *TemplateVersion) map[string]any {
	return map[string]any{
		"id":          v.ID,
		"tenant_id":   v.TenantID,
		"template_id": v.TemplateID,
		"version":     v.Version,
		"language":    v.Language,
		"subject":     v.Subject,
		"body":        v.Body,
		"created_at":  v.CreatedAt.Format(time.RFC3339),
	}
}

func resolvedTemplateView(r *ResolvedTemplate) map[string]any {
	return map[string]any{
		"template_key": r.Template.TemplateKey,
		"audience":     r.Template.Audience,
		"language":     r.Language,
		"version":      r.Version,
		"subject":      r.Subject,
		"body":         r.Body,
	}
}

func preferencesView(p *CommunicationPreferences) map[string]any {
	return map[string]any{
		"id":                       p.ID,
		"tenant_id":                p.TenantID,
		"recipient_id":             p.RecipientID,
		"audience":                 p.Audience,
		"consent_transactional":    p.ConsentTransactional,
		"consent_urgent":           p.ConsentUrgent,
		"consent_marketing":        p.ConsentMarketing,
		"consent_sponsored":        p.ConsentSponsored,
		"channel":                  p.Channel,
		"severity":                 p.Severity,
		"quiet_hours_start_minute": p.QuietHoursStartMinute,
		"quiet_hours_end_minute":   p.QuietHoursEndMinute,
		"escalation_contacts":      p.EscalationContacts,
		"version":                  p.Version,
		"created_at":               p.CreatedAt.Format(time.RFC3339),
		"updated_at":               p.UpdatedAt.Format(time.RFC3339),
	}
}

func draftView(d *CommunicationDraft) map[string]any {
	return map[string]any{
		"id":              d.ID,
		"tenant_id":       d.TenantID,
		"audience":        d.Audience,
		"recipient_id":    d.RecipientID,
		"source":          d.Source,
		"template_key":    d.TemplateKey,
		"consent_class":   d.ConsentClass,
		"channel":         d.Channel,
		"severity":        d.Severity,
		"subject":         d.Subject,
		"body":            d.Body,
		"status":          d.Status,
		"requires_review": d.RequiresReview,
		"created_at":      d.CreatedAt.Format(time.RFC3339),
		"updated_at":      d.UpdatedAt.Format(time.RFC3339),
	}
}

func reviewView(r *CommunicationReview) map[string]any {
	return map[string]any{
		"id":          r.ID,
		"tenant_id":   r.TenantID,
		"draft_id":    r.DraftID,
		"reviewer_id": r.ReviewerID,
		"decision":    r.Decision,
		"reason":      r.Reason,
		"reviewed_at": r.ReviewedAt.Format(time.RFC3339),
	}
}

func deliveryView(d *Delivery) map[string]any {
	m := map[string]any{
		"id":            d.ID,
		"tenant_id":     d.TenantID,
		"draft_id":      d.DraftID,
		"recipient_id":  d.RecipientID,
		"audience":      d.Audience,
		"consent_class": d.ConsentClass,
		"channel":       d.Channel,
		"status":        d.Status,
		"error":         d.Error,
		"created_at":    d.CreatedAt.Format(time.RFC3339),
		"updated_at":    d.UpdatedAt.Format(time.RFC3339),
	}
	if d.DeliveredAt != nil {
		m["delivered_at"] = d.DeliveredAt.Format(time.RFC3339)
	}
	return m
}

func secureLinkView(l *SecureLink) map[string]any {
	m := map[string]any{
		"id":           l.ID,
		"tenant_id":    l.TenantID,
		"property_id":  l.PropertyID,
		"audience":     l.Audience,
		"recipient_id": l.RecipientID,
		"purpose":      l.Purpose,
		"token_tail":   l.TokenTail,
		"expires_at":   l.ExpiresAt.Format(time.RFC3339),
		"status":       l.Status,
		"created_at":   l.CreatedAt.Format(time.RFC3339),
	}
	if l.UsedAt != nil {
		m["used_at"] = l.UsedAt.Format(time.RFC3339)
	}
	if l.RevokedAt != nil {
		m["revoked_at"] = l.RevokedAt.Format(time.RFC3339)
	}
	return m
}

func previewView(p *Preview) map[string]any {
	return map[string]any{
		"template_key": p.TemplateKey,
		"audience":     p.Audience,
		"language":     p.Language,
		"subject":      p.Subject,
		"body":         p.Body,
	}
}

// --- template handlers ---

func (h *CommunicationsHandler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
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

	var req createTemplateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	template, err := h.svc.CreateTemplate(r.Context(), tenantID, TemplateParams{
		TemplateKey:  req.TemplateKey,
		Audience:     req.Audience,
		ConsentClass: req.ConsentClass,
		Channel:      req.Channel,
		Severity:     req.Severity,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrTemplateKeyRequired) || errors.Is(err, ErrInvalidAudience) || errors.Is(err, ErrInvalidConsentClass) || errors.Is(err, ErrInvalidChannel) || errors.Is(err, ErrInvalidSeverity) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrTemplateExists) {
			status = http.StatusConflict
			code = "CONFLICT"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      template.ID,
		Version: 1,
		Data:    templateView(template),
	})
}

func (h *CommunicationsHandler) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	templateKey := r.PathValue("template_key")
	template, err := h.svc.GetTemplateByKey(r.Context(), tenantID, templateKey)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      template.ID,
		Version: 1,
		Data:    templateView(template),
	})
}

func (h *CommunicationsHandler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	audience := r.URL.Query().Get("audience")
	templates, err := h.svc.ListTemplates(r.Context(), tenantID, audience)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if templates == nil {
		templates = []MessageTemplate{}
	}

	resources := make([]apiResource, 0, len(templates))
	for i := range templates {
		resources = append(resources, apiResource{
			ID:      templates[i].ID,
			Version: 1,
			Data:    templateView(&templates[i]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *CommunicationsHandler) handleAddTemplateVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	templateKey := r.PathValue("template_key")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req addTemplateVersionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	template, err := h.svc.GetTemplateByKey(r.Context(), tenantID, templateKey)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	version, err := h.svc.AddTemplateVersion(r.Context(), tenantID, template.ID, TemplateVersionParams{
		Language: req.Language,
		Subject:  req.Subject,
		Body:     req.Body,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidLanguage) || errors.Is(err, ErrTemplateContentRequired) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      version.ID,
		Version: version.Version,
		Data:    templateVersionView(version),
	})
}

func (h *CommunicationsHandler) handleResolveTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	templateKey := r.PathValue("template_key")
	language := r.URL.Query().Get("language")

	resolved, err := h.svc.ResolveTemplateContent(r.Context(), tenantID, templateKey, language)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) || errors.Is(err, ErrCrossTenantDenied) || errors.Is(err, ErrTemplateVersionMissing) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "template or version not found")
			return
		}
		if errors.Is(err, ErrInvalidLanguage) {
			writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      resolved.Template.ID,
		Version: resolved.Version,
		Data:    resolvedTemplateView(resolved),
	})
}

func (h *CommunicationsHandler) handlePreviewTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	templateKey := r.PathValue("template_key")
	language := r.URL.Query().Get("language")

	preview, err := h.svc.PreviewTemplate(r.Context(), tenantID, templateKey, language)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) || errors.Is(err, ErrCrossTenantDenied) || errors.Is(err, ErrTemplateVersionMissing) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "template or version not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, previewView(preview))
}

// --- preferences handlers ---

func (h *CommunicationsHandler) handleSetPreferences(w http.ResponseWriter, r *http.Request) {
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

	var req setPreferencesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	prefs, err := h.svc.SetPreferences(r.Context(), tenantID, PreferencesParams{
		RecipientID:           req.RecipientID,
		Audience:              req.Audience,
		ConsentTransactional:  req.ConsentTransactional,
		ConsentUrgent:         req.ConsentUrgent,
		ConsentMarketing:      req.ConsentMarketing,
		ConsentSponsored:      req.ConsentSponsored,
		Channel:               req.Channel,
		Severity:              req.Severity,
		QuietHoursStartMinute: req.QuietHoursStartMinute,
		QuietHoursEndMinute:   req.QuietHoursEndMinute,
		EscalationContacts:    req.EscalationContacts,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidPreferences) || errors.Is(err, ErrInvalidAudience) || errors.Is(err, ErrInvalidChannel) || errors.Is(err, ErrInvalidSeverity) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      prefs.ID,
		Version: prefs.Version,
		Data:    preferencesView(prefs),
	})
}

func (h *CommunicationsHandler) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	recipientID := r.URL.Query().Get("recipient_id")
	audience := r.URL.Query().Get("audience")

	prefs, err := h.svc.GetPreferences(r.Context(), tenantID, recipientID, audience)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      prefs.ID,
		Version: prefs.Version,
		Data:    preferencesView(prefs),
	})
}

// --- draft handlers ---

func (h *CommunicationsHandler) handleCreateDraft(w http.ResponseWriter, r *http.Request) {
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

	var req createDraftRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	draft, err := h.svc.CreateDraft(r.Context(), tenantID, DraftParams{
		Audience:     req.Audience,
		RecipientID:  req.RecipientID,
		Source:       req.Source,
		TemplateKey:  req.TemplateKey,
		ConsentClass: req.ConsentClass,
		Channel:      req.Channel,
		Severity:     req.Severity,
		Language:     req.Language,
		Subject:      req.Subject,
		Body:         req.Body,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrRecipientRequired) || errors.Is(err, ErrInvalidAudience) || errors.Is(err, ErrInvalidSource) || errors.Is(err, ErrInvalidConsentClass) || errors.Is(err, ErrInvalidChannel) || errors.Is(err, ErrInvalidSeverity) || errors.Is(err, ErrTemplateKeyRequired) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrAudienceMismatch) {
			status = http.StatusUnprocessableEntity
			code = "AUDIENCE_MISMATCH"
		} else if errors.Is(err, ErrTemplateNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrDraftRequiresReview) {
			status = http.StatusUnprocessableEntity
			code = "REQUIRES_REVIEW"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, apiResource{
		ID:      draft.ID,
		Version: 1,
		Data:    draftView(draft),
	})
}

func (h *CommunicationsHandler) handleGetDraft(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")
	draft, err := h.svc.GetDraft(r.Context(), tenantID, draftID)
	if err != nil {
		if errors.Is(err, ErrDraftNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "draft not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      draft.ID,
		Version: 1,
		Data:    draftView(draft),
	})
}

func (h *CommunicationsHandler) handleReviewDraft(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req reviewDraftRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	draft, err := h.svc.ReviewDraft(r.Context(), tenantID, draftID, ReviewParams{
		ReviewerID: req.ReviewerID,
		Decision:   req.Decision,
		Reason:     req.Reason,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidReviewer) || errors.Is(err, ErrReviewDecisionRequired) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrReviewNotRequired) {
			status = http.StatusUnprocessableEntity
			code = "REVIEW_NOT_REQUIRED"
		} else if errors.Is(err, ErrDraftAlreadyReviewed) {
			status = http.StatusConflict
			code = "ALREADY_REVIEWED"
		} else if errors.Is(err, ErrDraftNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      draft.ID,
		Version: 1,
		Data:    draftView(draft),
	})
}

func (h *CommunicationsHandler) handleListReviews(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")
	reviews, err := h.svc.ListReviews(r.Context(), tenantID, draftID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if reviews == nil {
		reviews = []CommunicationReview{}
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

func (h *CommunicationsHandler) handleDeliver(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")
	delivery, err := h.svc.Deliver(r.Context(), tenantID, draftID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrDraftNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrDraftRequiresReview) || errors.Is(err, ErrDraftNotApproved) {
			status = http.StatusUnprocessableEntity
			code = "INVALID_STATE"
		} else if errors.Is(err, ErrDraftAlreadyDelivered) {
			status = http.StatusConflict
			code = "ALREADY_DELIVERED"
		} else if errors.Is(err, ErrConsentNotGranted) {
			status = http.StatusUnprocessableEntity
			code = "CONSENT_NOT_GRANTED"
		} else if errors.Is(err, ErrQuietHours) {
			status = http.StatusUnprocessableEntity
			code = "QUIET_HOURS"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      delivery.ID,
		Version: 1,
		Data:    deliveryView(delivery),
	})
}

func (h *CommunicationsHandler) handlePreviewDraft(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	draftID := r.PathValue("draft_id")
	preview, err := h.svc.PreviewDraft(r.Context(), tenantID, draftID)
	if err != nil {
		if errors.Is(err, ErrDraftNotFound) || errors.Is(err, ErrCrossTenantDenied) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "draft not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, previewView(preview))
}

// --- delivery handlers ---

func (h *CommunicationsHandler) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	recipientID := r.URL.Query().Get("recipient_id")
	deliveries, err := h.svc.ListDeliveries(r.Context(), tenantID, recipientID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if deliveries == nil {
		deliveries = []Delivery{}
	}

	resources := make([]apiResource, 0, len(deliveries))
	for i := range deliveries {
		resources = append(resources, apiResource{
			ID:      deliveries[i].ID,
			Version: 1,
			Data:    deliveryView(&deliveries[i]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}

func (h *CommunicationsHandler) handleRecordDeliveryResult(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	deliveryID := r.PathValue("delivery_id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req recordDeliveryResultRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	delivery, err := h.svc.RecordDeliveryResult(r.Context(), tenantID, deliveryID, req.Status, req.Failure)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidDeliveryStatus) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrDeliveryNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      delivery.ID,
		Version: 1,
		Data:    deliveryView(delivery),
	})
}

// --- secure link handlers ---

func (h *CommunicationsHandler) handleCreateSecureLink(w http.ResponseWriter, r *http.Request) {
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

	var req createSecureLinkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	expiresAt, err := parseRFC3339(req.ExpiresAt)
	if err != nil {
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid expires_at, use RFC3339")
		return
	}

	link, token, err := h.svc.CreateSecureLink(r.Context(), tenantID, SecureLinkParams{
		PropertyID:  req.PropertyID,
		Audience:    req.Audience,
		RecipientID: req.RecipientID,
		Purpose:     req.Purpose,
		ExpiresAt:   expiresAt,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidAudience) || errors.Is(err, ErrInvalidSecureLink) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           link.ID,
		"tenant_id":    link.TenantID,
		"property_id":  link.PropertyID,
		"audience":     link.Audience,
		"recipient_id": link.RecipientID,
		"purpose":      link.Purpose,
		"token_tail":   link.TokenTail,
		"token":        token,
		"expires_at":   link.ExpiresAt.Format(time.RFC3339),
		"status":       link.Status,
		"created_at":   link.CreatedAt.Format(time.RFC3339),
	})
}

func (h *CommunicationsHandler) handleRedeemSecureLink(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "cannot read request body")
		return
	}

	var req redeemSecureLinkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	link, err := h.svc.RedeemSecureLink(r.Context(), req.Token)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrLinkNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrLinkExpired) {
			status = http.StatusGone
			code = "LINK_EXPIRED"
		} else if errors.Is(err, ErrLinkAlreadyUsed) {
			status = http.StatusConflict
			code = "LINK_USED"
		} else if errors.Is(err, ErrLinkRevoked) {
			status = http.StatusGone
			code = "LINK_REVOKED"
		}
		writeAPIError(w, r, status, code, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           link.ID,
		"tenant_id":    link.TenantID,
		"property_id":  link.PropertyID,
		"audience":     link.Audience,
		"recipient_id": link.RecipientID,
		"purpose":      link.Purpose,
		"status":       link.Status,
		"used_at":      link.UsedAt,
	})
}

func (h *CommunicationsHandler) handleRevokeSecureLink(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	linkID := r.PathValue("link_id")
	link, err := h.svc.RevokeSecureLink(r.Context(), tenantID, linkID)
	if err != nil {
		if errors.Is(err, ErrLinkNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "link not found")
			return
		}
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, apiResource{
		ID:      link.ID,
		Version: 1,
		Data:    secureLinkView(link),
	})
}

func (h *CommunicationsHandler) handleListSecureLinks(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	links, err := h.svc.ListSecureLinks(r.Context(), tenantID, propertyID)
	if err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if links == nil {
		links = []SecureLink{}
	}

	resources := make([]apiResource, 0, len(links))
	for i := range links {
		resources = append(resources, apiResource{
			ID:      links[i].ID,
			Version: 1,
			Data:    secureLinkView(&links[i]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": resources,
		"total": len(resources),
	})
}
