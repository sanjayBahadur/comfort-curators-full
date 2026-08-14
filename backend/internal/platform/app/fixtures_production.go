//go:build !acceptance

package app

import (
	"net/http"

	"comfort-curators-backend/internal/iam"
)

// registerAcceptanceFixtures is a no-op in every build except one tagged
// `acceptance` (see fixtures_acceptance.go). This is the default: a plain
// `go build ./cmd/api` or a normal `docker compose up` never sets that tag,
// so the session-bypass route this would otherwise register is compiled
// into production binaries at all.
func registerAcceptanceFixtures(mux *http.ServeMux, svc *iam.IdentityService) {}
