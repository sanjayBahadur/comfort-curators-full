package main

import (
	"fmt"
	"os"
	"testing"

	"comfort-curators-backend/internal/platform/testdb"
)

// liveAcceptanceEnabled reports whether the operator has explicitly asked for
// the live-stack acceptance probes.
//
// These are not unit tests. probeCCREL001CapacityTarget runs the NFR-001
// capacity scenario, which bulk-loads 50,000 tickets and 100,000 inventory
// movements into the database the running stack is using, and it needs the
// stack's fully provisioned schema — an empty per-package test database cannot
// satisfy it. So it cannot simply be pointed somewhere safe: it has to be
// deliberately invoked against a stack whose data the operator is willing to
// bury in load-test rows.
//
// Left ungated it ran on every `go test ./...` and silently added 50,000
// tickets to the application database each time. P1-02 did not catch it
// because the offending default lives in probes.go, an ordinary .go file, and
// that migration only rewrote *_test.go.
//
//	CC_ACCEPTANCE_LIVE=1 CC_DB_NAME=<target> go test ./tests/acceptance/
func liveAcceptanceEnabled() bool {
	return os.Getenv("CC_ACCEPTANCE_LIVE") == "1"
}

func TestMain(m *testing.M) {
	if liveAcceptanceEnabled() {
		// Opted in. Require an explicit target rather than letting
		// dbConnString() fall back to the application database.
		name := os.Getenv("CC_DB_NAME")
		if name == "" {
			fmt.Fprintln(os.Stderr,
				"CC_ACCEPTANCE_LIVE=1 requires CC_DB_NAME naming the database to load with\n"+
					"capacity data. These probes write 150,000 rows; they will not guess a target.")
			os.Exit(1)
		}
		if err := testdb.ValidateName(name); err != nil {
			fmt.Fprintf(os.Stderr,
				"%v\n\nThe capacity probe bulk-loads 150,000 rows. Point it at a disposable\n"+
					"database, not one you care about.\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
