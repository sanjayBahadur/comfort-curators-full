package operations

import (
	"errors"
	"reflect"
	"testing"
)

func TestSyncPayloadHashIsDeterministic(t *testing.T) {
	items := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean", Status: ChecklistStatusCompleted},
		{TemplateItemIndex: 1, Label: "swap towels", Status: ChecklistStatusPending},
	}
	first := syncPayloadHash(items)
	second := syncPayloadHash(items)
	if first != second {
		t.Fatalf("same payload must produce the same hash: %s != %s", first, second)
	}

	items[0].Status = ChecklistStatusInProgress
	changed := syncPayloadHash(items)
	if first == changed {
		t.Fatal("different payload must produce a different hash")
	}
}

func TestDetectSyncConflictsPreservesBothVersions(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusPending, Version: 1},
	}
	existing := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusCompleted, Version: 2},
	}

	conflicts := DetectSyncConflicts(incoming, existing)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}

	c := conflicts[0]
	if c.ServerStatus != ChecklistStatusCompleted {
		t.Errorf("server version not preserved, got %q", c.ServerStatus)
	}
	if c.ClientStatus != ChecklistStatusPending {
		t.Errorf("client version not preserved, got %q", c.ClientStatus)
	}
	if c.ServerVersion != 2 {
		t.Errorf("server version number not preserved, got %d", c.ServerVersion)
	}
	if c.ClientVersion != 1 {
		t.Errorf("client version number not preserved, got %d", c.ClientVersion)
	}
}

func TestDetectSyncConflictsNoConflictWhenSame(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusCompleted, Version: 2},
	}
	existing := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusCompleted, Version: 2},
	}

	conflicts := DetectSyncConflicts(incoming, existing)
	if len(conflicts) != 0 {
		t.Fatalf("same status on both sides must not conflict, got %d", len(conflicts))
	}
}

func TestDetectSyncConflictsNewItemNoConflict(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "new item", Status: ChecklistStatusPending, Version: 0},
	}
	existing := []TicketChecklistItem{}

	conflicts := DetectSyncConflicts(incoming, existing)
	if len(conflicts) != 0 {
		t.Fatalf("new items must not conflict, got %d", len(conflicts))
	}
}

func TestDetectSyncConflictsClientAheadDoesNotConflict(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusCompleted, Version: 3},
	}
	existing := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusInProgress, Version: 2},
	}

	conflicts := DetectSyncConflicts(incoming, existing)
	if len(conflicts) != 0 {
		t.Fatalf("when client version is >= server version, no conflict should be raised, got %d", len(conflicts))
	}
}

func TestMergeSyncInsertsNewItems(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean", Status: ChecklistStatusPending},
		{TemplateItemIndex: 1, Label: "towel swap", Status: ChecklistStatusCompleted},
	}
	existing := []TicketChecklistItem{}

	merged, conflicts := MergeSync(incoming, existing)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged items, got %d", len(merged))
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestMergeSyncUpdatesExistingAndPreservesExtraServerItems(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean", Status: ChecklistStatusCompleted},
	}
	existing := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean", ID: "tci_1", Version: 1, Status: ChecklistStatusPending},
		{TemplateItemIndex: 1, Label: "towel swap", ID: "tci_2", Version: 1, Status: ChecklistStatusPending},
	}

	merged, conflicts := MergeSync(incoming, existing)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged items (updated + preserved), got %d", len(merged))
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts on clean update, got %d", len(conflicts))
	}

	for _, m := range merged {
		if m.TemplateItemIndex == 0 {
			if m.ID != "tci_1" {
				t.Error("existing item lost its ID")
			}
			if m.Status != ChecklistStatusCompleted {
				t.Errorf("item not updated, status is %q", m.Status)
			}
		}
		if m.TemplateItemIndex == 1 {
			if m.ID != "tci_2" {
				t.Error("server-only item ID not preserved")
			}
		}
	}
}

func TestMergeSyncRaisesConflictWhenServerIsAhead(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusInProgress, Version: 1},
	}
	existing := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", ID: "tci_1", Status: ChecklistStatusCompleted, Version: 3},
	}

	merged, conflicts := MergeSync(incoming, existing)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict when server is ahead and status differs, got %d", len(conflicts))
	}
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged item (server version), got %d", len(merged))
	}
	if merged[0].Status != ChecklistStatusCompleted {
		t.Errorf("merge must preserve the server version in the result, got %q", merged[0].Status)
	}

	c := conflicts[0]
	if c.ServerStatus != ChecklistStatusCompleted || c.ClientStatus != ChecklistStatusInProgress {
		t.Error("conflict must preserve both server and client versions")
	}
}

func TestPreserveCompletedWorkOnInterrupt(t *testing.T) {
	completed := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean", Status: ChecklistStatusCompleted},
		{TemplateItemIndex: 2, Label: "restock", Status: ChecklistStatusCompleted},
	}
	allIndexes := []int{0, 1, 2}

	missing := PreserveCompletedWorkOnInterrupt(completed, allIndexes)
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing item index, got %d", len(missing))
	}
	if missing[0] != 1 {
		t.Errorf("expected missing index 1, got %d", missing[0])
	}
}

func TestPreserveCompletedWorkAllDone(t *testing.T) {
	completed := []TicketChecklistItem{
		{TemplateItemIndex: 0},
		{TemplateItemIndex: 1},
	}
	allIndexes := []int{0, 1}

	missing := PreserveCompletedWorkOnInterrupt(completed, allIndexes)
	if len(missing) != 0 {
		t.Fatalf("expected no missing items when all completed, got %v", missing)
	}
}

func TestValidateSyncItems(t *testing.T) {
	items := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "clean", Status: ChecklistStatusCompleted},
	}
	if err := validateSyncItems(items); err != nil {
		t.Fatalf("valid items rejected: %v", err)
	}

	if err := validateSyncItems([]TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "", Status: ChecklistStatusPending},
	}); err == nil {
		t.Fatal("empty label must be rejected")
	}

	if err := validateSyncItems([]TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "x", Status: "bogus"},
	}); err == nil {
		t.Fatal("invalid status must be rejected")
	}
}

func TestValidateOfflineEvidenceParams(t *testing.T) {
	hash := ComputeEvidenceHash([]byte("offline-photo"))
	if err := ValidateOfflineEvidenceParams(hash, 1024); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
	if err := ValidateOfflineEvidenceParams("", 1024); err == nil {
		t.Fatal("empty hash must be rejected")
	}
	if err := ValidateOfflineEvidenceParams("not-a-hash", 1024); !errors.Is(err, ErrInvalidEvidenceHash) {
		t.Fatalf("expected ErrInvalidEvidenceHash, got %v", err)
	}
	if err := ValidateOfflineEvidenceParams(hash, -1); err == nil {
		t.Fatal("negative size must be rejected")
	}
}

func TestValidateLanguage(t *testing.T) {
	if err := ValidateLanguage("en"); err != nil {
		t.Fatalf("en language rejected: %v", err)
	}
	if err := ValidateLanguage("hi"); err != nil {
		t.Fatalf("hi language rejected: %v", err)
	}
	if err := ValidateLanguage("EN"); err != nil {
		t.Fatalf("EN (uppercase) rejected: %v", err)
	}
	if err := ValidateLanguage("HI"); err != nil {
		t.Fatalf("HI (uppercase) rejected: %v", err)
	}
	if err := ValidateLanguage(""); err != nil {
		t.Fatalf("empty language rejected: %v", err)
	}
	if err := ValidateLanguage("fr"); err == nil {
		t.Fatal("unsupported language must be rejected")
	}
}

func TestSyncKeyConflictIsVisiblyReported(t *testing.T) {
	hash1 := syncPayloadHash([]TicketChecklistItem{{TemplateItemIndex: 0, Label: "a", Status: ChecklistStatusPending}})
	hash2 := syncPayloadHash([]TicketChecklistItem{{TemplateItemIndex: 0, Label: "a", Status: ChecklistStatusCompleted}})

	if hash1 == hash2 {
		t.Fatal("different statuses must produce different hashes for conflict detection")
	}

	// A sync record with hash1 under a given key means hash2 is a conflict.
	if hash1 == hash2 {
		t.Error("hashes must differ for conflict surface")
	}
}

func TestBuildAndParseSyncResultRoundTrip(t *testing.T) {
	items := []TicketChecklistItem{
		{ID: "tci_a", TemplateItemIndex: 0},
		{ID: "tci_b", TemplateItemIndex: 1},
	}
	result := buildSyncResult(items)
	ids := parseSyncResult(result)

	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if ids[0] != "tci_a" || ids[1] != "tci_b" {
		t.Errorf("round-trip mismatch: %v", ids)
	}
}

func TestIsSyncConflictResolved(t *testing.T) {
	open := SyncConflict{}
	if IsSyncConflictResolved(open) {
		t.Error("unresolved conflict must not report as resolved")
	}

	resolved := SyncConflict{Resolved: true, Resolution: "accept_server", ResolvedBy: "actor-ops-1"}
	if !IsSyncConflictResolved(resolved) {
		t.Error("resolved conflict must report as resolved")
	}

	partial := SyncConflict{Resolved: true, Resolution: "", ResolvedBy: "actor-ops-1"}
	if IsSyncConflictResolved(partial) {
		t.Error("conflict without resolution text must not be reported as resolved")
	}
}

func TestValidSyncStatuses(t *testing.T) {
	statuses := ValidSyncStatuses()
	if len(statuses) != 4 {
		t.Fatalf("expected 4 valid sync statuses, got %d", len(statuses))
	}

	expected := []string{ChecklistStatusPending, ChecklistStatusInProgress, ChecklistStatusCompleted, ChecklistStatusNA}
	if !reflect.DeepEqual(statuses, expected) {
		t.Fatalf("expected %v, got %v", expected, statuses)
	}
}

func TestMergeSyncPreservesBothWhenConflict(t *testing.T) {
	incoming := []TicketChecklistItem{
		{TemplateItemIndex: 0, Label: "photo", Status: ChecklistStatusInProgress, Version: 1},
	}
	existing := []TicketChecklistItem{
		{TemplateItemIndex: 0, ID: "tci_srv", Label: "photo", Status: ChecklistStatusCompleted, Version: 3},
	}

	merged, conflicts := MergeSync(incoming, existing)

	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].ServerStatus != ChecklistStatusCompleted {
		t.Errorf("server status not preserved in conflict, got %q", conflicts[0].ServerStatus)
	}
	if conflicts[0].ClientStatus != ChecklistStatusInProgress {
		t.Errorf("client status not preserved in conflict, got %q", conflicts[0].ClientStatus)
	}
	if conflicts[0].ServerVersion != 3 {
		t.Errorf("server version not preserved, got %d", conflicts[0].ServerVersion)
	}
	if conflicts[0].ClientVersion != 1 {
		t.Errorf("client version not preserved, got %d", conflicts[0].ClientVersion)
	}

	// Server version is retained in merged result.
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(merged))
	}
	if merged[0].ID != "tci_srv" {
		t.Errorf("server item ID not preserved in merge result, got %q", merged[0].ID)
	}
	if merged[0].Status != ChecklistStatusCompleted {
		t.Errorf("merged result must retain server status, got %q", merged[0].Status)
	}
}
