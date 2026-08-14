package reservations

import (
	"sort"
	"strings"
	"time"
)

type OverlapIssue struct {
	EventAID    string
	EventBID    string
	EventAStart time.Time
	EventAEnd   time.Time
	EventBStart time.Time
	EventBEnd   time.Time
}

type TurnaroundIssue struct {
	CheckoutEventID string
	CheckinEventID  string
	GapMinutes      int
	MinimumMinutes  int
}

type AmbiguityIssue struct {
	EventID string
	Reason  string
}

type DuplicateIssue struct {
	EventAID string
	EventBID string
	EventA   ExternalCalendarEvent
	EventB   ExternalCalendarEvent
}

type Detection struct {
	Overlaps              []OverlapIssue
	ImpossibleTurnarounds []TurnaroundIssue
	Ambiguities           []AmbiguityIssue
	Duplicates            []DuplicateIssue
}

// DetectIssues checks a property's external calendar events for overlaps,
// impossible turnaround windows, timezone ambiguity and suspected
// duplicates. Cancelled and no-longer-listed events are excluded from every
// check because they no longer represent accepted work.
func DetectIssues(events []*ExternalCalendarEvent, minimumTurnaroundMinutes int) Detection {
	var detection Detection
	if len(events) < 2 {
		return detection
	}

	active := make([]*ExternalCalendarEvent, 0, len(events))
	for _, ev := range events {
		if ev.IsActive() {
			active = append(active, ev)
		}
	}
	if len(active) < 2 {
		return detection
	}

	// Suspected duplicates: the same source UID mirrored on more than one
	// feed, or distinct source IDs that describe the same stay. These are
	// never auto-merged; they become visible exceptions for human review.
	seenByFeed := make(map[string]*ExternalCalendarEvent)
	seenByFingerprint := make(map[string]*ExternalCalendarEvent)
	for _, ev := range active {
		key := ev.ExternalEventID
		if other, ok := seenByFeed[key]; ok && other.FeedID != ev.FeedID {
			detection.Duplicates = append(detection.Duplicates, DuplicateIssue{
				EventA: *other,
				EventB: *ev,
			})
		} else if ok {
			continue
		}
		seenByFeed[key] = ev

		fp := eventFingerprint(ev)
		if other, ok := seenByFingerprint[fp]; ok && other.FeedID != ev.FeedID {
			detection.Duplicates = append(detection.Duplicates, DuplicateIssue{
				EventA: *other,
				EventB: *ev,
			})
			continue
		}
		seenByFingerprint[fp] = ev
	}

	// Timezone ambiguity is intrinsic to the event, independent of pairing.
	for _, ev := range active {
		if ev.TimezoneAmbiguous {
			reason := "floating local time with no timezone"
			if ev.Timezone == "" {
				reason = "floating local time with no timezone"
			} else {
				reason = "unknown timezone \"" + ev.Timezone + "\""
			}
			detection.Ambiguities = append(detection.Ambiguities, AmbiguityIssue{
				EventID: ev.ID,
				Reason:  reason,
			})
		}
	}

	sorted := append([]*ExternalCalendarEvent(nil), active...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartAt.Before(sorted[j].StartAt)
	})

	for i := 0; i < len(sorted); i++ {
		a := sorted[i]
		for j := i + 1; j < len(sorted); j++ {
			b := sorted[j]
			if a.StartAt.Before(b.StartAt) && !a.EndAt.After(b.StartAt) {
				break
			}
			if a.ID == b.ID {
				continue
			}
			if overlaps(a, b) {
				detection.Overlaps = append(detection.Overlaps, OverlapIssue{
					EventAID: a.ID, EventBID: b.ID,
					EventAStart: a.StartAt, EventAEnd: a.EndAt,
					EventBStart: b.StartAt, EventBEnd: b.EndAt,
				})
			}
		}
	}

	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1]
		next := sorted[i]
		if prev.EndAt.After(next.StartAt) {
			continue
		}
		gap := next.StartAt.Sub(prev.EndAt)
		gapMinutes := int(gap.Minutes())
		if gapMinutes < minimumTurnaroundMinutes {
			detection.ImpossibleTurnarounds = append(detection.ImpossibleTurnarounds, TurnaroundIssue{
				CheckoutEventID: prev.ID,
				CheckinEventID:  next.ID,
				GapMinutes:      gapMinutes,
				MinimumMinutes:  minimumTurnaroundMinutes,
			})
		}
	}

	return detection
}

func overlaps(a, b *ExternalCalendarEvent) bool {
	return a.StartAt.Before(b.EndAt) && b.StartAt.Before(a.EndAt)
}

// eventFingerprint is a canonical description of a stay used to recognize
// the same booking published under a different source UID.
func eventFingerprint(ev *ExternalCalendarEvent) string {
	summary := strings.ToLower(strings.Join(strings.Fields(ev.Summary), " "))
	return ev.StartAt.UTC().Format(time.RFC3339) + "|" +
		ev.EndAt.UTC().Format(time.RFC3339) + "|" +
		fmtBool(ev.AllDay) + "|" + summary
}

func fmtBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ExceptionsFromDetection converts detected issues into calendar exceptions
// with deterministic dedupe keys so repeated polls do not duplicate them.
func ExceptionsFromDetection(tenantID, propertyID string, detection Detection, now time.Time) []*CalendarException {
	var exceptions []*CalendarException
	for _, ov := range detection.Overlaps {
		pair := sortedEventPair(ov.EventAID, ov.EventBID)
		exceptions = append(exceptions, &CalendarException{
			TenantID:   tenantID,
			PropertyID: propertyID,
			Kind:       ExceptionKindOverlap,
			Severity:   ExceptionSeverityCritical,
			Status:     ExceptionStatusOpen,
			Message:    "overlapping bookings on the same property",
			DedupeKey:  pair,
			Metadata: map[string]any{
				"events":  []string{ov.EventAID, ov.EventBID},
				"start_a": ov.EventAStart.UTC().Format(time.RFC3339),
				"end_a":   ov.EventAEnd.UTC().Format(time.RFC3339),
				"start_b": ov.EventBStart.UTC().Format(time.RFC3339),
				"end_b":   ov.EventBEnd.UTC().Format(time.RFC3339),
			},
			CreatedAt: now,
		})
	}

	for _, ta := range detection.ImpossibleTurnarounds {
		pair := sortedEventPair(ta.CheckoutEventID, ta.CheckinEventID)
		exceptions = append(exceptions, &CalendarException{
			TenantID:   tenantID,
			PropertyID: propertyID,
			Kind:       ExceptionKindImpossibleTurnaround,
			Severity:   ExceptionSeverityCritical,
			Status:     ExceptionStatusOpen,
			Message:    "turnaround gap shorter than the required minimum",
			DedupeKey:  pair,
			Metadata: map[string]any{
				"checkout_event":  ta.CheckoutEventID,
				"checkin_event":   ta.CheckinEventID,
				"gap_minutes":     ta.GapMinutes,
				"minimum_minutes": ta.MinimumMinutes,
			},
			CreatedAt: now,
		})
	}

	for _, amb := range detection.Ambiguities {
		exceptions = append(exceptions, &CalendarException{
			TenantID:   tenantID,
			PropertyID: propertyID,
			Kind:       ExceptionKindTimezoneAmbiguity,
			Severity:   ExceptionSeverityWarning,
			Status:     ExceptionStatusOpen,
			Message:    "event timezone is ambiguous: " + amb.Reason,
			DedupeKey:  "event:" + amb.EventID,
			Metadata: map[string]any{
				"event_id": amb.EventID,
				"reason":   amb.Reason,
			},
			CreatedAt: now,
		})
	}

	for _, dup := range detection.Duplicates {
		pair := sortedEventPair(dup.EventAID, dup.EventBID)
		exceptions = append(exceptions, &CalendarException{
			TenantID:   tenantID,
			PropertyID: propertyID,
			Kind:       ExceptionKindDuplicate,
			Severity:   ExceptionSeverityWarning,
			Status:     ExceptionStatusOpen,
			Message:    "suspected duplicate booking from a second source",
			DedupeKey:  pair,
			Metadata: map[string]any{
				"events":        []string{dup.EventAID, dup.EventBID},
				"source_a":      dup.EventA.Source,
				"external_id_a": dup.EventA.ExternalEventID,
				"source_b":      dup.EventB.Source,
				"external_id_b": dup.EventB.ExternalEventID,
			},
			CreatedAt: now,
		})
	}

	return exceptions
}

func sortedEventPair(a, b string) string {
	if a <= b {
		return a + "|" + b
	}
	return b + "|" + a
}
