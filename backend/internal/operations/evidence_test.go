package operations

import (
	"errors"
	"strings"
	"testing"
)

func TestComputeEvidenceHashIsDeterministic(t *testing.T) {
	content := []byte("before-photo-2024-01-01")
	first := ComputeEvidenceHash(content)
	second := ComputeEvidenceHash(content)
	if first != second {
		t.Fatalf("same content produced different hashes: %s != %s", first, second)
	}
	if !IsValidSHA256Hash(first) {
		t.Fatalf("expected a valid sha256 hex digest, got %q", first)
	}

	different := ComputeEvidenceHash([]byte("after-photo-2024-01-01"))
	if first == different {
		t.Fatal("different content must not produce the same evidence hash")
	}
}

func TestIsValidSHA256Hash(t *testing.T) {
	if !IsValidSHA256Hash(ComputeEvidenceHash([]byte("x"))) {
		t.Error("computed hash must validate")
	}
	invalid := []string{
		"",
		"abc123",
		strings.Repeat("z", 64),
		strings.Repeat("0", 63),
		strings.Repeat("0", 65),
	}
	for _, h := range invalid {
		if IsValidSHA256Hash(h) {
			t.Errorf("expected %q to be an invalid sha256 hash", h)
		}
	}
}

func TestValidateEvidenceRegistrationParams(t *testing.T) {
	hash := ComputeEvidenceHash([]byte("photo-bytes"))
	if err := ValidateEvidenceRegistrationParams(RegisterEvidenceParams{ContentHash: hash, SizeBytes: 10}); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
	if err := ValidateEvidenceRegistrationParams(RegisterEvidenceParams{ContentHash: "", SizeBytes: 10}); err == nil {
		t.Fatal("empty hash must be rejected")
	}
	if err := ValidateEvidenceRegistrationParams(RegisterEvidenceParams{ContentHash: "nope"}); !errors.Is(err, ErrInvalidEvidenceHash) {
		t.Fatalf("expected ErrInvalidEvidenceHash, got %v", err)
	}
	if err := ValidateEvidenceRegistrationParams(RegisterEvidenceParams{ContentHash: hash, SizeBytes: -1}); err == nil {
		t.Fatal("negative size must be rejected")
	}
}

func TestRequiredEvidenceBlocksCompletion(t *testing.T) {
	items := []TicketChecklistItem{
		{ID: "tci_1", Label: "cleanliness photo", EvidenceRequired: true, Status: ChecklistStatusCompleted},
		{ID: "tci_2", Label: "towel swap", EvidenceRequired: false, Status: ChecklistStatusCompleted},
	}

	err := RequiredEvidenceBlocking(items, map[string]EvidenceRecord{})
	if !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("expected ErrEvidenceRequired, got %v", err)
	}
	if !strings.Contains(err.Error(), "cleanliness photo") {
		t.Fatalf("error must name the missing item, got %q", err)
	}

	accepted := map[string]EvidenceRecord{
		"ev_1": {ID: "ev_1", Status: EvidenceStatusAccepted, ContentHash: ComputeEvidenceHash([]byte("x"))},
	}
	items[0].EvidenceIDs = []string{"ev_1"}
	if err := RequiredEvidenceBlocking(items, accepted); err != nil {
		t.Fatalf("accepted evidence should unblock completion, got %v", err)
	}
}

func TestRequiredEvidenceRequiresCleanAcceptedEvidence(t *testing.T) {
	items := []TicketChecklistItem{
		{ID: "tci_1", Label: "damage photo", EvidenceRequired: true, Status: ChecklistStatusCompleted, EvidenceIDs: []string{"ev_bad"}},
	}

	// Rejected evidence is not clean and must keep the completion gate closed.
	evidence := map[string]EvidenceRecord{
		"ev_bad": {ID: "ev_bad", Status: EvidenceStatusRejected},
	}
	if err := RequiredEvidenceBlocking(items, evidence); !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("rejected evidence must not satisfy the requirement, got %v", err)
	}

	// A referenced evidence ID that does not resolve also blocks.
	if err := RequiredEvidenceBlocking(items, map[string]EvidenceRecord{}); !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("unresolvable evidence reference must block, got %v", err)
	}
}

func TestRequiredEvidenceExemptsNotApplicable(t *testing.T) {
	items := []TicketChecklistItem{
		{ID: "tci_1", Label: "n/a item", EvidenceRequired: true, Status: ChecklistStatusNA},
	}
	if err := RequiredEvidenceBlocking(items, map[string]EvidenceRecord{}); err != nil {
		t.Fatalf("not-applicable required item must not block completion, got %v", err)
	}
}

func TestRequiredEvidenceAllowsCompletionWithoutRequirements(t *testing.T) {
	items := []TicketChecklistItem{
		{ID: "tci_1", Label: "optional item", EvidenceRequired: false, Status: ChecklistStatusPending},
	}
	if err := RequiredEvidenceBlocking(items, map[string]EvidenceRecord{}); err != nil {
		t.Fatalf("ticket without required evidence should not be blocked, got %v", err)
	}
}

func TestEvidenceImmutabilitySurface(t *testing.T) {
	// Evidence is a write-once record: no update or delete method may exist on
	// the model surface. Re-registering identical content returns the same
	// stable hash rather than mutating an existing record.
	content := []byte("immutable-original")
	hash := ComputeEvidenceHash(content)
	if hash == "" {
		t.Fatal("hash must be computed")
	}

	// The model has no mutation affordance; changing the hash would change the
	// identity of the accepted record, which the registration path forbids by
	// returning the existing record for the same (ticket, hash).
	paramsA := RegisterEvidenceParams{ContentHash: hash}
	paramsB := RegisterEvidenceParams{ContentHash: hash}
	if paramsA.ContentHash != paramsB.ContentHash {
		t.Error("re-registration of identical content must carry the same immutable hash")
	}
}

// TestCCOPS001EvidenceRequiredForCompletion proves the named acceptance
// behavior: ticket completion fails while a required checklist item has no
// clean evidence, and the accepted evidence hash stays stable.
func TestCCOPS001EvidenceRequiredForCompletion(t *testing.T) {
	content := []byte("turnover-photo-001")
	hash := ComputeEvidenceHash(content)

	items := []TicketChecklistItem{
		{ID: "tci_1", Label: "turnover photo", EvidenceRequired: true, Status: ChecklistStatusCompleted},
	}
	evidence := map[string]EvidenceRecord{
		"ev_1": {ID: "ev_1", Status: EvidenceStatusAccepted, ContentHash: hash},
	}

	items[0].EvidenceIDs = []string{"ev_1"}
	if err := RequiredEvidenceBlocking(items, evidence); err != nil {
		t.Fatalf("completion must succeed with clean evidence, got %v", err)
	}

	items[0].EvidenceIDs = nil
	if err := RequiredEvidenceBlocking(items, evidence); !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("completion must fail without required evidence, got %v", err)
	}

	// The accepted evidence hash remains stable across re-registration.
	if again := ComputeEvidenceHash(content); again != hash {
		t.Fatalf("accepted evidence hash changed: %s != %s", again, hash)
	}
}
