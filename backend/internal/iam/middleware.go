package iam

import (
	"context"
	"net/http"
	"strings"

	"comfort-curators-backend/internal/platform/logging"
	"comfort-curators-backend/internal/platform/security"
)

type contextKey int

const (
	subjectKey contextKey = iota
)

func WithSubject(ctx context.Context, subject security.Subject) context.Context {
	return context.WithValue(ctx, subjectKey, subject)
}

func SubjectFromContext(ctx context.Context) (security.Subject, bool) {
	subject, ok := ctx.Value(subjectKey).(security.Subject)
	return subject, ok
}

func AuthMiddleware(sessionStore *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			session, err := sessionStore.Get(r.Context(), token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			subject := security.Subject{
				ActorID:  session.ActorID,
				TenantID: session.TenantID,
				Roles:    session.Roles,
			}

			ctx := WithSubject(r.Context(), subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := SubjectFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"request_id": logging.RequestIDFromCtx(r.Context()),
				"code":       "UNAUTHORIZED",
				"message":    "authentication required",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, ok := SubjectFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"request_id": logging.RequestIDFromCtx(r.Context()),
					"code":       "UNAUTHORIZED",
					"message":    "authentication required",
				})
				return
			}

			for _, required := range roles {
				for _, userRole := range subject.Roles {
					if userRole == required {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			writeJSON(w, http.StatusForbidden, map[string]any{
				"request_id": logging.RequestIDFromCtx(r.Context()),
				"code":       "FORBIDDEN",
				"message":    "insufficient permissions",
			})
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
