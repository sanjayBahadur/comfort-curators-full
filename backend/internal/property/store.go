package property

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx so store operations
// can run inside a transaction when atomicity requires it.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PropertyStore struct {
	pool *pgxpool.Pool
}

func NewPropertyStore(pool *pgxpool.Pool) *PropertyStore {
	return &PropertyStore{pool: pool}
}

func (s *PropertyStore) Create(ctx context.Context, p *Property) error {
	return createProperty(ctx, s.pool, p)
}

func createProperty(ctx context.Context, q querier, p *Property) error {
	addressJSON, err := json.Marshal(p.ServiceAddress)
	if err != nil {
		return fmt.Errorf("marshal service address: %w", err)
	}
	contactsJSON, err := json.Marshal(p.EmergencyContacts)
	if err != nil {
		return fmt.Errorf("marshal emergency contacts: %w", err)
	}
	if p.ID == "" {
		p.ID = newID("prop")
	}
	if p.Timezone == "" {
		p.Timezone = "Asia/Kolkata"
	}
	if p.Version == 0 {
		p.Version = 1
	}

	err = q.QueryRow(ctx, `
		INSERT INTO properties (
			id, tenant_id, owner_authority_id, service_address, geolocation_zone,
			timezone, emergency_contacts, access_method, maximum_occupancy,
			state, owner_contract_accepted, compliance_complete, mandatory_fields_set,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())
		RETURNING created_at, updated_at
	`,
		p.ID, p.TenantID, p.OwnerAuthorityID, addressJSON, p.GeolocationZone,
		p.Timezone, contactsJSON, p.AccessMethod, p.MaximumOccupancy,
		p.State, p.Readiness.OwnerContractAccepted, p.Readiness.ComplianceComplete,
		p.Readiness.MandatoryFieldsSet, p.Version,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert property: %w", err)
	}
	return nil
}

func (s *PropertyStore) Get(ctx context.Context, tenantID, propertyID string) (*Property, error) {
	p, err := s.getByID(ctx, s.pool, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	holds, err := listHolds(ctx, s.pool, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	p.ComplianceHolds = holds
	return p, nil
}

func (s *PropertyStore) getByID(ctx context.Context, q querier, tenantID, propertyID string) (*Property, error) {
	var p Property
	var addressJSON, contactsJSON []byte
	err := q.QueryRow(ctx, `
		SELECT id, tenant_id, owner_authority_id, service_address, geolocation_zone,
			timezone, emergency_contacts, access_method, maximum_occupancy,
			state, owner_contract_accepted, compliance_complete, mandatory_fields_set,
			version, created_at, updated_at
		FROM properties
		WHERE id = $1 AND tenant_id = $2
	`, propertyID, tenantID).Scan(
		&p.ID, &p.TenantID, &p.OwnerAuthorityID, &addressJSON, &p.GeolocationZone,
		&p.Timezone, &contactsJSON, &p.AccessMethod, &p.MaximumOccupancy,
		&p.State, &p.Readiness.OwnerContractAccepted, &p.Readiness.ComplianceComplete,
		&p.Readiness.MandatoryFieldsSet, &p.Version, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPropertyNotFound
		}
		return nil, fmt.Errorf("get property: %w", err)
	}
	if err := json.Unmarshal(addressJSON, &p.ServiceAddress); err != nil {
		return nil, fmt.Errorf("decode service address: %w", err)
	}
	if len(contactsJSON) > 0 {
		if err := json.Unmarshal(contactsJSON, &p.EmergencyContacts); err != nil {
			return nil, fmt.Errorf("decode emergency contacts: %w", err)
		}
	}
	return &p, nil
}

func (s *PropertyStore) List(ctx context.Context, tenantID string) ([]Property, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, owner_authority_id, service_address, geolocation_zone,
			timezone, emergency_contacts, access_method, maximum_occupancy,
			state, owner_contract_accepted, compliance_complete, mandatory_fields_set,
			version, created_at, updated_at
		FROM properties
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list properties: %w", err)
	}
	defer rows.Close()

	var props []Property
	for rows.Next() {
		var p Property
		var addressJSON, contactsJSON []byte
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.OwnerAuthorityID, &addressJSON, &p.GeolocationZone,
			&p.Timezone, &contactsJSON, &p.AccessMethod, &p.MaximumOccupancy,
			&p.State, &p.Readiness.OwnerContractAccepted, &p.Readiness.ComplianceComplete,
			&p.Readiness.MandatoryFieldsSet, &p.Version, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan property: %w", err)
		}
		if err := json.Unmarshal(addressJSON, &p.ServiceAddress); err != nil {
			return nil, fmt.Errorf("decode service address: %w", err)
		}
		if len(contactsJSON) > 0 {
			if err := json.Unmarshal(contactsJSON, &p.EmergencyContacts); err != nil {
				return nil, fmt.Errorf("decode emergency contacts: %w", err)
			}
		}
		props = append(props, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list properties rows: %w", err)
	}

	for i := range props {
		holds, err := listHolds(ctx, s.pool, tenantID, props[i].ID)
		if err != nil {
			return nil, err
		}
		props[i].ComplianceHolds = holds
	}
	return props, nil
}

func (s *PropertyStore) UpdateState(ctx context.Context, p *Property) error {
	return updatePropertyState(ctx, s.pool, p)
}

func updatePropertyState(ctx context.Context, q querier, p *Property) error {
	tag, err := q.Exec(ctx, `
		UPDATE properties
		SET state = $3, version = $4, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND version = $5
	`, p.ID, p.TenantID, p.State, p.Version, p.Version-1)
	if err != nil {
		return fmt.Errorf("update property state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("property state update lost a concurrent write (optimistic version)")
	}
	return nil
}

func (s *PropertyStore) InsertTransition(ctx context.Context, t PropertyTransition) error {
	return insertTransition(ctx, s.pool, t)
}

func insertTransition(ctx context.Context, q querier, t PropertyTransition) error {
	_, err := q.Exec(ctx, `
		INSERT INTO property_transitions (
			id, property_id, tenant_id, from_state, to_state, actor_id,
			reason, from_version, to_version, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, t.ID, t.PropertyID, t.TenantID, t.FromState, t.ToState, t.ActorID,
		t.Reason, t.FromVersion, t.ToVersion)
	if err != nil {
		return fmt.Errorf("insert property transition: %w", err)
	}
	return nil
}

func (s *PropertyStore) ListTransitions(ctx context.Context, tenantID, propertyID string) ([]PropertyTransition, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, property_id, tenant_id, from_state, to_state, actor_id,
			reason, from_version, to_version, created_at
		FROM property_transitions
		WHERE property_id = $1 AND tenant_id = $2
		ORDER BY created_at ASC
	`, propertyID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list property transitions: %w", err)
	}
	defer rows.Close()

	var transitions []PropertyTransition
	for rows.Next() {
		var t PropertyTransition
		if err := rows.Scan(
			&t.ID, &t.PropertyID, &t.TenantID, &t.FromState, &t.ToState, &t.ActorID,
			&t.Reason, &t.FromVersion, &t.ToVersion, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan property transition: %w", err)
		}
		transitions = append(transitions, t)
	}
	return transitions, rows.Err()
}

func (s *PropertyStore) InsertComplianceHold(ctx context.Context, q pgx.Tx, hold *ComplianceHold) error {
	return insertComplianceHold(ctx, q, hold)
}

func insertComplianceHold(ctx context.Context, q querier, hold *ComplianceHold) error {
	if hold.ID == "" {
		hold.ID = newID("hold")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO property_compliance_holds (
			id, property_id, tenant_id, kind, severity, status, reason,
			expires_at, exception_by, exception_at, exception_expires_at,
			resolved_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
	`,
		hold.ID, hold.PropertyID, hold.TenantID, hold.Kind, hold.Severity, hold.Status,
		hold.Reason, hold.ExpiresAt, hold.ExceptionBy, hold.ExceptionAt,
		hold.ExceptionExpiresAt, hold.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("insert compliance hold: %w", err)
	}
	return nil
}

func (s *PropertyStore) UpdateComplianceHold(ctx context.Context, hold *ComplianceHold) error {
	return updateComplianceHold(ctx, s.pool, hold)
}

func updateComplianceHold(ctx context.Context, q querier, hold *ComplianceHold) error {
	tag, err := q.Exec(ctx, `
		UPDATE property_compliance_holds
		SET status = $4, exception_by = $5, exception_at = $6,
			exception_expires_at = $7, resolved_at = $8
		WHERE id = $1 AND property_id = $2 AND tenant_id = $3
	`,
		hold.ID, hold.PropertyID, hold.TenantID, hold.Status, hold.ExceptionBy,
		hold.ExceptionAt, hold.ExceptionExpiresAt, hold.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("update compliance hold: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrHoldNotFound
	}
	return nil
}

func listHolds(ctx context.Context, q querier, tenantID, propertyID string) ([]ComplianceHold, error) {
	rows, err := q.Query(ctx, `
		SELECT id, property_id, tenant_id, kind, severity, status, reason,
			expires_at, exception_by, exception_at, exception_expires_at,
			resolved_at, created_at
		FROM property_compliance_holds
		WHERE property_id = $1 AND tenant_id = $2
		ORDER BY created_at ASC
	`, propertyID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list compliance holds: %w", err)
	}
	defer rows.Close()

	var holds []ComplianceHold
	for rows.Next() {
		var h ComplianceHold
		if err := rows.Scan(
			&h.ID, &h.PropertyID, &h.TenantID, &h.Kind, &h.Severity, &h.Status, &h.Reason,
			&h.ExpiresAt, &h.ExceptionBy, &h.ExceptionAt, &h.ExceptionExpiresAt,
			&h.ResolvedAt, &h.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan compliance hold: %w", err)
		}
		holds = append(holds, h)
	}
	return holds, rows.Err()
}

// GrantOwnerAuthority records that actorID controls authorityID, if it is
// not already recorded. Idempotent so repeated property creation under the
// same authority is a no-op after the first grant.
func (s *PropertyStore) GrantOwnerAuthority(ctx context.Context, q querier, tenantID, actorID, authorityID string) error {
	if actorID == "" || authorityID == "" {
		return nil
	}
	_, err := q.Exec(ctx, `
		INSERT INTO owner_authority_grants (tenant_id, actor_id, authority_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (actor_id, authority_id) DO NOTHING
	`, tenantID, actorID, authorityID)
	if err != nil {
		return fmt.Errorf("grant owner authority: %w", err)
	}
	return nil
}

// ListAuthoritiesForActor returns every owner authority actorID has been
// granted, across all tenants they hold a grant in.
func (s *PropertyStore) ListAuthoritiesForActor(ctx context.Context, actorID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT authority_id FROM owner_authority_grants WHERE actor_id = $1
	`, actorID)
	if err != nil {
		return nil, fmt.Errorf("list authorities for actor: %w", err)
	}
	defer rows.Close()

	var authorities []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, fmt.Errorf("scan authority: %w", err)
		}
		authorities = append(authorities, a)
	}
	return authorities, rows.Err()
}
