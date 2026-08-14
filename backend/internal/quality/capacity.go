package quality

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// P95TargetMilliseconds is the NFR-003 p95 server response target for owner
// and worker core pages (500 ms), excluding third-party integrations and file
// upload.
const P95TargetMilliseconds = 500.0

// CapacityTarget is the volume described by NFR-001 for the MVP: 50 active
// properties, 1,000 reservations, 100 workers or vendors, 50,000 tickets and
// 100,000 inventory movements.
type CapacityTarget struct {
	Properties   int `json:"properties"`
	Reservations int `json:"reservations"`
	Workers      int `json:"workers"`
	Tickets      int `json:"tickets"`
	Movements    int `json:"movements"`
}

// DefaultCapacityTarget returns the NFR-001 capacity volume.
func DefaultCapacityTarget() CapacityTarget {
	return CapacityTarget{
		Properties:   50,
		Reservations: 1000,
		Workers:      100,
		Tickets:      50000,
		Movements:    100000,
	}
}

// Validate ensures every dimension is a non-negative integer and that the
// property-anchored dimensions (reservations, tickets, movements) require at
// least one property, since those records are property-scoped.
func (t CapacityTarget) Validate() error {
	if t.Properties < 0 || t.Reservations < 0 || t.Workers < 0 || t.Tickets < 0 || t.Movements < 0 {
		return fmt.Errorf("capacity target dimensions must be non-negative")
	}
	if t.Properties == 0 && (t.Reservations > 0 || t.Tickets > 0 || t.Movements > 0) {
		return fmt.Errorf("capacity target requires properties when reservations, tickets or movements are requested")
	}
	return nil
}

// CapacitySeeds is what SeedCapacity wrote for one tenant. The identifiers are
// derived from the tenant so multiple capacity scenarios can share a database.
type CapacitySeeds struct {
	TenantID        string   `json:"tenant_id"`
	Prefix          string   `json:"prefix"`
	PropertyIDs     []string `json:"property_ids"`
	LocationIDs     []string `json:"location_ids"`
	FirstPropertyID string   `json:"first_property_id"`
	FirstLocationID string   `json:"first_location_id"`
}

// CapacityResult records what the capacity scenario measured and confirmed.
type CapacityResult struct {
	TenantID      string           `json:"tenant_id"`
	Target        CapacityTarget   `json:"target"`
	Counts        map[string]int64 `json:"counts"`
	Completed     bool             `json:"completed"`
	SeedSeconds   float64          `json:"seed_seconds"`
	VerifySeconds float64          `json:"verify_seconds"`
	Queries       []QueryDuration  `json:"queries"`
}

// QueryDuration records one representative core query and its latency.
type QueryDuration struct {
	Query string  `json:"query"`
	MS    float64 `json:"ms"`
}

// seedIDPrefix derives a short, stable per-tenant token so globally-unique
// primary keys do not collide when several tenants are seeded into one schema.
func seedIDPrefix(tenantID string) string {
	sum := sha256.Sum256([]byte(tenantID))
	return hex.EncodeToString(sum[:4])
}

// SeedCapacity bulk-loads the NFR-001 volume for a dedicated tenant using COPY
// so the scenario completes quickly against the existing schema. The tenant
// identifier is caller-selected so the immutable inventory movements ledger can
// be exercised without destructive cleanup.
func SeedCapacity(ctx context.Context, pool *pgxpool.Pool, tenantID string, target CapacityTarget) (*CapacitySeeds, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}

	prefix := seedIDPrefix(tenantID)
	seeds := &CapacitySeeds{TenantID: tenantID, Prefix: prefix}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin capacity seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	propertyIDs := make([]string, 0, target.Properties)
	for i := 0; i < target.Properties; i++ {
		propertyIDs = append(propertyIDs, fmt.Sprintf("%s-prop-%05d", prefix, i))
	}
	seeds.PropertyIDs = propertyIDs
	if len(propertyIDs) > 0 {
		seeds.FirstPropertyID = propertyIDs[0]
	}

	feedIDs := make([]string, 0, target.Properties)
	for i := 0; i < target.Properties; i++ {
		feedIDs = append(feedIDs, fmt.Sprintf("%s-feed-%05d", prefix, i))
	}

	if target.Properties > 0 {
		feedRows := make([][]any, 0, target.Properties)
		for i, id := range feedIDs {
			feedRows = append(feedRows, []any{
				id, tenantID, propertyIDs[i], "airbnb",
				fmt.Sprintf("https://feeds.invalid/%s-%05d.ics", prefix, i),
				"active", "Asia/Kolkata", 1440, 180,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"calendar_feeds"},
			[]string{"id", "tenant_id", "property_id", "source", "url", "status", "property_timezone", "stale_after_minutes", "minimum_turnaround_minutes"},
			pgx.CopyFromRows(feedRows)); err != nil {
			return nil, fmt.Errorf("seed calendar_feeds: %w", err)
		}

		propRows := make([][]any, 0, target.Properties)
		for i, id := range propertyIDs {
			propRows = append(propRows, []any{
				id, tenantID, fmt.Sprintf("%s-owner-%d", prefix, i),
				fmt.Sprintf(`{"line1":"Capacity Rd %d","city":"New Delhi","state":"Delhi","postal_code":"110001","country":"IN"}`, i),
				"zone-north", "Asia/Kolkata", "[]", "keycode", 4,
				"active", true, true, true, 1,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"properties"},
			[]string{"id", "tenant_id", "owner_authority_id", "service_address", "geolocation_zone", "timezone", "emergency_contacts", "access_method", "maximum_occupancy", "state", "owner_contract_accepted", "compliance_complete", "mandatory_fields_set", "version"},
			pgx.CopyFromRows(propRows)); err != nil {
			return nil, fmt.Errorf("seed properties: %w", err)
		}

		resRows := make([][]any, 0, target.Reservations)
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < target.Reservations; i++ {
			propIdx := i % target.Properties
			start := base.Add(time.Duration(i) * 24 * time.Hour)
			resRows = append(resRows, []any{
				fmt.Sprintf("%s-res-%06d", prefix, i), tenantID, propertyIDs[propIdx], feedIDs[propIdx],
				fmt.Sprintf("%s-ext-%06d", prefix, i), "airbnb", "capacity guest", "active",
				start, start.Add(2 * 24 * time.Hour), false, "Asia/Kolkata",
				0, 1,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"reservations"},
			[]string{"id", "tenant_id", "property_id", "feed_id", "external_event_id", "source", "guest_summary", "status", "start_at", "end_at", "all_day", "timezone", "sequence", "version"},
			pgx.CopyFromRows(resRows)); err != nil {
			return nil, fmt.Errorf("seed reservations: %w", err)
		}
	}

	if target.Workers > 0 {
		workerRows := make([][]any, 0, target.Workers)
		for i := 0; i < target.Workers; i++ {
			classification := "employee"
			if i%2 == 1 {
				classification = "vendor"
			}
			workerRows = append(workerRows, []any{
				fmt.Sprintf("%s-worker-%04d", prefix, i), tenantID,
				fmt.Sprintf("Capacity Worker %d", i), true,
				time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), true,
				"phone", classification, i%5 == 0, "zone-north",
				`["cleaning"]`, "active", 1,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"workers"},
			[]string{"id", "tenant_id", "legal_name", "verified_identity", "date_of_birth", "age_eligible", "contact_method", "classification", "specialist", "service_zone", "skills", "status", "version"},
			pgx.CopyFromRows(workerRows)); err != nil {
			return nil, fmt.Errorf("seed workers: %w", err)
		}
	}

	if target.Properties > 0 {
		locRows := make([][]any, 0, 10)
		locIDs := make([]string, 0, 10)
		for i := 0; i < 10; i++ {
			locID := fmt.Sprintf("%s-loc-%02d", prefix, i)
			locIDs = append(locIDs, locID)
			locRows = append(locRows, []any{
				locID, tenantID, propertyIDs[i%len(propertyIDs)],
				fmt.Sprintf("Location %d", i), "storage", 1,
			})
		}
		seeds.LocationIDs = locIDs
		if len(locIDs) > 0 {
			seeds.FirstLocationID = locIDs[0]
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"stock_locations"},
			[]string{"id", "tenant_id", "property_id", "name", "location_type", "version"},
			pgx.CopyFromRows(locRows)); err != nil {
			return nil, fmt.Errorf("seed stock_locations: %w", err)
		}
	}

	if target.Tickets > 0 {
		ticketRows := make([][]any, 0, target.Tickets)
		for i := 0; i < target.Tickets; i++ {
			propIdx := i % target.Properties
			// Status is derived from the per-property index so every property
			// carries a mix of open, assigned and completed tickets.
			j := i / target.Properties
			status := "open"
			if j%3 == 0 {
				status = "assigned"
			} else if j%5 == 0 {
				status = "completed"
			}
			workerRef := fmt.Sprintf("%s-worker-0000", prefix)
			if target.Workers > 0 {
				workerRef = fmt.Sprintf("%s-worker-%04d", prefix, i%target.Workers)
			}
			ticketRows = append(ticketRows, []any{
				fmt.Sprintf("%s-ticket-%06d", prefix, i), tenantID, propertyIDs[propIdx],
				"cleaning", status, "capacity run",
				"{}", nil, fmt.Sprintf("%s-actor-%d", prefix, i%100), workerRef,
				nil, nil, nil, nil, nil, nil, 1,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tickets"},
			[]string{"id", "tenant_id", "property_id", "type", "status", "reason", "requested_window", "checklist_version_id", "created_by", "assigned_to", "verified_by", "verifier_note", "blocker", "follow_up_ticket_id", "reopen_reason", "notification_intent", "version"},
			pgx.CopyFromRows(ticketRows)); err != nil {
			return nil, fmt.Errorf("seed tickets: %w", err)
		}
	}

	if target.Movements > 0 {
		locIDs := make([]string, 0, 10)
		for i := 0; i < 10; i++ {
			locIDs = append(locIDs, fmt.Sprintf("%s-loc-%02d", prefix, i))
		}
		movementRows := make([][]any, 0, target.Movements)
		for i := 0; i < target.Movements; i++ {
			movementType := "restock"
			qty := int64(1)
			if i%2 == 0 {
				movementType = "consumption"
				qty = -1
			}
			ticketRef := fmt.Sprintf("%s-ticket-000000", prefix)
			if target.Tickets > 0 {
				ticketRef = fmt.Sprintf("%s-ticket-%06d", prefix, i%target.Tickets)
			}
			workerRef := fmt.Sprintf("%s-worker-0000", prefix)
			if target.Workers > 0 {
				workerRef = fmt.Sprintf("%s-worker-%04d", prefix, i%target.Workers)
			}
			movementRows = append(movementRows, []any{
				fmt.Sprintf("%s-mov-%07d", prefix, i), tenantID, locIDs[i%10],
				fmt.Sprintf("%s-item-%d", prefix, i%50), movementType, qty,
				"ticket", ticketRef,
				"capacity run", workerRef,
				nil,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"inventory_movements"},
			[]string{"id", "tenant_id", "location_id", "catalog_item_id", "movement_type", "quantity", "reference_type", "reference_id", "reason", "actor_id", "expires_at"},
			pgx.CopyFromRows(movementRows)); err != nil {
			return nil, fmt.Errorf("seed inventory_movements: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit capacity seed: %w", err)
	}
	return seeds, nil
}

// VerifyCapacity confirms the seeded volume is queryable through the existing
// schema without architectural change. It counts each dimension and runs the
// representative index-backed queries the core pages rely on at scale.
func VerifyCapacity(ctx context.Context, pool *pgxpool.Pool, seeds *CapacitySeeds, target CapacityTarget) (CapacityResult, error) {
	start := time.Now()
	result := CapacityResult{
		TenantID: seeds.TenantID,
		Target:   target,
		Counts:   map[string]int64{},
		Queries:  []QueryDuration{},
	}

	counts := map[string]string{
		"properties":   "properties",
		"reservations": "reservations",
		"workers":      "workers",
		"tickets":      "tickets",
		"movements":    "inventory_movements",
	}
	for key, table := range counts {
		var n int64
		if err := pool.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE tenant_id=$1", table), seeds.TenantID,
		).Scan(&n); err != nil {
			return result, fmt.Errorf("count %s: %w", key, err)
		}
		result.Counts[key] = n
	}

	min := func(got, want int64) bool { return got >= want }

	if !min(result.Counts["properties"], int64(target.Properties)) {
		return result, fmt.Errorf("capacity: properties=%d below target %d", result.Counts["properties"], target.Properties)
	}
	if !min(result.Counts["reservations"], int64(target.Reservations)) {
		return result, fmt.Errorf("capacity: reservations=%d below target %d", result.Counts["reservations"], target.Reservations)
	}
	if !min(result.Counts["workers"], int64(target.Workers)) {
		return result, fmt.Errorf("capacity: workers=%d below target %d", result.Counts["workers"], target.Workers)
	}
	if !min(result.Counts["tickets"], int64(target.Tickets)) {
		return result, fmt.Errorf("capacity: tickets=%d below target %d", result.Counts["tickets"], target.Tickets)
	}
	if !min(result.Counts["movements"], int64(target.Movements)) {
		return result, fmt.Errorf("capacity: movements=%d below target %d", result.Counts["movements"], target.Movements)
	}

	coreQueries := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "tickets_by_status",
			sql:  "SELECT COUNT(*) FROM tickets WHERE tenant_id=$1 AND property_id=$2 AND status=$3",
			args: []any{seeds.TenantID, seeds.FirstPropertyID, "open"},
		},
		{
			name: "movements_by_location_and_item",
			sql:  "SELECT COUNT(*) FROM inventory_movements WHERE tenant_id=$1 AND location_id=$2 AND catalog_item_id=$3",
			args: []any{seeds.TenantID, seeds.FirstLocationID, fmt.Sprintf("%s-item-0", seeds.Prefix)},
		},
		{
			name: "reservations_by_property",
			sql:  "SELECT COUNT(*) FROM reservations WHERE tenant_id=$1 AND property_id=$2",
			args: []any{seeds.TenantID, seeds.FirstPropertyID},
		},
		{
			name: "active_workers",
			sql:  "SELECT COUNT(*) FROM workers WHERE tenant_id=$1 AND status=$2",
			args: []any{seeds.TenantID, "active"},
		},
	}

	for _, q := range coreQueries {
		qStart := time.Now()
		var n int64
		if err := pool.QueryRow(ctx, q.sql, q.args...).Scan(&n); err != nil {
			return result, fmt.Errorf("core query %s: %w", q.name, err)
		}
		if n < 1 {
			return result, fmt.Errorf("core query %s returned no rows at capacity", q.name)
		}
		result.Queries = append(result.Queries, QueryDuration{
			Query: q.name,
			MS:    float64(time.Since(qStart).Microseconds()) / 1000.0,
		})
	}

	result.VerifySeconds = round2(time.Since(start).Seconds())
	result.Completed = true
	return result, nil
}

// MeasureCorePaths measures p95 latency of the authenticated core server paths
// and returns a summary per path. It creates an owner session for the given
// tenant before measuring and uses the seeded first property for the worker
// tickets page exactly as the worker core page does.
func MeasureCorePaths(ctx context.Context, baseURL, tenantID, firstPropertyID string, iterations int) ([]LatencySummary, error) {
	if iterations <= 0 {
		iterations = 8
	}

	authHeader, err := createOwnerSession(ctx, baseURL, tenantID)
	if err != nil {
		return nil, err
	}

	paths := []string{
		"/v1/properties",
		"/v1/tickets?property_id=" + firstPropertyID,
		"/v1/inventory/locations",
	}
	summaries := make([]LatencySummary, 0, len(paths))
	for _, path := range paths {
		series := NewLatencySeries()
		if _, err := timedGET(ctx, baseURL, path, authHeader); err != nil {
			return nil, fmt.Errorf("warm up %s: %w", path, err)
		}
		for i := 0; i < iterations; i++ {
			ms, err := timedGET(ctx, baseURL, path, authHeader)
			if err != nil && strings.Contains(err.Error(), "429") {
				// Rate-limit contention with other probes on the same path key;
				// retry without recording the rejected attempt.
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(2 * time.Second):
				}
				ms, err = timedGET(ctx, baseURL, path, authHeader)
			}
			if err != nil {
				return nil, fmt.Errorf("measure %s: %w", path, err)
			}
			series.Observe(ms)
		}
		summaries = append(summaries, series.Summary(path, P95TargetMilliseconds))
	}

	return summaries, nil
}

func timedGET(ctx context.Context, baseURL, path, authHeader string) (float64, error) {
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return elapsed, fmt.Errorf("GET %s: expected 2xx, got %d", path, resp.StatusCode)
	}
	return elapsed, nil
}

func createOwnerSession(ctx context.Context, baseURL, tenantID string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"tenant_id": tenantID,
		"contact":   "quality-capacity@test.invalid",
		"roles":     []string{"owner"},
	})
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(baseURL, "/") + "/auth/session/create"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	const maxAttempts = 5
	var lastBody string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts-1 {
			// The API rate limiter is per IP+path and shared across the whole
			// acceptance suite; back off briefly and retry instead of failing.
			lastBody = string(respBody)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
			}
			continue
		}
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("session create: expected 2xx, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			SessionToken string `json:"session_token"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", fmt.Errorf("parse session create: %w", err)
		}
		if result.SessionToken == "" {
			return "", fmt.Errorf("session create returned empty session_token (body: %s)", lastBody)
		}
		return "Bearer " + result.SessionToken, nil
	}
	return "", fmt.Errorf("session create: rate limited after %d attempts: %s", maxAttempts, lastBody)
}
