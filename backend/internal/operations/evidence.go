package operations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ComputeEvidenceHash returns the stable sha256 hex digest used as the
// immutable identity of evidence content.
func ComputeEvidenceHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// IsValidSHA256Hash reports whether h is a well-formed sha256 hex digest.
func IsValidSHA256Hash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// evidenceByIDIndex indexes accepted and rejected evidence records by ID so
// completion checks can resolve checklist item references.
func evidenceByIDIndex(records []EvidenceRecord) map[string]EvidenceRecord {
	idx := make(map[string]EvidenceRecord, len(records))
	for _, r := range records {
		idx[r.ID] = r
	}
	return idx
}

// itemHasAcceptedEvidence reports whether a checklist item carries at least
// one evidence reference that resolves to a clean (accepted) record.
func itemHasAcceptedEvidence(item TicketChecklistItem, evidence map[string]EvidenceRecord) bool {
	for _, id := range item.EvidenceIDs {
		rec, ok := evidence[id]
		if ok && rec.Status == EvidenceStatusAccepted {
			return true
		}
	}
	return false
}

// RequiredEvidenceBlocking returns an error when any checklist item that
// requires evidence has not been backed by clean, accepted evidence. Items
// marked not-applicable are exempt. The returned error wraps ErrEvidenceRequired
// and names every blocking item so the worker can act on it.
func RequiredEvidenceBlocking(items []TicketChecklistItem, evidence map[string]EvidenceRecord) error {
	var missing []string
	for _, item := range items {
		if !item.EvidenceRequired {
			continue
		}
		if item.Status == ChecklistStatusNA {
			continue
		}
		if !itemHasAcceptedEvidence(item, evidence) {
			missing = append(missing, item.Label)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrEvidenceRequired, strings.Join(missing, ", "))
	}
	return nil
}

// ValidateEvidenceRegistrationParams validates the immutable evidence fields
// before they are persisted.
func ValidateEvidenceRegistrationParams(p RegisterEvidenceParams) error {
	if p.ContentHash == "" {
		return fmt.Errorf("evidence content hash is required")
	}
	if !IsValidSHA256Hash(p.ContentHash) {
		return fmt.Errorf("%w: %s", ErrInvalidEvidenceHash, p.ContentHash)
	}
	if p.SizeBytes < 0 {
		return fmt.Errorf("evidence size must not be negative")
	}
	return nil
}
