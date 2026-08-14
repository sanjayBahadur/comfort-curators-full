package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RecordEvent(ctx context.Context, pool *pgxpool.Pool, runID, eventName string, eventData json.RawMessage) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO agent_run_events (run_id, event_name, event_data, occurred_at)
		 VALUES ($1, $2, $3, $4)`,
		runID, eventName, eventData, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("automation: record event: %w", err)
	}
	return nil
}

func ListEvents(ctx context.Context, pool *pgxpool.Pool, runID string) ([]AgentRunEvent, error) {
	rows, err := pool.Query(ctx,
		`SELECT event_id, run_id, event_name, event_data, occurred_at
		 FROM agent_run_events
		 WHERE run_id = $1
		 ORDER BY occurred_at ASC, event_id ASC`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("automation: list events: %w", err)
	}
	defer rows.Close()

	var events []AgentRunEvent
	for rows.Next() {
		var e AgentRunEvent
		if err := rows.Scan(&e.EventID, &e.RunID, &e.EventName, &e.EventData, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("automation: scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func ListEventsAfter(ctx context.Context, pool *pgxpool.Pool, runID string, afterOccurredAt time.Time, afterEventID string) ([]AgentRunEvent, error) {
	rows, err := pool.Query(ctx,
		`SELECT event_id, run_id, event_name, event_data, occurred_at
		 FROM agent_run_events
		 WHERE run_id = $1
		   AND (occurred_at > $2 OR (occurred_at = $2 AND event_id > $3))
		 ORDER BY occurred_at ASC, event_id ASC`,
		runID, afterOccurredAt, afterEventID,
	)
	if err != nil {
		return nil, fmt.Errorf("automation: list events after: %w", err)
	}
	defer rows.Close()

	var events []AgentRunEvent
	for rows.Next() {
		var e AgentRunEvent
		if err := rows.Scan(&e.EventID, &e.RunID, &e.EventName, &e.EventData, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("automation: scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
