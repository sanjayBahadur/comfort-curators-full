package iam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"comfort-curators-backend/internal/platform/security"
)

func RegisterAuthRoutes(mux *http.ServeMux, svc *IdentityService) {
	mux.HandleFunc("POST /auth/otp/request", handleRequestOTP(svc))
	mux.HandleFunc("POST /auth/otp/verify", handleVerifyOTP(svc))
	mux.HandleFunc("GET /auth/session", handleGetSession(svc))
	mux.HandleFunc("POST /auth/session/revoke", handleRevokeSession(svc))
	mux.HandleFunc("POST /auth/mfa/enroll", handleEnrollMFA(svc))
	mux.HandleFunc("POST /auth/mfa/confirm", handleConfirmMFA(svc))
	mux.HandleFunc("POST /auth/mfa/verify", handleVerifyMFA(svc))
	mux.HandleFunc("POST /auth/mfa/check", handleCheckMFA(svc))
}

func RegisterTenancyRoutes(mux *http.ServeMux, tenancy *TenancyService) {
	mux.HandleFunc("POST /tenants", handleCreateTenant(tenancy))
	mux.HandleFunc("GET /tenants/{tenant_id}", handleGetTenant(tenancy))
	mux.HandleFunc("POST /tenants/{tenant_id}/memberships", handleAddMembership(tenancy))
	mux.HandleFunc("DELETE /tenants/{tenant_id}/memberships/{user_id}", handleRemoveMembership(tenancy))
	mux.HandleFunc("POST /tenants/{tenant_id}/support-access-grants", handleCreateSupportAccessGrant(tenancy))
	mux.HandleFunc("DELETE /support-access-grants/{grant_id}", handleRevokeSupportAccessGrant(tenancy))
	mux.HandleFunc("POST /access/check", handleCheckAccess(tenancy))
}

func handleRequestOTP(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID string `json:"tenant_id"`
			Contact  string `json:"contact"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		otp, err := svc.RequestOTP(r.Context(), RequestOTPParams{
			TenantID: req.TenantID,
			Contact:  req.Contact,
			Role:     req.Role,
		})
		if err != nil {
			status := http.StatusInternalServerError
			switch err {
			case ErrRoleNotAllowed:
				status = http.StatusBadRequest
			case ErrRateLimited:
				status = http.StatusTooManyRequests
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"otp_expires_at": otp.ExpiresAt,
		})
	}
}

func handleVerifyOTP(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID string `json:"tenant_id"`
			Contact  string `json:"contact"`
			Token    string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		session, err := svc.VerifyOTP(r.Context(), VerifyOTPParams{
			TenantID: req.TenantID,
			Contact:  req.Contact,
			Token:    req.Token,
		})
		if err != nil {
			status := http.StatusUnauthorized
			switch err {
			case ErrOTPNotFound, ErrOTPExpired, ErrOTPConsumed, ErrOTPInvalid:
				status = http.StatusUnauthorized
			case ErrUserNotFound:
				status = http.StatusUnauthorized
			default:
				status = http.StatusInternalServerError
			}
			writeJSON(w, status, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session_token": session.ID,
			"session":       session,
		})
	}
}

func handleGetSession(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": "authentication required",
			})
			return
		}

		token := extractBearerToken(r)
		session, err := svc.GetSession(r.Context(), token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": "invalid session",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session": session,
		})
		_ = subject
	}
}

func handleRevokeSession(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SessionToken string `json:"session_token"`
			Reason       string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		subject, _ := SubjectFromContext(r.Context())
		revokedBy := "system"
		if subject.ActorID != "" {
			revokedBy = subject.ActorID
		}

		if err := svc.RevokeSession(r.Context(), req.SessionToken, req.Reason, revokedBy); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "revoked",
		})
	}
}

func handleEnrollMFA(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		if err := requireSelfSubject(r, req.UserID); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": err.Error(),
			})
			return
		}

		result, err := svc.EnrollMFA(r.Context(), req.UserID)
		if err != nil {
			status := http.StatusInternalServerError
			if err == ErrMFAAlreadyEnrolled {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"mfa_method":       result.Method,
			"secret":           result.Secret,
			"provisioning_uri": result.ProvisioningURI,
		})
	}
}

func handleConfirmMFA(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID string `json:"user_id"`
			Code   string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		if err := requireSelfSubject(r, req.UserID); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": err.Error(),
			})
			return
		}

		if err := svc.ConfirmMFA(r.Context(), ConfirmMFAParams{
			UserID: req.UserID,
			Code:   req.Code,
		}); err != nil {
			status := http.StatusBadRequest
			switch err {
			case ErrMFAInvalid, ErrMFAEnrollmentNotFound:
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "confirmed",
		})
	}
}

func handleVerifyMFA(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID string `json:"user_id"`
			Code   string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		if err := requireSelfSubject(r, req.UserID); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": err.Error(),
			})
			return
		}

		if err := svc.VerifyMFA(r.Context(), VerifyMFAParams{
			UserID: req.UserID,
			Code:   req.Code,
		}); err != nil {
			status := http.StatusBadRequest
			switch err {
			case ErrMFAInvalid, ErrMFAUnverified:
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "verified",
		})
	}
}

func requireSelfSubject(r *http.Request, userID string) error {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		return fmt.Errorf("authentication required")
	}
	if subject.ActorID != userID {
		return fmt.Errorf("unauthorized user")
	}
	return nil
}

func handleCheckMFA(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": "authentication required",
			})
			return
		}

		var req struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		err := svc.CheckMFARequired(r.Context(), subject, security.Action(req.Action))
		required := false
		if err != nil {
			required = err == security.ErrMFARequired
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"mfa_required": required,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleCreateTenant(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		tenant, err := tenancy.CreateTenant(r.Context(), CreateTenantParams{Name: req.Name})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"tenant": tenant})
	}
}

func handleGetTenant(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenant_id")

		subject, ok := SubjectFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": "authentication required",
			})
			return
		}

		tenant, err := tenancy.GetTenant(r.Context(), tenantID)
		if err != nil {
			if err == ErrTenantNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"code":    "NOT_FOUND",
					"message": "tenant not found",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		if err := tenancy.RequireResourceAccess(r.Context(), tenant.ID, "tenant", tenant.ID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"code":    "NOT_FOUND",
				"message": "tenant not found",
			})
			return
		}
		_ = subject

		writeJSON(w, http.StatusOK, map[string]any{"tenant": tenant})
	}
}

func handleAddMembership(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenant_id")

		var req struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		membership, err := tenancy.AddMembership(r.Context(), tenantID, req.UserID, req.Role)
		if err != nil {
			status := http.StatusInternalServerError
			if err == ErrRoleNotAllowed || err == ErrTenantNotFound {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"membership": membership})
	}
}

func handleRemoveMembership(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenant_id")
		userID := r.PathValue("user_id")

		if err := tenancy.RemoveMembership(r.Context(), tenantID, userID); err != nil {
			status := http.StatusInternalServerError
			if err == ErrMembershipNotFound {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	}
}

func handleCreateSupportAccessGrant(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenant_id")

		var req struct {
			GrantedByUserID string `json:"granted_by_user_id"`
			GrantedToUserID string `json:"granted_to_user_id"`
			Reason          string `json:"reason"`
			Scope           string `json:"scope"`
			TTLSeconds      int    `json:"ttl_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		ttl := 1 * time.Hour
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}

		grant, err := tenancy.CreateSupportAccessGrant(r.Context(), CreateSupportAccessGrantParams{
			TenantID:        tenantID,
			GrantedByUserID: req.GrantedByUserID,
			GrantedToUserID: req.GrantedToUserID,
			Reason:          req.Reason,
			Scope:           req.Scope,
			TTL:             ttl,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"support_access_grant": grant})
	}
}

func handleRevokeSupportAccessGrant(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		grantID := r.PathValue("grant_id")

		subject, _ := SubjectFromContext(r.Context())
		revokedBy := "system"
		if subject.ActorID != "" {
			revokedBy = subject.ActorID
		}

		if err := tenancy.RevokeSupportAccessGrant(r.Context(), grantID, revokedBy); err != nil {
			status := http.StatusInternalServerError
			if err == ErrSupportAccessNotFound {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	}
}

func handleCheckAccess(tenancy *TenancyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    "UNAUTHORIZED",
				"message": "authentication required",
			})
			return
		}

		var req struct {
			TenantID string `json:"tenant_id"`
			GrantID  string `json:"grant_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		allowed := false
		if subject.TenantID == req.TenantID {
			allowed = true
		} else if err := tenancy.ValidateSupportAccess(r.Context(), subject.ActorID, req.TenantID); err == nil {
			allowed = true
		}

		if !allowed {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"code":    "FORBIDDEN",
				"message": "cross-tenant access denied",
				"allowed": false,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"allowed": true,
		})
	}
}
