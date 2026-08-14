package reservations

import (
	"time"
)

// ReservationFromEvent derives a normalized reservation from an external
// calendar event. The reservation keeps the source identity so a later poll
// matches and updates the same operational record, and cancelled events map to
// cancelled reservations instead of being forgotten.
func ReservationFromEvent(ev *ExternalCalendarEvent) *Reservation {
	status := ReservationStatusActive
	if !ev.IsActive() {
		status = ReservationStatusCancelled
	}
	return &Reservation{
		TenantID:        ev.TenantID,
		PropertyID:      ev.PropertyID,
		FeedID:          ev.FeedID,
		ExternalEventID: ev.ExternalEventID,
		Source:          ev.Source,
		GuestSummary:    ev.Summary,
		Status:          status,
		StartAt:         ev.StartAt,
		EndAt:           ev.EndAt,
		AllDay:          ev.AllDay,
		Timezone:        ev.Timezone,
		Sequence:        ev.Sequence,
	}
}

// ConflictFromDetection builds a human-reviewable reservation conflict for one
// detected issue. EventIDs carries the internal external-calendar-event ids so
// the store can resolve them to normalized reservations.
type ConflictFromDetection struct {
	Conflict  *ReservationConflict
	EventIDs  []string
	Kind      string
	DedupeKey string
}

// ConflictsFromDetection converts every detected issue that represents a
// booking conflict into a ReservationConflict shell. Dedupe keys match the
// calendar exceptions so a repeated poll cannot duplicate either surface.
func ConflictsFromDetection(tenantID, propertyID string, detection Detection, now time.Time) []ConflictFromDetection {
	var out []ConflictFromDetection
	for _, ov := range detection.Overlaps {
		pair := sortedEventPair(ov.EventAID, ov.EventBID)
		out = append(out, ConflictFromDetection{
			Kind:      ExceptionKindOverlap,
			DedupeKey: pair,
			EventIDs:  []string{ov.EventAID, ov.EventBID},
			Conflict: &ReservationConflict{
				TenantID:   tenantID,
				PropertyID: propertyID,
				Kind:       ExceptionKindOverlap,
				Severity:   ExceptionSeverityCritical,
				Status:     ConflictStatusOpen,
				Message:    "overlapping bookings on the same property",
				DedupeKey:  pair,
				Metadata: map[string]any{
					"start_a": ov.EventAStart.UTC().Format(time.RFC3339),
					"end_a":   ov.EventAEnd.UTC().Format(time.RFC3339),
					"start_b": ov.EventBStart.UTC().Format(time.RFC3339),
					"end_b":   ov.EventBEnd.UTC().Format(time.RFC3339),
				},
				CreatedAt: now,
			},
		})
	}

	for _, ta := range detection.ImpossibleTurnarounds {
		pair := sortedEventPair(ta.CheckoutEventID, ta.CheckinEventID)
		out = append(out, ConflictFromDetection{
			Kind:      ExceptionKindImpossibleTurnaround,
			DedupeKey: pair,
			EventIDs:  []string{ta.CheckoutEventID, ta.CheckinEventID},
			Conflict: &ReservationConflict{
				TenantID:   tenantID,
				PropertyID: propertyID,
				Kind:       ExceptionKindImpossibleTurnaround,
				Severity:   ExceptionSeverityCritical,
				Status:     ConflictStatusOpen,
				Message:    "turnaround gap shorter than the required minimum",
				DedupeKey:  pair,
				Metadata: map[string]any{
					"checkout_event":  ta.CheckoutEventID,
					"checkin_event":   ta.CheckinEventID,
					"gap_minutes":     ta.GapMinutes,
					"minimum_minutes": ta.MinimumMinutes,
				},
				CreatedAt: now,
			},
		})
	}

	for _, amb := range detection.Ambiguities {
		out = append(out, ConflictFromDetection{
			Kind:      ExceptionKindTimezoneAmbiguity,
			DedupeKey: "event:" + amb.EventID,
			EventIDs:  []string{amb.EventID},
			Conflict: &ReservationConflict{
				TenantID:   tenantID,
				PropertyID: propertyID,
				Kind:       ExceptionKindTimezoneAmbiguity,
				Severity:   ExceptionSeverityWarning,
				Status:     ConflictStatusOpen,
				Message:    "reservation timezone is ambiguous: " + amb.Reason,
				DedupeKey:  "event:" + amb.EventID,
				Metadata: map[string]any{
					"reason": amb.Reason,
				},
				CreatedAt: now,
			},
		})
	}

	for _, dup := range detection.Duplicates {
		pair := sortedEventPair(dup.EventA.ID, dup.EventB.ID)
		out = append(out, ConflictFromDetection{
			Kind:      ExceptionKindDuplicate,
			DedupeKey: pair,
			EventIDs:  []string{dup.EventA.ID, dup.EventB.ID},
			Conflict: &ReservationConflict{
				TenantID:   tenantID,
				PropertyID: propertyID,
				Kind:       ExceptionKindDuplicate,
				Severity:   ExceptionSeverityWarning,
				Status:     ConflictStatusOpen,
				Message:    "suspected duplicate reservation from a second source",
				DedupeKey:  pair,
				Metadata: map[string]any{
					"source_a":      dup.EventA.Source,
					"external_id_a": dup.EventA.ExternalEventID,
					"source_b":      dup.EventB.Source,
					"external_id_b": dup.EventB.ExternalEventID,
				},
				CreatedAt: now,
			},
		})
	}

	return out
}

// ProposalSpec describes one deterministic piece of work for a reservation.
type ProposalSpec struct {
	Kind          string
	ScheduledAt   time.Time
	ChecklistHint string
}

// ProposalSpecs is the deterministic rule: a stay requires an arrival
// inspection at check-in and a turnover after checkout.
func ProposalSpecs(r *Reservation) []ProposalSpec {
	return []ProposalSpec{
		{Kind: ProposalKindTurnover, ScheduledAt: r.EndAt, ChecklistHint: "turnover_clean"},
		{Kind: ProposalKindInspection, ScheduledAt: r.StartAt, ChecklistHint: "arrival_inspection"},
	}
}

func proposalFromSpec(r *Reservation, spec ProposalSpec) *TurnoverProposal {
	return &TurnoverProposal{
		TenantID:      r.TenantID,
		PropertyID:    r.PropertyID,
		ReservationID: r.ID,
		Kind:          spec.Kind,
		Status:        ProposalStatusProposed,
		ScheduledAt:   spec.ScheduledAt,
		ChecklistHint: spec.ChecklistHint,
	}
}

// ValidResolutionOutcome reports whether an outcome is an allowed human
// decision on a reservation conflict.
func ValidResolutionOutcome(outcome string) bool {
	switch outcome {
	case ResolutionOutcomeConfirm, ResolutionOutcomeReject, ResolutionOutcomeMerge:
		return true
	default:
		return false
	}
}
