//go:build acceptance

// This file exists only to let the acceptance suite create authenticated
// sessions for arbitrary tenant/contact/role combinations without going
// through OTP + MFA. It is compiled in only when the api binary is built
// with `go build -tags acceptance` (see the Dockerfile's BUILD_TAGS arg and
// scripts/run-phase, which is the only caller that ever sets it).
//
// A normal `docker compose up` or plain `go build ./cmd/api` never sets this
// tag, so a production binary never has this file's code in it at all --
// the route it registers, and the everything-bypassing session it mints,
// simply do not exist to be reached, gated, or forgotten-about. That is
// deliberate: this bypasses all real authentication, and no runtime check
// (env var, config flag, role check) is trustworthy enough to gate
// something this sensitive -- only "the code isn't in the binary" is.
package iam

import (
	"context"
	"encoding/json"
	"net/http"

	"comfort-curators-backend/internal/platform/audit"
)

// RegisterTestFixtureRoutes registers the acceptance-only session-fixture
// route. Call only from an acceptance-tagged build; see package app's
// registerAcceptanceFixtures for the other half of that gate.
func RegisterTestFixtureRoutes(mux *http.ServeMux, svc *IdentityService) {
	mux.HandleFunc("POST /auth/session/create", handleSessionCreate(svc))
}

type sessionCreateResult struct {
	SessionToken string
	UserID       string
	Roles        []string
}

// createSessionForTest mints a session for any tenant/contact/role
// combination without OTP or MFA. Every requested role is validated
// (ValidRole), not just the first -- the original version validated only
// roles[0] but stored the entire caller-supplied array into the session,
// so a caller could smuggle in an unvalidated privileged role alongside a
// valid unprivileged one.
func createSessionForTest(s *IdentityService, ctx context.Context, tenantID, contact string, roles []string) (*sessionCreateResult, error) {
	if len(roles) == 0 {
		return nil, ErrRoleNotAllowed
	}
	for _, role := range roles {
		if !ValidRole(role) {
			return nil, ErrRoleNotAllowed
		}
	}

	user, err := s.EnsureUser(ctx, CreateUserParams{
		TenantID: tenantID,
		Contact:  contact,
		Role:     roles[0],
	})
	if err != nil {
		return nil, err
	}

	session, err := s.sessions.Create(ctx, user.ID, tenantID, roles)
	if err != nil {
		return nil, err
	}

	s.appendAudit(ctx, audit.AuditEvent{
		EventType:    audit.EventTypeAuth,
		TenantID:     tenantID,
		ActorID:      user.ID,
		Action:       "session.create",
		ResourceType: "session",
		ResourceID:   session.ID,
	})

	return &sessionCreateResult{
		SessionToken: session.ID,
		UserID:       user.ID,
		Roles:        roles,
	}, nil
}

func handleSessionCreate(svc *IdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID string   `json:"tenant_id"`
			Contact  string   `json:"contact"`
			Roles    []string `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "BAD_REQUEST",
				"message": "invalid request body",
			})
			return
		}

		result, err := createSessionForTest(svc, r.Context(), req.TenantID, req.Contact, req.Roles)
		if err != nil {
			status := http.StatusInternalServerError
			if err == ErrRoleNotAllowed {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, map[string]string{
				"code":    "ERROR",
				"message": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session_token": result.SessionToken,
			"user_id":       result.UserID,
			"roles":         result.Roles,
		})
	}
}
