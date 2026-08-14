package superhost_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/superhost"
	"comfort-curators-backend/internal/communications"
	"comfort-curators-backend/internal/contracts"
	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/property"
	"comfort-curators-backend/internal/reservations"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	tenantA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tenantB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	propA   = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	propB   = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
)

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
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)

	automation.EnsureSchema(context.Background(), pool)
	property.EnsureSchema(context.Background(), pool)
	reservations.EnsureSchema(context.Background(), pool)
	operations.EnsureSchema(context.Background(), pool)
	inventory.EnsureSchema(context.Background(), pool)
	contracts.EnsureSchema(context.Background(), pool)
	communications.EnsureSchema(context.Background(), pool)

	truncateAll(t, pool)

	return pool
}

func truncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	tables := []string{
		"agent_run_events", "agent_runs",
		"superhost_threads",
		"property_compliance_holds", "property_transitions", "properties", "owner_authority_grants",
		"reservations", "calendar_exceptions", "calendar_feeds", "external_calendar_events",
		"tickets", "ticket_evidence", "incident_alerts", "service_recoveries",
		"inventory_movements", "stock_locations",
		"service_contracts", "service_contract_versions",
		"communication_preferences",
	}
	for _, table := range tables {
		if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

func seedProperty(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID, state string) {
	t.Helper()
	address := property.Address{
		Line1:      "123 Test Street",
		City:       "Mumbai",
		State:      "Maharashtra",
		PostalCode: "400001",
		Country:    "IN",
	}
	addressJSON, _ := json.Marshal(address)
	contactsJSON, _ := json.Marshal([]property.EmergencyContact{})
	_, err := pool.Exec(context.Background(), `
		INSERT INTO properties (
			id, tenant_id, owner_authority_id, service_address, geolocation_zone,
			timezone, emergency_contacts, access_method, maximum_occupancy,
			state, owner_contract_accepted, compliance_complete, mandatory_fields_set,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())
	`, propertyID, tenantID, "auth-1", addressJSON, "zone-west", "Asia/Kolkata",
		contactsJSON, "keypad", 6, state, true, true, true, 1)
	if err != nil {
		t.Fatalf("seed property: %v", err)
	}
}

func seedComplianceHold(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID, holdID, kind, severity, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO property_compliance_holds (
			id, property_id, tenant_id, kind, severity, status, reason, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'test hold', NOW())
	`, holdID, propertyID, tenantID, kind, severity, status)
	if err != nil {
		t.Fatalf("seed compliance hold: %v", err)
	}
}

func seedReservation(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID, resvID, status string, startAt, endAt time.Time) {
	t.Helper()
	// reservations.feed_id is a foreign key into calendar_feeds; ensure the
	// hardcoded 'feed-1' referenced below actually exists first (ON
	// CONFLICT DO NOTHING so repeated calls across subtests, and calls that
	// share a tenant/property, don't collide on the primary key).
	_, err := pool.Exec(context.Background(), `
		INSERT INTO calendar_feeds (id, tenant_id, property_id, source, url)
		VALUES ('feed-1', $1, $2, 'airbnb', 'https://example.invalid/feed-1.ics')
		ON CONFLICT (id) DO NOTHING
	`, tenantID, propertyID)
	if err != nil {
		t.Fatalf("seed calendar feed: %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO reservations (
			id, tenant_id, property_id, feed_id, external_event_id, source,
			guest_summary, status, start_at, end_at, all_day, timezone, sequence,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, 'feed-1', $4, 'airbnb', 'Guest stay',
			$5, $6, $7, false, 'Asia/Kolkata', 1, 1, NOW(), NOW())
	`, resvID, tenantID, propertyID, "ext-"+resvID, status, startAt, endAt)
	if err != nil {
		t.Fatalf("seed reservation: %v", err)
	}
}

func seedTicket(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID, ticketID, ticketType, status, severity string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tickets (
			id, tenant_id, property_id, type, status, reason, severity, created_by,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'test ticket', $6, 'system', 1, NOW(), NOW())
	`, ticketID, tenantID, propertyID, ticketType, status, severity)
	if err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
}

func seedStockLocation(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID, locID, name, locationType string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO stock_locations (
			id, tenant_id, property_id, name, location_type, version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 1, NOW(), NOW())
	`, locID, tenantID, propertyID, name, locationType)
	if err != nil {
		t.Fatalf("seed stock location: %v", err)
	}
}

func seedMovement(t *testing.T, pool *pgxpool.Pool, tenantID, locID, catalogItemID string, quantity int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO inventory_movements (
			id, tenant_id, location_id, catalog_item_id, movement_type,
			quantity, actor_id, created_at
		) VALUES ($1, $2, $3, $4, 'receive', $5, 'system', NOW())
	`, fmt.Sprintf("mov-%s-%s-%d", locID, catalogItemID, quantity), tenantID, locID, catalogItemID, quantity)
	if err != nil {
		t.Fatalf("seed movement: %v", err)
	}
}

func seedAgreement(t *testing.T, pool *pgxpool.Pool, tenantID, propertyID, agreeID, status string, ver int) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO service_contracts (
			id, tenant_id, property_id, status, current_version, version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
	`, agreeID, tenantID, propertyID, status, ver, ver)
	if err != nil {
		t.Fatalf("seed agreement: %v", err)
	}
}

func seedPreference(t *testing.T, pool *pgxpool.Pool, tenantID, recipientID, audience string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO communication_preferences (
			id, tenant_id, recipient_id, audience,
			consent_transactional, consent_urgent, consent_marketing, consent_sponsored,
			channel, severity, quiet_hours_start_minute, quiet_hours_end_minute,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, true, true, false, false,
			'email', 'normal', 1320, 480, 1, NOW(), NOW())
	`, fmt.Sprintf("pref-%s-%s", recipientID, audience), tenantID, recipientID, audience)
	if err != nil {
		t.Fatalf("seed preference: %v", err)
	}
}

func setupFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	seedProperty(t, pool, tenantA, propA, "active")
	seedProperty(t, pool, tenantB, propB, "active")
	seedComplianceHold(t, pool, tenantA, propA, "hold-1", "insurance", "critical", "open")
	seedComplianceHold(t, pool, tenantA, propA, "hold-2", "registration", "non_critical", "resolved")

	now := time.Now()
	seedReservation(t, pool, tenantA, propA, "resv-1", "active",
		now.Add(24*time.Hour), now.Add(72*time.Hour))
	seedReservation(t, pool, tenantA, propA, "resv-2", "active",
		now.Add(96*time.Hour), now.Add(168*time.Hour))
	seedReservation(t, pool, tenantB, propB, "resv-3", "active",
		now.Add(48*time.Hour), now.Add(96*time.Hour))

	seedTicket(t, pool, tenantA, propA, "ticket-1", "turnover", "scheduled", "")
	seedTicket(t, pool, tenantA, propA, "ticket-2", "restock", "in_progress", "")
	seedTicket(t, pool, tenantA, propA, "ticket-3", "incident", "proposed", "high")
	seedTicket(t, pool, tenantB, propB, "ticket-4", "routine_maintenance", "draft", "")

	seedStockLocation(t, pool, tenantA, propA, "loc-1", "Property A Supplies", "property")
	seedStockLocation(t, pool, tenantA, propA, "loc-2", "Property A Linens", "property")
	seedStockLocation(t, pool, tenantB, propB, "loc-3", "Property B Supplies", "property")

	seedMovement(t, pool, tenantA, "loc-1", "soap", 50)
	seedMovement(t, pool, tenantA, "loc-1", "tissue", 30)
	seedMovement(t, pool, tenantA, "loc-2", "towel", 20)
	seedMovement(t, pool, tenantA, "loc-2", "towel", -5)
	seedMovement(t, pool, tenantB, "loc-3", "soap", 100)

	seedAgreement(t, pool, tenantA, propA, "agree-1", "accepted", 3)
	seedAgreement(t, pool, tenantB, propB, "agree-2", "accepted", 1)

	seedPreference(t, pool, tenantA, "owner-a", "owner")
	seedPreference(t, pool, tenantA, "guest-a", "guest")
	seedPreference(t, pool, tenantB, "owner-b", "owner")
}

func TestAssembleContextForValidTenantProperty(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	if pc.TenantID != tenantA {
		t.Errorf("tenant_id = %q, want %q", pc.TenantID, tenantA)
	}
	if pc.PropertyID != propA {
		t.Errorf("property_id = %q, want %q", pc.PropertyID, propA)
	}
	if pc.AssembledAt.IsZero() {
		t.Error("assembled_at must be set")
	}

	if pc.Property.State != "active" {
		t.Errorf("property state = %q, want 'active'", pc.Property.State)
	}
	if len(pc.Property.ComplianceHolds) != 2 {
		t.Errorf("expected 2 compliance holds, got %d", len(pc.Property.ComplianceHolds))
	}

	if pc.Property.Fact.Source != "properties" {
		t.Errorf("property fact source = %q, want 'properties'", pc.Property.Fact.Source)
	}
	if pc.Property.Fact.RecordKind != "property" {
		t.Errorf("property fact record_kind = %q, want 'property'", pc.Property.Fact.RecordKind)
	}
	if pc.Property.Fact.EffectiveAt.IsZero() {
		t.Error("property fact effective_at must be set")
	}

	if len(pc.Reservations) != 2 {
		t.Errorf("expected 2 active reservations near now, got %d", len(pc.Reservations))
	}
	for _, r := range pc.Reservations {
		if r.Fact.Source != "reservations" {
			t.Errorf("reservation fact source = %q, want 'reservations'", r.Fact.Source)
		}
	}

	if len(pc.Tickets) != 3 {
		t.Errorf("expected 3 tickets, got %d", len(pc.Tickets))
	}
	for _, tkt := range pc.Tickets {
		if tkt.Fact.Source != "tickets" {
			t.Errorf("ticket fact source = %q, want 'tickets'", tkt.Fact.Source)
		}
	}

	if len(pc.Stock) != 2 {
		t.Errorf("expected 2 stock locations, got %d", len(pc.Stock))
	}
	foundSupply := false
	for _, s := range pc.Stock {
		if s.Name == "Property A Supplies" {
			foundSupply = true
			if s.Balance != 80 {
				t.Errorf("supply balance = %d, want 80", s.Balance)
			}
		}
	}
	if !foundSupply {
		t.Error("supply stock location not found")
	}

	if pc.Agreement == nil {
		t.Error("expected an agreement")
	} else if pc.Agreement.ID != "agree-1" {
		t.Errorf("agreement id = %q, want 'agree-1'", pc.Agreement.ID)
	}

	if len(pc.Preferences) != 2 {
		t.Errorf("expected 2 preferences for tenant A, got %d", len(pc.Preferences))
	}

	if len(pc.Summaries) == 0 {
		t.Error("expected summary data")
	}

	for _, s := range pc.Summaries {
		if s.Fact.Source == "" {
			t.Errorf("summary fact source must be set for kind %q", s.Kind)
		}
	}
}

func TestCrossPropertyRequestIsDenied(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	_, err := assembler.Assemble(ctx, tenantA, propB, "")
	if err == nil {
		t.Fatal("expected cross-property denial")
	}
	if !strings.Contains(err.Error(), "cross-property") {
		t.Errorf("error must mention cross-property, got: %v", err)
	}

	_, err = assembler.Assemble(ctx, tenantB, propA, "")
	if err == nil {
		t.Fatal("expected cross-property denial for tenant B requesting property A")
	}
}

func TestNonExistentPropertyReturnsDenied(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	_, err := assembler.Assemble(ctx, tenantA, "nonexistent-property-id", "")
	if err == nil {
		t.Fatal("expected denial for non-existent property")
	}
	if !strings.Contains(err.Error(), "cross-property") {
		t.Errorf("error must mention cross-property for non-existent property, got: %v", err)
	}
}

func TestContextDoesNotContainSecrets(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	contextJSON, err := json.Marshal(pc)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	contextStr := strings.ToLower(string(contextJSON))

	forbidden := []string{
		"password", "secret", "access_key", "access_method",
		"encryption_key", "private_key", "token",
	}
	for _, word := range forbidden {
		if strings.Contains(contextStr, word) {
			t.Errorf("context must not contain %q", word)
		}
	}
}

func TestContextDoesNotLeakOtherTenantData(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	contextJSON, err := json.Marshal(pc)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	contextStr := string(contextJSON)

	if strings.Contains(contextStr, tenantB) {
		t.Error("context must not contain tenant B ID")
	}
	if strings.Contains(contextStr, propB) {
		t.Error("context must not contain property B ID")
	}
	if strings.Contains(contextStr, "resv-3") {
		t.Error("context must not contain tenant B's reservation")
	}
	if strings.Contains(contextStr, "ticket-4") {
		t.Error("context must not contain tenant B's ticket")
	}
	if strings.Contains(contextStr, "Property B Supplies") {
		t.Error("context must not contain tenant B's stock location names")
	}
	if strings.Contains(contextStr, "agree-2") {
		t.Error("context must not contain tenant B's agreement")
	}
	if strings.Contains(contextStr, "owner-b") {
		t.Error("context must not contain tenant B's preferences")
	}
}

func TestAssembleWithoutPropertyIDFails(t *testing.T) {
	pool := testPool(t)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	_, err := assembler.Assemble(ctx, tenantA, "", "")
	if err == nil {
		t.Fatal("expected error for empty property_id")
	}
}

func TestAssembleWithoutTenantIDFails(t *testing.T) {
	pool := testPool(t)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	_, err := assembler.Assemble(ctx, "", propA, "")
	if err == nil {
		t.Fatal("expected error for empty tenant_id")
	}
}

func TestContextHasFactReferences(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	if pc.Property.Fact.RecordID == "" {
		t.Error("property fact must have record_id")
	}

	for _, h := range pc.Property.ComplianceHolds {
		if h.Fact.RecordID == "" {
			t.Error("compliance hold fact must have record_id")
		}
		if h.Fact.RecordKind != "compliance_hold" {
			t.Errorf("compliance hold fact record_kind = %q, want 'compliance_hold'", h.Fact.RecordKind)
		}
	}

	for _, r := range pc.Reservations {
		if r.Fact.RecordID == "" {
			t.Error("reservation fact must have record_id")
		}
		if r.Fact.Source != "reservations" {
			t.Errorf("reservation fact source = %q", r.Fact.Source)
		}
	}

	for _, tkt := range pc.Tickets {
		if tkt.Fact.RecordID == "" {
			t.Error("ticket fact must have record_id")
		}
	}

	for _, s := range pc.Stock {
		if s.Fact.RecordKind != "stock_location" {
			t.Errorf("stock fact record_kind = %q, want 'stock_location'", s.Fact.RecordKind)
		}
	}

	if pc.Agreement != nil && pc.Agreement.Fact.Source != "service_contracts" {
		t.Errorf("agreement fact source = %q", pc.Agreement.Fact.Source)
	}

	for _, p := range pc.Preferences {
		if p.Fact.RecordKind != "communication_preference" {
			t.Errorf("preference fact record_kind = %q", p.Fact.RecordKind)
		}
	}
}

func TestEmptyPropertyReturnsEmptySections(t *testing.T) {
	pool := testPool(t)
	seedProperty(t, pool, tenantA, propA, "active")
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	if len(pc.Reservations) != 0 {
		t.Errorf("empty property should have 0 reservations, got %d", len(pc.Reservations))
	}
	if len(pc.Tickets) != 0 {
		t.Errorf("empty property should have 0 tickets, got %d", len(pc.Tickets))
	}
	if len(pc.Stock) != 0 {
		t.Errorf("empty property should have 0 stock locations, got %d", len(pc.Stock))
	}
	if pc.Agreement != nil {
		t.Error("empty property should have no agreement")
	}
	if pc.Property.State != "active" {
		t.Errorf("property state should be 'active', got %q", pc.Property.State)
	}
}

func TestInvalidTenantIDCrossTenantDenied(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	_, err := assembler.Assemble(ctx, "nonexistent-tenant", propA, "")
	if err == nil {
		t.Fatal("expected cross-property denial for non-existent tenant")
	}
	if !strings.Contains(err.Error(), "cross-property") {
		t.Errorf("error must mention cross-property, got: %v", err)
	}
}

func TestModelArgumentsCannotSelectOtherTenant(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	_, err := assembler.Assemble(ctx, tenantA, propB, "")
	if err == nil {
		t.Fatal("model arguments selecting another tenant's property must be denied")
	}
	if !strings.Contains(err.Error(), "cross-property") {
		t.Errorf("error must mention cross-property, got: %v", err)
	}
}

func TestAssembledContextIsValidJSON(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	contextJSON, err := json.MarshalIndent(pc, "", "  ")
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(contextJSON, &parsed); err != nil {
		t.Fatalf("context must be valid JSON: %v", err)
	}

	requiredFields := []string{
		"tenant_id", "property_id", "assembled_at",
		"property", "reservations", "tickets", "stock", "summaries",
	}
	for _, field := range requiredFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("context must contain field %q", field)
		}
	}
}

func TestContextRespectsReservationTimeWindow(t *testing.T) {
	pool := testPool(t)
	setupFixtures(t, pool)
	assembler := superhost.NewContextAssembler(pool)

	now := time.Now()
	seedReservation(t, pool, tenantA, propA, "old-resv", "active",
		now.Add(-365*24*time.Hour), now.Add(-360*24*time.Hour))

	ctx := context.Background()
	pc, err := assembler.Assemble(ctx, tenantA, propA, "")
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	for _, r := range pc.Reservations {
		if r.ID == "old-resv" {
			t.Error("old reservations outside the 30-day window must be excluded")
		}
	}
}
