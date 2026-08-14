package operations

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

func syncPayloadHash(items []TicketChecklistItem) string {
	canonical := make([]map[string]any, len(items))
	for i, item := range items {
		canonical[i] = map[string]any{
			"template_item_index": item.TemplateItemIndex,
			"label":               item.Label,
			"status":              item.Status,
			"completed_by":        item.CompletedBy,
			"evidence_ids":        item.EvidenceIDs,
			"evidence_required":   item.EvidenceRequired,
			"notes":               item.Notes,
		}
	}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func buildSyncResult(items []TicketChecklistItem) string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	raw, _ := json.Marshal(ids)
	return string(raw)
}

func parseSyncResult(raw string) []string {
	var ids []string
	json.Unmarshal([]byte(raw), &ids)
	return ids
}

// DetectSyncConflicts compares incoming checklist items against the persisted
// server version. When an item exists on the server with a higher version than
// the client provides and the status differs, a SyncConflict is raised so both
// versions remain visible. Items that match or only exist on one side are not
// conflicted.
func DetectSyncConflicts(incoming []TicketChecklistItem, existing []TicketChecklistItem) []SyncConflict {
	existingIdx := make(map[int]TicketChecklistItem, len(existing))
	for _, e := range existing {
		existingIdx[e.TemplateItemIndex] = e
	}

	var conflicts []SyncConflict
	for _, inc := range incoming {
		serv, ok := existingIdx[inc.TemplateItemIndex]
		if !ok {
			continue
		}
		if inc.Version == 0 || serv.Version <= inc.Version {
			continue
		}
		if inc.Status == serv.Status && inc.Label == serv.Label {
			continue
		}
		conflicts = append(conflicts, SyncConflict{
			ChecklistItemID:   serv.ID,
			TemplateItemIndex: inc.TemplateItemIndex,
			ServerLabel:       serv.Label,
			ServerStatus:      serv.Status,
			ServerVersion:     serv.Version,
			ClientLabel:       inc.Label,
			ClientStatus:      inc.Status,
			ClientVersion:     inc.Version,
		})
	}
	return conflicts
}

// MergeSync applies each incoming checklist item to the store, handling
// inserts for new items, safe updates for items without conflicts, and
// recording conflicts for items the server has progressed ahead of the client.
// Returns the list of resulting items and any conflicts raised.
func MergeSync(incoming []TicketChecklistItem, existing []TicketChecklistItem) (
	merged []TicketChecklistItem,
	conflicts []SyncConflict,
) {
	existingIdx := make(map[int]TicketChecklistItem, len(existing))
	for _, e := range existing {
		existingIdx[e.TemplateItemIndex] = e
	}

	for _, inc := range incoming {
		serv, ok := existingIdx[inc.TemplateItemIndex]
		if !ok {
			merged = append(merged, inc)
			continue
		}

		if inc.Version > 0 && serv.Version > inc.Version {
			if inc.Status != serv.Status || inc.Label != serv.Label {
				conflicts = append(conflicts, SyncConflict{
					ChecklistItemID:   serv.ID,
					TemplateItemIndex: inc.TemplateItemIndex,
					ServerLabel:       serv.Label,
					ServerStatus:      serv.Status,
					ServerVersion:     serv.Version,
					ClientLabel:       inc.Label,
					ClientStatus:      inc.Status,
					ClientVersion:     inc.Version,
				})
				merged = append(merged, serv)
				continue
			}
		}

		inc.ID = serv.ID
		inc.Version = serv.Version
		merged = append(merged, inc)
	}

	for _, e := range existing {
		found := false
		for _, inc := range incoming {
			if inc.TemplateItemIndex == e.TemplateItemIndex {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, e)
		}
	}

	return merged, conflicts
}

// IsSyncConflictResolved reports whether a conflict has been resolved with a
// declared resolution action (server or client) by a specific actor.
func IsSyncConflictResolved(c SyncConflict) bool {
	return c.Resolved && c.Resolution != "" && c.ResolvedBy != ""
}

// ValidateOfflineEvidenceParams enforces that queued offline evidence carries
// the required identity fields before being stored.
func ValidateOfflineEvidenceParams(contentHash string, sizeBytes int64) error {
	if contentHash == "" {
		return fmt.Errorf("offline evidence content hash is required")
	}
	if !IsValidSHA256Hash(contentHash) {
		return fmt.Errorf("%w: %s", ErrInvalidEvidenceHash, contentHash)
	}
	if sizeBytes < 0 {
		return fmt.Errorf("offline evidence size must not be negative")
	}
	return nil
}

// PreserveCompletedWorkOnInterrupt accepts a partial sync result after a
// transaction rollback. Items that were successfully committed in a previous
// sync remain untouched; only items in the aborted batch need to be retried.
// The function returns the list of template_item_index values that were NOT
// persisted (the caller should retry only those items).
func PreserveCompletedWorkOnInterrupt(completedItems []TicketChecklistItem, allIndexes []int) []int {
	completedIdx := map[int]bool{}
	for _, item := range completedItems {
		completedIdx[item.TemplateItemIndex] = true
	}

	var missing []int
	for _, idx := range allIndexes {
		if !completedIdx[idx] {
			missing = append(missing, idx)
		}
	}
	return missing
}

// SyncResultView builds the API-visible payload for a sync result.
type SyncResultView struct {
	Items     []TicketChecklistItem `json:"items"`
	Conflicts []SyncConflict        `json:"conflicts,omitempty"`
	Replay    bool                  `json:"replay"`
}

// ValidSyncStatuses returns true for any recognised checklist status.
func ValidSyncStatuses() []string {
	return []string{ChecklistStatusPending, ChecklistStatusInProgress, ChecklistStatusCompleted, ChecklistStatusNA}
}

func isValidChecklistStatus(s string) bool {
	for _, vs := range ValidSyncStatuses() {
		if vs == s {
			return true
		}
	}
	return false
}

func validateSyncItems(items []TicketChecklistItem) error {
	for _, item := range items {
		if item.Label == "" {
			return fmt.Errorf("checklist item label is required for template_item_index %d", item.TemplateItemIndex)
		}
		if !isValidChecklistStatus(item.Status) {
			return fmt.Errorf("invalid checklist status %q for item %q", item.Status, item.Label)
		}
	}
	return nil
}

// OfflineEvidencePayload carries the critical flow content that a Curator
// device captures while offline. The language field signals which locale
// payload the client used for labels and hints.
type OfflineEvidencePayload struct {
	ContentHash  string `json:"content_hash"`
	FileName     string `json:"file_name,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	Language     string `json:"language,omitempty"`
	CapturedNote string `json:"captured_note,omitempty"`
}

// ValidateLanguage enforces that the language tag is either English (en) or
// Hindi (hi), the two critical-flow locales required by the frozen product.
func ValidateLanguage(lang string) error {
	if lang == "" {
		return nil
	}
	lang = strings.ToLower(lang)
	if lang != "en" && lang != "hi" {
		return fmt.Errorf("unsupported language %q; only en and hi are supported", lang)
	}
	return nil
}
