//go:build acceptance

package app

import (
	"net/http"

	"comfort-curators-backend/internal/iam"
)

// registerAcceptanceFixtures wires the acceptance-only session-fixture
// route in when the binary is built with `-tags acceptance` (see
// scripts/run-phase, the only caller that sets it, and the Dockerfile's
// BUILD_TAGS arg). Its no-tag counterpart in fixtures_production.go is a
// no-op with the same signature -- a production build never has this file
// compiled in at all, so there is no runtime flag to misconfigure.
func registerAcceptanceFixtures(mux *http.ServeMux, svc *iam.IdentityService) {
	iam.RegisterTestFixtureRoutes(mux, svc)
}
