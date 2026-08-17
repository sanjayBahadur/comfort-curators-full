package superhost

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultTicketLimit       = 50
	defaultReservationWindow = 30 * 24 * time.Hour
)

type ContextAssembler struct {
	pool  *pgxpool.Pool
	tasks *AccountTaskStore
}

func NewContextAssembler(pool *pgxpool.Pool) *ContextAssembler {
	return &ContextAssembler{pool: pool, tasks: NewAccountTaskStore(pool)}
}

// Assemble builds the property-scoped context for a run. actorID is
// optional (pass "" if unknown, e.g. from a caller that hasn't been
// updated to thread the real actor through yet) -- when empty, the
// account task ledger is simply omitted rather than erroring, since a
// missing actor identity shouldn't block context assembly entirely.
// actorRole is likewise optional and, when empty, simply omitted from the
// context rather than guessed.
func (a *ContextAssembler) Assemble(ctx context.Context, tenantID, propertyID, actorID, actorRole string) (*PropertyContext, error) {
	if tenantID == "" {
		return nil, ErrTenantScopeRequired
	}
	if propertyID == "" {
		return nil, fmt.Errorf("superhost: property_id is required")
	}

	var exists bool
	err := a.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM properties WHERE id = $1 AND tenant_id = $2)`,
		propertyID, tenantID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("superhost: verify property scope: %w", err)
	}
	if !exists {
		return nil, ErrCrossPropertyDenied
	}

	pc := &PropertyContext{
		TenantID:    tenantID,
		PropertyID:  propertyID,
		ActorRole:   actorRole,
		AssembledAt: time.Now(),
	}

	prop, err := a.assembleProperty(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	pc.Property = prop

	reservations, err := a.assembleReservations(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	pc.Reservations = reservations

	tickets, err := a.assembleTickets(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	pc.Tickets = tickets

	stock, err := a.assembleStock(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	pc.Stock = stock

	agreement, err := a.assembleAgreement(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	pc.Agreement = agreement

	prefs, err := a.assemblePreferences(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	pc.Preferences = prefs

	summaries, err := a.assembleSummaries(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	pc.Summaries = summaries

	if actorID != "" {
		accountTasks, err := a.tasks.ListForAccount(ctx, tenantID, actorID, 5)
		if err != nil {
			return nil, fmt.Errorf("superhost: assemble account tasks: %w", err)
		}
		for _, t := range accountTasks {
			pc.AccountTasks = append(pc.AccountTasks, ContextAccountTask{
				ID:           t.ID,
				Description:  t.Description,
				Status:       t.Status,
				ResolvedNote: t.ResolvedNote,
			})
		}
	}

	return pc, nil
}

// AssemblePortfolio builds context spanning every property on the tenant,
// for a thread that isn't scoped to one property (see
// PortfolioPropertyIDSentinel in models.go and handler.go's handling of
// it). This is what lets one Superhost thread reason about and act on
// several properties without the human re-scoping between each -- "manage
// individual properties in parallel" in one conversation, not N separate
// property-locked threads. Each property still gets the exact same
// per-property assembly as Assemble (same real reservations/tickets/
// stock/compliance data); this only adds the loop and a shared account-
// task ledger fetched once, not duplicated per property.
func (a *ContextAssembler) AssemblePortfolio(ctx context.Context, tenantID, actorID, actorRole string) (*PortfolioContext, error) {
	if tenantID == "" {
		return nil, ErrTenantScopeRequired
	}

	rows, err := a.pool.Query(ctx, `SELECT id FROM properties WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("superhost: list portfolio properties: %w", err)
	}
	var propertyIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("superhost: scan portfolio property: %w", err)
		}
		propertyIDs = append(propertyIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("superhost: list portfolio properties: %w", err)
	}

	pc := &PortfolioContext{TenantID: tenantID, ActorRole: actorRole, AssembledAt: time.Now()}
	for _, propertyID := range propertyIDs {
		propContext, err := a.Assemble(ctx, tenantID, propertyID, "", actorRole)
		if err != nil {
			return nil, fmt.Errorf("superhost: assemble portfolio property %s: %w", propertyID, err)
		}
		pc.Properties = append(pc.Properties, *propContext)
	}

	if actorID != "" {
		accountTasks, err := a.tasks.ListForAccount(ctx, tenantID, actorID, 5)
		if err != nil {
			return nil, fmt.Errorf("superhost: assemble account tasks: %w", err)
		}
		for _, t := range accountTasks {
			pc.AccountTasks = append(pc.AccountTasks, ContextAccountTask{
				ID:           t.ID,
				Description:  t.Description,
				Status:       t.Status,
				ResolvedNote: t.ResolvedNote,
			})
		}
	}

	return pc, nil
}

func (a *ContextAssembler) assembleProperty(ctx context.Context, tenantID, propertyID string) (ContextProperty, error) {
	var p ContextProperty
	var addressJSON []byte

	err := a.pool.QueryRow(ctx, `
		SELECT id, state, service_address, geolocation_zone, timezone, maximum_occupancy,
			owner_contract_accepted, compliance_complete, mandatory_fields_set, updated_at
		FROM properties
		WHERE id = $1 AND tenant_id = $2
	`, propertyID, tenantID).Scan(
		&p.ID, &p.State, &addressJSON, &p.GeolocationZone, &p.Timezone, &p.MaximumOccupancy,
		&p.Readiness.OwnerContractAccepted, &p.Readiness.ComplianceComplete, &p.Readiness.MandatoryFieldsSet,
		&p.Fact.EffectiveAt,
	)
	if err != nil {
		return ContextProperty{}, fmt.Errorf("superhost: assemble property: %w", err)
	}

	if err := json.Unmarshal(addressJSON, &p.ServiceAddress); err != nil {
		return ContextProperty{}, fmt.Errorf("superhost: decode address: %w", err)
	}

	p.Fact.Source = "properties"
	p.Fact.RecordID = p.ID
	p.Fact.RecordKind = "property"

	holds, err := a.assembleHolds(ctx, tenantID, propertyID)
	if err != nil {
		return ContextProperty{}, fmt.Errorf("superhost: assemble holds: %w", err)
	}
	p.ComplianceHolds = holds

	return p, nil
}

func (a *ContextAssembler) assembleHolds(ctx context.Context, tenantID, propertyID string) ([]ContextHold, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, kind, severity, status, reason, expires_at, resolved_at, created_at
		FROM property_compliance_holds
		WHERE property_id = $1 AND tenant_id = $2
		ORDER BY created_at ASC
	`, propertyID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("superhost: query holds: %w", err)
	}
	defer rows.Close()

	var holds []ContextHold
	for rows.Next() {
		var h ContextHold
		if err := rows.Scan(&h.Fact.RecordID, &h.Kind, &h.Severity, &h.Status, &h.Reason,
			&h.ExpiresAt, &h.ResolvedAt, &h.Fact.EffectiveAt); err != nil {
			return nil, fmt.Errorf("superhost: scan hold: %w", err)
		}
		h.Fact.Source = "property_compliance_holds"
		h.Fact.RecordKind = "compliance_hold"
		holds = append(holds, h)
	}
	return holds, rows.Err()
}

func (a *ContextAssembler) assembleReservations(ctx context.Context, tenantID, propertyID string) ([]ContextReservation, error) {
	now := time.Now()
	cutoff := now.Add(-defaultReservationWindow)

	rows, err := a.pool.Query(ctx, `
		SELECT id, status, start_at, end_at, timezone, all_day, updated_at
		FROM reservations
		WHERE tenant_id = $1 AND property_id = $2
			AND end_at >= $3
			AND status = 'active'
		ORDER BY start_at ASC
	`, tenantID, propertyID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("superhost: query reservations: %w", err)
	}
	defer rows.Close()

	var reservations []ContextReservation
	for rows.Next() {
		var rv ContextReservation
		var tz *string
		if err := rows.Scan(&rv.ID, &rv.Status, &rv.StartAt, &rv.EndAt, &tz, &rv.AllDay, &rv.Fact.EffectiveAt); err != nil {
			return nil, fmt.Errorf("superhost: scan reservation: %w", err)
		}
		if tz != nil {
			rv.Timezone = *tz
		}
		rv.Fact.Source = "reservations"
		rv.Fact.RecordID = rv.ID
		rv.Fact.RecordKind = "reservation"
		reservations = append(reservations, rv)
	}
	return reservations, rows.Err()
}

func (a *ContextAssembler) assembleTickets(ctx context.Context, tenantID, propertyID string) ([]ContextTicket, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, type, status, reason, severity, updated_at
		FROM tickets
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY updated_at DESC
		LIMIT $3
	`, tenantID, propertyID, defaultTicketLimit)
	if err != nil {
		return nil, fmt.Errorf("superhost: query tickets: %w", err)
	}
	defer rows.Close()

	var tickets []ContextTicket
	for rows.Next() {
		var t ContextTicket
		var severity *string
		if err := rows.Scan(&t.ID, &t.Type, &t.Status, &t.Reason, &severity, &t.Fact.EffectiveAt); err != nil {
			return nil, fmt.Errorf("superhost: scan ticket: %w", err)
		}
		if severity != nil {
			t.Severity = *severity
		}
		t.Fact.Source = "tickets"
		t.Fact.RecordID = t.ID
		t.Fact.RecordKind = "ticket"
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (a *ContextAssembler) assembleStock(ctx context.Context, tenantID, propertyID string) ([]ContextStockLocation, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT sl.id, sl.name, sl.location_type, sl.updated_at,
			COALESCE(SUM(im.quantity), 0) AS balance
		FROM stock_locations sl
		LEFT JOIN inventory_movements im
			ON im.location_id = sl.id AND im.tenant_id = sl.tenant_id
		WHERE sl.tenant_id = $1 AND sl.property_id = $2
		GROUP BY sl.id, sl.name, sl.location_type, sl.updated_at
		ORDER BY sl.name ASC
	`, tenantID, propertyID)
	if err != nil {
		return nil, fmt.Errorf("superhost: query stock: %w", err)
	}
	defer rows.Close()

	var stock []ContextStockLocation
	for rows.Next() {
		var s ContextStockLocation
		if err := rows.Scan(&s.ID, &s.Name, &s.LocationType, &s.Fact.EffectiveAt, &s.Balance); err != nil {
			return nil, fmt.Errorf("superhost: scan stock: %w", err)
		}
		s.Fact.Source = "inventory"
		s.Fact.RecordID = s.ID
		s.Fact.RecordKind = "stock_location"
		stock = append(stock, s)
	}
	return stock, rows.Err()
}

func (a *ContextAssembler) assembleAgreement(ctx context.Context, tenantID, propertyID string) (*ContextAgreement, error) {
	var ag ContextAgreement
	err := a.pool.QueryRow(ctx, `
		SELECT id, status, version, updated_at
		FROM service_contracts
		WHERE tenant_id = $1 AND property_id = $2 AND status = 'accepted'
		ORDER BY version DESC
		LIMIT 1
	`, tenantID, propertyID).Scan(&ag.ID, &ag.Status, &ag.Version, &ag.Fact.EffectiveAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("superhost: query agreement: %w", err)
	}
	ag.Fact.Source = "service_contracts"
	ag.Fact.RecordID = ag.ID
	ag.Fact.RecordKind = "service_contract"
	return &ag, nil
}

func (a *ContextAssembler) assemblePreferences(ctx context.Context, tenantID string) ([]ContextPreference, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT recipient_id, audience, consent_transactional, consent_urgent,
			channel, quiet_hours_start_minute, quiet_hours_end_minute, updated_at
		FROM communication_preferences
		WHERE tenant_id = $1
		ORDER BY audience, recipient_id
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("superhost: query preferences: %w", err)
	}
	defer rows.Close()

	var prefs []ContextPreference
	for rows.Next() {
		var p ContextPreference
		if err := rows.Scan(&p.RecipientID, &p.Audience,
			&p.ConsentTransactional, &p.ConsentUrgent,
			&p.Channel, &p.QuietHoursStartMinute, &p.QuietHoursEndMinute,
			&p.Fact.EffectiveAt); err != nil {
			return nil, fmt.Errorf("superhost: scan preference: %w", err)
		}
		p.Fact.Source = "communication_preferences"
		p.Fact.RecordID = p.RecipientID + "." + p.Audience
		p.Fact.RecordKind = "communication_preference"
		prefs = append(prefs, p)
	}
	return prefs, rows.Err()
}

func (a *ContextAssembler) assembleSummaries(ctx context.Context, tenantID, propertyID string) ([]ContextSummary, error) {
	var summaries []ContextSummary

	openTickets, err := a.countTicketsByStatus(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	summaries = append(summaries, openTickets...)

	activeReservations, err := a.countActiveReservations(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	summaries = append(summaries, activeReservations...)

	openHolds, err := a.countOpenCriticalHolds(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	summaries = append(summaries, openHolds...)

	stockSummary, err := a.summarizeStock(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	summaries = append(summaries, stockSummary...)

	return summaries, nil
}

func (a *ContextAssembler) countTicketsByStatus(ctx context.Context, tenantID, propertyID string) ([]ContextSummary, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT status, COUNT(*), MAX(updated_at)
		FROM tickets
		WHERE tenant_id = $1 AND property_id = $2
		GROUP BY status
	`, tenantID, propertyID)
	if err != nil {
		return nil, fmt.Errorf("superhost: count tickets: %w", err)
	}
	defer rows.Close()

	var summaries []ContextSummary
	for rows.Next() {
		var s ContextSummary
		if err := rows.Scan(&s.Label, &s.Value, &s.Fact.EffectiveAt); err != nil {
			return nil, fmt.Errorf("superhost: scan ticket count: %w", err)
		}
		s.Kind = "ticket_count_by_status"
		s.Fact.Source = "tickets"
		s.Fact.RecordKind = "ticket_aggregate"
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

func (a *ContextAssembler) countActiveReservations(ctx context.Context, tenantID, propertyID string) ([]ContextSummary, error) {
	now := time.Now()
	var s ContextSummary
	err := a.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(MAX(updated_at), NOW())
		FROM reservations
		WHERE tenant_id = $1 AND property_id = $2
			AND status = 'active' AND end_at >= $3
	`, tenantID, propertyID, now).Scan(&s.Value, &s.Fact.EffectiveAt)
	if err != nil {
		return nil, fmt.Errorf("superhost: count active reservations: %w", err)
	}
	s.Kind = "active_reservation_count"
	s.Label = "active_reservations"
	s.Fact.Source = "reservations"
	s.Fact.RecordKind = "reservation_aggregate"
	return []ContextSummary{s}, nil
}

func (a *ContextAssembler) countOpenCriticalHolds(ctx context.Context, tenantID, propertyID string) ([]ContextSummary, error) {
	var s ContextSummary
	err := a.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(MAX(created_at), NOW())
		FROM property_compliance_holds
		WHERE tenant_id = $1 AND property_id = $2
			AND status = 'open' AND severity = 'critical'
	`, tenantID, propertyID).Scan(&s.Value, &s.Fact.EffectiveAt)
	if err != nil {
		return nil, fmt.Errorf("superhost: count critical holds: %w", err)
	}
	s.Kind = "open_critical_hold_count"
	s.Label = "open_critical_holds"
	s.Fact.Source = "property_compliance_holds"
	s.Fact.RecordKind = "compliance_hold_aggregate"
	return []ContextSummary{s}, nil
}

func (a *ContextAssembler) summarizeStock(ctx context.Context, tenantID, propertyID string) ([]ContextSummary, error) {
	var s ContextSummary
	err := a.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(quantity), 0), COALESCE(MAX(im.created_at), NOW())
		FROM inventory_movements im
		JOIN stock_locations sl ON sl.id = im.location_id AND sl.tenant_id = im.tenant_id
		WHERE im.tenant_id = $1 AND sl.property_id = $2
	`, tenantID, propertyID).Scan(&s.Value, &s.Fact.EffectiveAt)
	if err != nil {
		return nil, fmt.Errorf("superhost: summarize stock: %w", err)
	}
	s.Kind = "total_stock_balance"
	s.Label = "total_stock_at_property"
	s.Fact.Source = "inventory"
	s.Fact.RecordKind = "inventory_aggregate"
	return []ContextSummary{s}, nil
}

func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}
