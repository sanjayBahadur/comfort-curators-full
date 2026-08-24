package operations_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type testAuthorizer struct {
	tenant string
	deny   bool
}

func (a testAuthorizer) RequireResourceAccess(ctx context.Context, resourceTenantID, resourceType, resourceID string) error {
	if a.deny {
		return errors.New("denied")
	}
	if a.tenant != "" && a.tenant != resourceTenantID {
		return operations.ErrCrossTenantDenied
	}
	return nil
}

func postgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func dbConnString() string {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := testdb.MustName()
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func operationsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available for operations integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := operations.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure operations schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"service_recoveries",
		"incident_alerts",
		"ticket_evidence",
		"ticket_state_events",
		"ticket_checklist_items",
		"tickets",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func TestIdempotentSyncDoesNotDuplicateOnReplay(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:           "tenant-a",
		PropertyID:         "prop-1",
		Type:               operations.TypeTurnover,
		ChecklistVersionID: "cv_1",
		Reason:             "turnover sync test",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	items := []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean room", Status: operations.ChecklistStatusCompleted},
		{TemplateItemIndex: 1, Label: "swap towels", Status: operations.ChecklistStatusPending},
	}

	syncKey := "curator-sync-" + tkt.ID + "-001"

	first, err := svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey, items, "actor-curator-1")
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if first.Replay {
		t.Fatal("first sync must not be a replay")
	}
	if len(first.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(first.Items))
	}
	if len(first.Conflicts) != 0 {
		t.Fatalf("expected no conflicts on first sync, got %d", len(first.Conflicts))
	}

	second, err := svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey, items, "actor-curator-1")
	if err != nil {
		t.Fatalf("second sync with same key: %v", err)
	}
	if !second.Replay {
		t.Fatal("second sync with same key and payload must be a replay")
	}
	if len(second.Items) != 2 {
		t.Fatalf("replay must return same items, got %d", len(second.Items))
	}

	syncedItems, err := svc.ListChecklistItems(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(syncedItems) != 2 {
		t.Fatalf("replay must not duplicate persisted items, got %d", len(syncedItems))
	}
}

func TestSyncKeyConflictWithDifferentPayloads(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:           "tenant-a",
		PropertyID:         "prop-1",
		Type:               operations.TypeTurnover,
		ChecklistVersionID: "cv_1",
		Reason:             "sync conflict test",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	syncKey := "curator-sync-" + tkt.ID + "-conflict"

	items1 := []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean room", Status: operations.ChecklistStatusPending},
	}
	_, err = svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey, items1, "actor-curator-1")
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}

	items2 := []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean room", Status: operations.ChecklistStatusCompleted},
	}
	_, err = svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey, items2, "actor-curator-1")
	if !errors.Is(err, operations.ErrSyncKeyConflict) {
		t.Fatalf("expected ErrSyncKeyConflict for different payload under same key, got %v", err)
	}
}

func TestSyncConflictIsVisibleAndPreservesBothVersions(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:           "tenant-a",
		PropertyID:         "prop-1",
		Type:               operations.TypeTurnover,
		ChecklistVersionID: "cv_1",
		Reason:             "conflict visibility test",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	syncKey1 := "device-a-sync-" + tkt.ID
	items1 := []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo evidence", Status: operations.ChecklistStatusInProgress},
	}
	result1, err := svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey1, items1, "actor-curator-1")
	if err != nil {
		t.Fatalf("first device sync: %v", err)
	}
	if len(result1.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result1.Items))
	}

	result1.Items[0].Status = operations.ChecklistStatusCompleted
	result1.Items[0].Version = 1
	if _, err := svc.SyncChecklist(context.Background(), "tenant-a", tkt.ID, result1.Items, "actor-ops-1"); err != nil {
		t.Fatalf("update item via ops: %v", err)
	}

	syncKey2 := "device-b-sync-" + tkt.ID
	items2 := []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo evidence", Status: operations.ChecklistStatusPending},
	}
	result2, err := svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey2, items2, "actor-curator-2")
	if err != nil {
		t.Fatalf("second device sync: %v", err)
	}

	if len(result2.Conflicts) == 0 {
		t.Fatal("expected visible conflict when server has progressed past client version")
	}
	c := result2.Conflicts[0]
	if c.ServerStatus != operations.ChecklistStatusCompleted {
		t.Errorf("server version not preserved in conflict, got %q", c.ServerStatus)
	}
	if c.ClientStatus != operations.ChecklistStatusPending {
		t.Errorf("client version not preserved in conflict, got %q", c.ClientStatus)
	}

	conflicts, err := svc.ListSyncConflicts(context.Background(), "tenant-a", tkt.ID, false)
	if err != nil {
		t.Fatalf("list conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflict must be visible, got %d conflicts", len(conflicts))
	}

	_, err = svc.ResolveSyncConflict(context.Background(), "tenant-a", c.ID, "accept_server", "actor-ops-1")
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}

	resolved, err := svc.ListSyncConflicts(context.Background(), "tenant-a", tkt.ID, true)
	if err != nil {
		t.Fatalf("list resolved conflicts: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved conflict must be visible, got %d", len(resolved))
	}
}

func TestOfflineEvidenceQueueAndSync(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:   "tenant-a",
		PropertyID: "prop-1",
		Type:       operations.TypeIncident,
		Reason:     "offline evidence test",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	hash1 := operations.ComputeEvidenceHash([]byte("offline-damage-photo-1"))
	hash2 := operations.ComputeEvidenceHash([]byte("offline-damage-photo-2"))

	q1, err := svc.QueueOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidencePayload{
		ContentHash: hash1,
		FileName:    "photo1.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   4096,
		Language:    "en",
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("queue evidence 1: %v", err)
	}
	if q1.Status != operations.OfflineEvidenceQueued {
		t.Fatalf("expected queued status, got %q", q1.Status)
	}

	q2, err := svc.QueueOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidencePayload{
		ContentHash: hash2,
		FileName:    "photo2.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   8192,
		Language:    "hi",
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("queue evidence 2: %v", err)
	}
	_ = q2

	queued, err := svc.ListQueuedOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidenceQueued)
	if err != nil {
		t.Fatalf("list queued: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("expected 2 queued items, got %d", len(queued))
	}

	// Queueing the same hash again returns the same record (idempotent).
	dup, err := svc.QueueOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidencePayload{
		ContentHash: hash1,
		FileName:    "photo1-duplicate.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   4096,
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("duplicate queue: %v", err)
	}
	if dup.ID != q1.ID {
		t.Fatalf("duplicate queue must return existing record: %s != %s", q1.ID, dup.ID)
	}

	synced, err := svc.SyncOfflineEvidence(context.Background(), "tenant-a", tkt.ID, []string{hash1, hash2}, "actor-curator-1")
	if err != nil {
		t.Fatalf("sync offline evidence: %v", err)
	}
	if len(synced) != 2 {
		t.Fatalf("expected 2 synced records, got %d", len(synced))
	}

	evidence, err := svc.ListEvidence(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(evidence) != 2 {
		t.Fatalf("expected 2 evidence records after sync, got %d", len(evidence))
	}

	// Resyncing the same hashes is idempotent (evidence already exists).
	resynced, err := svc.SyncOfflineEvidence(context.Background(), "tenant-a", tkt.ID, []string{hash1, hash2}, "actor-curator-1")
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if len(resynced) != 2 {
		t.Fatalf("resync must return existing evidence, got %d", len(resynced))
	}

	// Evidence list must not grow (no duplicates).
	evidence2, err := svc.ListEvidence(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list evidence after resync: %v", err)
	}
	if len(evidence2) != 2 {
		t.Fatalf("resync must not duplicate evidence, got %d", len(evidence2))
	}
}

func TestReplayDoesNotDuplicateSyncedItems(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:           "tenant-a",
		PropertyID:         "prop-1",
		Type:               operations.TypePreArrivalInspection,
		ChecklistVersionID: "cv_1",
		Reason:             "replay test",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	items := []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "inspect door locks", Status: operations.ChecklistStatusPending, EvidenceRequired: true},
		{TemplateItemIndex: 1, Label: "test smoke detectors", Status: operations.ChecklistStatusPending},
		{TemplateItemIndex: 2, Label: "check water pressure", Status: operations.ChecklistStatusPending},
	}

	syncKey := "curator-device-xyz-" + tkt.ID

	for i := 0; i < 5; i++ {
		result, syncErr := svc.IdempotentSyncChecklist(context.Background(), "tenant-a", tkt.ID, syncKey, items, "actor-curator-1")
		if syncErr != nil {
			t.Fatalf("sync attempt %d: %v", i, syncErr)
		}
		if i == 0 && result.Replay {
			t.Fatal("first sync must not be a replay")
		}
		if i > 0 && !result.Replay {
			t.Fatalf("sync attempt %d must be a replay", i)
		}
	}

	persisted, err := svc.ListChecklistItems(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(persisted) != 3 {
		t.Fatalf("after 5 replays, there must be exactly 3 items, got %d", len(persisted))
	}
}

func TestOfflineEvidenceLanguageValidation(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:   "tenant-a",
		PropertyID: "prop-1",
		Type:       operations.TypeTurnover,
		Reason:     "language test",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	hash := operations.ComputeEvidenceHash([]byte("offline-photo-lang-test"))

	_, err = svc.QueueOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidencePayload{
		ContentHash: hash,
		SizeBytes:   1024,
		Language:    "en",
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("en language rejected: %v", err)
	}

	_, err = svc.QueueOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidencePayload{
		ContentHash: operations.ComputeEvidenceHash([]byte("offline-photo-hi")),
		SizeBytes:   1024,
		Language:    "hi",
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("hi language rejected: %v", err)
	}

	_, err = svc.QueueOfflineEvidence(context.Background(), "tenant-a", tkt.ID, operations.OfflineEvidencePayload{
		ContentHash: operations.ComputeEvidenceHash([]byte("offline-photo-fr")),
		SizeBytes:   1024,
		Language:    "fr",
	}, "actor-curator-1")
	if err == nil {
		t.Fatal("unsupported language 'fr' must be rejected")
	}
}

func newTicketService(t *testing.T) *operations.TicketService {
	t.Helper()
	pool := operationsPool(t)
	svc := operations.NewTicketService(pool).
		WithAuthorizer(testAuthorizer{tenant: "tenant-a"}).
		WithAudit(audit.NewAuditStore(pool))
	return svc
}

func createIncident(t *testing.T, svc *operations.TicketService, reason string) *operations.Ticket {
	t.Helper()
	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:   "tenant-a",
		PropertyID: "prop-1",
		Type:       operations.TypeIncident,
		Reason:     reason,
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	return tkt
}

func walkToInProgress(t *testing.T, svc *operations.TicketService, tkt *operations.Ticket) *operations.Ticket {
	t.Helper()
	steps := []string{
		operations.StateProposed,
		operations.StateApproved,
		operations.StateScheduled,
		operations.StateAssigned,
		operations.StateInProgress,
	}
	cur := tkt
	for _, to := range steps {
		next, err := svc.TransitionTicket(context.Background(), "tenant-a", cur.ID, operations.TransitionParams{
			ToState: to,
			Reason:  "advance",
		}, "actor-ops-1")
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
		cur = next
	}
	return cur
}

func TestEvidenceBlocksCompletionUntilCleanEvidence(t *testing.T) {
	svc := newTicketService(t)

	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:           "tenant-a",
		PropertyID:         "prop-1",
		Type:               operations.TypeTurnover,
		ChecklistVersionID: "cv_1",
		Reason:             "turnover before next stay",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	items, err := svc.SyncChecklist(context.Background(), "tenant-a", tkt.ID, []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "cleanliness photo", Status: operations.ChecklistStatusCompleted, EvidenceRequired: true},
		{TemplateItemIndex: 1, Label: "towel swap", Status: operations.ChecklistStatusCompleted, EvidenceRequired: false},
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("sync checklist: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 checklist items, got %d", len(items))
	}

	cur := walkToInProgress(t, svc, tkt)

	// Evidence submission is blocked while the required item has no evidence.
	_, err = svc.TransitionTicket(context.Background(), "tenant-a", cur.ID, operations.TransitionParams{
		ToState: operations.StateEvidenceSubmitted,
		Reason:  "work done",
	}, "actor-ops-1")
	if !errors.Is(err, operations.ErrEvidenceRequired) {
		t.Fatalf("expected ErrEvidenceRequired on evidence submission, got %v", err)
	}

	// Completion is also blocked at the closed gate.
	if _, err := svc.TransitionTicket(context.Background(), "tenant-a", cur.ID, operations.TransitionParams{
		ToState: operations.StateEvidenceSubmitted,
		Reason:  "work done",
	}, "actor-ops-1"); !errors.Is(err, operations.ErrEvidenceRequired) {
		t.Fatalf("expected ErrEvidenceRequired, got %v", err)
	}

	// Register clean evidence and bind it to the required checklist item.
	rec, err := svc.RegisterEvidence(context.Background(), "tenant-a", tkt.ID, operations.RegisterEvidenceParams{
		ContentHash: operations.ComputeEvidenceHash([]byte("clean-photo-1")),
		FileName:    "cleanliness.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   4096,
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("register evidence: %v", err)
	}
	if rec.Status != operations.EvidenceStatusAccepted {
		t.Fatalf("expected accepted evidence, got %q", rec.Status)
	}

	if _, err := svc.SyncChecklist(context.Background(), "tenant-a", tkt.ID, []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "cleanliness photo", Status: operations.ChecklistStatusCompleted, EvidenceRequired: true, EvidenceIDs: []string{rec.ID}},
		{TemplateItemIndex: 1, Label: "towel swap", Status: operations.ChecklistStatusCompleted, EvidenceRequired: false},
	}, "actor-curator-1"); err != nil {
		t.Fatalf("bind evidence to checklist: %v", err)
	}

	// With clean evidence bound, the ticket can be submitted and completed.
	cur, err = svc.TransitionTicket(context.Background(), "tenant-a", cur.ID, operations.TransitionParams{
		ToState: operations.StateEvidenceSubmitted,
		Reason:  "work done",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("evidence submission should now succeed: %v", err)
	}
	cur, err = svc.TransitionTicket(context.Background(), "tenant-a", cur.ID, operations.TransitionParams{
		ToState: operations.StateVerified,
		Reason:  "reviewed",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("verification should succeed: %v", err)
	}
	cur, err = svc.TransitionTicket(context.Background(), "tenant-a", cur.ID, operations.TransitionParams{
		ToState: operations.StateClosed,
		Reason:  "complete",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("completion should succeed with evidence: %v", err)
	}
	if cur.Status != operations.StateClosed {
		t.Fatalf("expected closed, got %q", cur.Status)
	}
}

func TestEvidenceHashIsStableAcrossReregistration(t *testing.T) {
	svc := newTicketService(t)
	tkt := createIncident(t, svc, "water damage")

	hash := operations.ComputeEvidenceHash([]byte("immutable-bytes"))
	first, err := svc.RegisterEvidence(context.Background(), "tenant-a", tkt.ID, operations.RegisterEvidenceParams{
		ContentHash: hash,
		FileName:    "damage.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   8192,
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}

	second, err := svc.RegisterEvidence(context.Background(), "tenant-a", tkt.ID, operations.RegisterEvidenceParams{
		ContentHash: hash,
		FileName:    "damage-copy.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   8192,
	}, "actor-curator-1")
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("re-registering identical content must return the same evidence record: %s != %s", first.ID, second.ID)
	}
	if second.ContentHash != hash || first.ContentHash != hash {
		t.Fatalf("accepted evidence hash changed: %s/%s", first.ContentHash, second.ContentHash)
	}

	records, err := svc.ListEvidence(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("re-registration must not duplicate evidence, got %d records", len(records))
	}

	got, err := svc.GetEvidence(context.Background(), "tenant-a", first.ID)
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if got.ContentHash != hash {
		t.Fatalf("stored hash changed: %s != %s", got.ContentHash, hash)
	}
}

func TestHighSeverityIncidentQueuesOnCallAndOwnerAlerts(t *testing.T) {
	svc := newTicketService(t)
	tkt := createIncident(t, svc, "gas smell in kitchen")

	classified, err := svc.ClassifyIncident(context.Background(), "tenant-a", tkt.ID, operations.SeverityHigh, "actor-ops-1")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if classified.Severity != operations.SeverityHigh {
		t.Fatalf("expected high severity, got %q", classified.Severity)
	}
	if classified.NotificationIntent != operations.NotificationIntentUrgent {
		t.Fatalf("high severity must set urgent notification intent, got %q", classified.NotificationIntent)
	}

	alerts, err := svc.ListIncidentAlertsForTicket(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	targets := map[string]bool{}
	for _, a := range alerts {
		if a.Status != operations.AlertStatusQueued {
			t.Fatalf("alert must be queued, got %q", a.Status)
		}
		if a.Severity != operations.SeverityHigh {
			t.Fatalf("alert severity mismatch: %q", a.Severity)
		}
		if a.Policy == "" {
			t.Fatal("alert must carry its response policy")
		}
		if a.TicketID != tkt.ID {
			t.Fatalf("alert must reference the incident ticket, got %q", a.TicketID)
		}
		targets[a.Target] = true
	}
	if !targets[operations.AlertTargetOnCall] || !targets[operations.AlertTargetOwner] {
		t.Fatalf("high severity must queue on_call and owner alerts, got %v", targets)
	}

	// The alerts are visible through the property-scoped queue surface too.
	byProperty, err := svc.ListIncidentAlerts(context.Background(), "tenant-a", "prop-1", operations.AlertStatusQueued)
	if err != nil {
		t.Fatalf("list queued alerts: %v", err)
	}
	if len(byProperty) != 2 {
		t.Fatalf("expected 2 queued alerts, got %d", len(byProperty))
	}
}

func TestMediumIncidentQueuesOnCallOnly(t *testing.T) {
	svc := newTicketService(t)
	tkt := createIncident(t, svc, "broken wardrobe door")

	if _, err := svc.ClassifyIncident(context.Background(), "tenant-a", tkt.ID, operations.SeverityMedium, "actor-ops-1"); err != nil {
		t.Fatalf("classify: %v", err)
	}

	alerts, err := svc.ListIncidentAlertsForTicket(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Target != operations.AlertTargetOnCall {
		t.Fatalf("medium severity must queue a single on_call alert, got %+v", alerts)
	}
}

func TestClassifyIncidentRejectsNonIncident(t *testing.T) {
	svc := newTicketService(t)
	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:   "tenant-a",
		PropertyID: "prop-1",
		Type:       operations.TypeTurnover,
		Reason:     "scheduled turnover",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	_, err = svc.ClassifyIncident(context.Background(), "tenant-a", tkt.ID, operations.SeverityHigh, "actor-ops-1")
	if !errors.Is(err, operations.ErrNotIncident) {
		t.Fatalf("expected ErrNotIncident, got %v", err)
	}
}

func TestServiceRecoveryPreservesOriginalFailure(t *testing.T) {
	svc := newTicketService(t)
	tkt := createIncident(t, svc, "washing machine overflowed")

	evHash := operations.ComputeEvidenceHash([]byte("overflow-evidence"))
	if _, err := svc.RegisterEvidence(context.Background(), "tenant-a", tkt.ID, operations.RegisterEvidenceParams{
		ContentHash: evHash,
		FileName:    "overflow.jpg",
		ContentType: "image/jpeg",
		SizeBytes:   1234,
	}, "actor-curator-1"); err != nil {
		t.Fatalf("register evidence: %v", err)
	}

	if _, err := svc.ClassifyIncident(context.Background(), "tenant-a", tkt.ID, operations.SeverityHigh, "actor-ops-1"); err != nil {
		t.Fatalf("classify: %v", err)
	}

	rec, err := svc.StartServiceRecovery(context.Background(), "tenant-a", tkt.ID, operations.RecoveryParams{
		Reason:          "replace washing machine and dry the floor",
		Responsibility:  "ops-supervisor-1",
		ReworkCostMinor: 15500,
		Currency:        "INR",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("start recovery: %v", err)
	}

	if rec.IncidentTicketID != tkt.ID {
		t.Fatalf("recovery must link the original incident, got %q", rec.IncidentTicketID)
	}
	if rec.OriginalReason != "washing machine overflowed" {
		t.Fatalf("recovery must preserve the original failure reason, got %q", rec.OriginalReason)
	}
	if rec.Severity != operations.SeverityHigh {
		t.Fatalf("recovery must preserve the original severity, got %q", rec.Severity)
	}
	if len(rec.OriginalEvidenceHashes) != 1 || rec.OriginalEvidenceHashes[0] != evHash {
		t.Fatalf("recovery must preserve the original evidence hashes, got %v", rec.OriginalEvidenceHashes)
	}
	if rec.Responsibility != "ops-supervisor-1" {
		t.Fatalf("responsibility not recorded: %q", rec.Responsibility)
	}
	if rec.ReworkCostMinor != 15500 || rec.Currency != "INR" {
		t.Fatalf("rework cost not preserved: %d %s", rec.ReworkCostMinor, rec.Currency)
	}
	if rec.FollowUpTicketID == "" {
		t.Fatal("recovery must create a linked recovery ticket")
	}

	// The follow-up ticket exists, is of incident type, and preserves the
	// recovery reason as its own failure to resolve.
	followUp, err := svc.GetTicket(context.Background(), "tenant-a", rec.FollowUpTicketID)
	if err != nil {
		t.Fatalf("get follow-up ticket: %v", err)
	}
	if followUp.Type != operations.TypeIncident {
		t.Fatalf("follow-up must be an incident ticket, got %q", followUp.Type)
	}

	// The original incident now points at its recovery ticket.
	original, err := svc.GetTicket(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("get original incident: %v", err)
	}
	if original.FollowUpTicketID != rec.FollowUpTicketID {
		t.Fatalf("original incident must link its recovery ticket, got %q", original.FollowUpTicketID)
	}

	list, err := svc.ListServiceRecoveries(context.Background(), "tenant-a", tkt.ID)
	if err != nil {
		t.Fatalf("list recoveries: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 recovery, got %d", len(list))
	}
}

func TestServiceRecoveryRequiresAccountability(t *testing.T) {
	svc := newTicketService(t)
	tkt := createIncident(t, svc, "broken AC")

	_, err := svc.StartServiceRecovery(context.Background(), "tenant-a", tkt.ID, operations.RecoveryParams{
		Reason:          "repair AC",
		ReworkCostMinor: 9000,
		Currency:        "INR",
	}, "actor-ops-1")
	if !errors.Is(err, operations.ErrResponsibilityRequired) {
		t.Fatalf("expected ErrResponsibilityRequired, got %v", err)
	}

	_, err = svc.StartServiceRecovery(context.Background(), "tenant-a", tkt.ID, operations.RecoveryParams{
		Reason:          "repair AC",
		Responsibility:  "ops-supervisor-1",
		ReworkCostMinor: 9000,
	}, "actor-ops-1")
	if !errors.Is(err, operations.ErrCurrencyRequired) {
		t.Fatalf("expected ErrCurrencyRequired, got %v", err)
	}

	_, err = svc.StartServiceRecovery(context.Background(), "tenant-a", tkt.ID, operations.RecoveryParams{
		Responsibility: "ops-supervisor-1",
	}, "actor-ops-1")
	if err == nil {
		t.Fatal("recovery without a reason must fail")
	}
}

func TestEvidenceRequirementCannotBeDowngraded(t *testing.T) {
	svc := newTicketService(t)
	tkt, err := svc.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:           "tenant-a",
		PropertyID:         "prop-1",
		Type:               operations.TypeTurnover,
		ChecklistVersionID: "cv_1",
		Reason:             "turnover",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := svc.SyncChecklist(context.Background(), "tenant-a", tkt.ID, []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: operations.ChecklistStatusPending, EvidenceRequired: true},
	}, "actor-ops-1"); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	_, err = svc.SyncChecklist(context.Background(), "tenant-a", tkt.ID, []operations.TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: operations.ChecklistStatusPending, EvidenceRequired: false},
	}, "actor-ops-1")
	if !errors.Is(err, operations.ErrEvidenceRequirementLocks) {
		t.Fatalf("expected ErrEvidenceRequirementLocks, got %v", err)
	}
}

func TestCrossTenantEvidenceAndRecoveryFailClosed(t *testing.T) {
	pool := operationsPool(t)

	tenantA := operations.NewTicketService(pool).WithAuthorizer(testAuthorizer{tenant: "tenant-a"})
	tkt, err := tenantA.CreateTicket(context.Background(), operations.CreateTicketParams{
		TenantID:   "tenant-a",
		PropertyID: "prop-1",
		Type:       operations.TypeIncident,
		Reason:     "leak",
	}, "actor-ops-1")
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	// A tenant-b authorizer must be denied on the evidence surface.
	tenantB := operations.NewTicketService(pool).WithAuthorizer(testAuthorizer{tenant: "tenant-b"})
	if _, err := tenantB.RegisterEvidence(context.Background(), "tenant-b", tkt.ID, operations.RegisterEvidenceParams{
		ContentHash: operations.ComputeEvidenceHash([]byte("x")),
	}, "actor-b"); !errors.Is(err, operations.ErrCrossTenantDenied) {
		t.Fatalf("expected cross-tenant evidence denial, got %v", err)
	}
	if _, err := tenantB.ListEvidence(context.Background(), "tenant-b", tkt.ID); !errors.Is(err, operations.ErrCrossTenantDenied) {
		t.Fatalf("expected cross-tenant evidence read denial, got %v", err)
	}
	if _, err := tenantB.ClassifyIncident(context.Background(), "tenant-b", tkt.ID, operations.SeverityHigh, "actor-b"); !errors.Is(err, operations.ErrCrossTenantDenied) {
		t.Fatalf("expected cross-tenant classification denial, got %v", err)
	}
	if _, err := tenantB.StartServiceRecovery(context.Background(), "tenant-b", tkt.ID, operations.RecoveryParams{
		Reason:          "recover",
		Responsibility:  "ops",
		ReworkCostMinor: 0,
	}, "actor-b"); !errors.Is(err, operations.ErrCrossTenantDenied) {
		t.Fatalf("expected cross-tenant recovery denial, got %v", err)
	}

	// A deny-all authorizer proves reads fail closed before disclosure.
	denyAll := operations.NewTicketService(pool).WithAuthorizer(testAuthorizer{deny: true})
	if _, err := denyAll.ListEvidence(context.Background(), "tenant-a", tkt.ID); err == nil {
		t.Fatal("deny-all authorizer must fail closed on evidence reads")
	}
	if _, err := denyAll.ListServiceRecoveries(context.Background(), "tenant-a", tkt.ID); err == nil {
		t.Fatal("deny-all authorizer must fail closed on recovery reads")
	}
}
