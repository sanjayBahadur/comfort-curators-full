package reservations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS calendar_feeds (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			source TEXT NOT NULL,
			url TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			property_timezone TEXT NOT NULL DEFAULT 'Asia/Kolkata',
			stale_after_minutes INTEGER NOT NULL DEFAULT 1440,
			minimum_turnaround_minutes INTEGER NOT NULL DEFAULT 180,
			last_polled_at TIMESTAMPTZ,
			last_success_at TIMESTAMPTZ,
			last_content_hash TEXT,
			last_error TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("reservations: create calendar_feeds table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_calendar_feeds_property
			ON calendar_feeds(tenant_id, property_id)
	`); err != nil {
		return fmt.Errorf("reservations: create calendar_feeds property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS external_calendar_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			feed_id TEXT NOT NULL REFERENCES calendar_feeds(id),
			external_event_id TEXT NOT NULL,
			source TEXT NOT NULL,
			summary TEXT,
			description TEXT,
			start_at TIMESTAMPTZ NOT NULL,
			end_at TIMESTAMPTZ NOT NULL,
			all_day BOOLEAN NOT NULL DEFAULT false,
			timezone TEXT,
			timezone_ambiguous BOOLEAN NOT NULL DEFAULT false,
			status TEXT NOT NULL DEFAULT 'confirmed',
			sequence INTEGER NOT NULL DEFAULT 0,
			raw_ical TEXT NOT NULL,
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT uq_external_event_source UNIQUE (feed_id, external_event_id)
		)
	`); err != nil {
		return fmt.Errorf("reservations: create external_calendar_events table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_external_events_property
			ON external_calendar_events(tenant_id, property_id)
	`); err != nil {
		return fmt.Errorf("reservations: create external events property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_external_events_feed
			ON external_calendar_events(feed_id)
	`); err != nil {
		return fmt.Errorf("reservations: create external events feed index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS calendar_exceptions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			feed_id TEXT,
			kind TEXT NOT NULL,
			severity TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			message TEXT NOT NULL,
			dedupe_key TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ
		)
	`); err != nil {
		return fmt.Errorf("reservations: create calendar_exceptions table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_calendar_exceptions_property
			ON calendar_exceptions(tenant_id, property_id, status)
	`); err != nil {
		return fmt.Errorf("reservations: create calendar exceptions property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS uq_calendar_exceptions_open
			ON calendar_exceptions(tenant_id, property_id, kind, dedupe_key)
			WHERE status = 'open'
	`); err != nil {
		return fmt.Errorf("reservations: create calendar exceptions open dedupe index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS reservations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			feed_id TEXT NOT NULL REFERENCES calendar_feeds(id),
			external_event_id TEXT NOT NULL,
			source TEXT NOT NULL,
			guest_summary TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			start_at TIMESTAMPTZ NOT NULL,
			end_at TIMESTAMPTZ NOT NULL,
			all_day BOOLEAN NOT NULL DEFAULT false,
			timezone TEXT,
			sequence INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT uq_reservation_source UNIQUE (feed_id, external_event_id)
		)
	`); err != nil {
		return fmt.Errorf("reservations: create reservations table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_reservations_property
			ON reservations(tenant_id, property_id)
	`); err != nil {
		return fmt.Errorf("reservations: create reservations property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS reservation_conflicts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			severity TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			message TEXT NOT NULL,
			reservation_ids TEXT[] NOT NULL DEFAULT '{}',
			exception_id TEXT,
			dedupe_key TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ
		)
	`); err != nil {
		return fmt.Errorf("reservations: create reservation conflicts table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_reservation_conflicts_property
			ON reservation_conflicts(tenant_id, property_id, status)
	`); err != nil {
		return fmt.Errorf("reservations: create reservation conflicts property index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS uq_reservation_conflicts_open
			ON reservation_conflicts(tenant_id, property_id, kind, dedupe_key)
			WHERE status = 'open'
	`); err != nil {
		return fmt.Errorf("reservations: create reservation conflicts open dedupe index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS reservation_conflict_resolutions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			conflict_id TEXT NOT NULL REFERENCES reservation_conflicts(id),
			actor_id TEXT NOT NULL,
			actor_type TEXT NOT NULL DEFAULT 'operator',
			outcome TEXT NOT NULL,
			note TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("reservations: create reservation conflict resolutions table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_reservation_conflict_resolutions_conflict
			ON reservation_conflict_resolutions(conflict_id)
	`); err != nil {
		return fmt.Errorf("reservations: create conflict resolutions conflict index: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS turnover_proposals (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			reservation_id TEXT NOT NULL REFERENCES reservations(id),
			kind TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'proposed',
			scheduled_at TIMESTAMPTZ NOT NULL,
			checklist_hint TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT uq_turnover_proposal UNIQUE (reservation_id, kind)
		)
	`); err != nil {
		return fmt.Errorf("reservations: create turnover proposals table: %w", err)
	}

	if _, err := db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_turnover_proposals_property
			ON turnover_proposals(tenant_id, property_id, status)
	`); err != nil {
		return fmt.Errorf("reservations: create turnover proposals property index: %w", err)
	}

	return nil
}
