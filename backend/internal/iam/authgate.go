package iam

import (
	"net/http"

	"comfort-curators-backend/internal/platform/logging"
)

// publicPaths is the complete, explicit allowlist of routes reachable
// without an authenticated session. Anything not listed here requires a
// valid subject by default -- the inverse of the old architecture, where
// AuthMiddleware only ever enriched the request with a subject when one was
// present and every individual handler was responsible for remembering to
// reject an absent one. Several didn't (tenant creation, membership
// management, support-access grant mutation among them), so an
// unauthenticated caller could reach them freely.
//
// /auth/session/create is listed here because it is only ever registered
// in an acceptance-tagged build (see testfixtures.go); its own absence from
// a production binary is the real security boundary, not this allowlist.
//
// /v1/communications/secure-links/redeem is possession-based auth by
// design: the request's own one-time token (hashed at rest, rejects
// replay) is the credential, not a prior login session -- that is the
// whole point of a link a guest without an account can use.
var publicPaths = map[string]bool{
	"/health/live":                           true,
	"/health/ready":                          true,
	"/auth/otp/request":                      true,
	"/auth/otp/verify":                       true,
	"/auth/session/create":                   true,
	"/v1/communications/secure-links/redeem": true,
}

// RequireAuthByDefault rejects any request outside publicPaths that does
// not carry an authenticated subject. It must run after AuthMiddleware has
// had a chance to populate the subject from a valid session token, and
// after CorrelationID has established a request id, so its rejection
// responses can carry one like every other error response in this app.
func RequireAuthByDefault(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := SubjectFromContext(r.Context()); !ok {
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
