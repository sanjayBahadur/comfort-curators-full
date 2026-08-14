package access

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type AccessStore struct {
	pool *pgxpool.Pool
}

func NewAccessStore(pool *pgxpool.Pool) *AccessStore {
	return &AccessStore{pool: pool}
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *AccessStore) InsertSecret(ctx context.Context, q querier, sec *PropertyAccessSecret) error {
	sec.ID = newID("sec")
	_, err := q.Exec(ctx, `
		INSERT INTO property_access_secrets (
			id, tenant_id, property_id, secret_type, label,
			encrypted_value, encryption_key_id, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sec.ID, sec.TenantID, sec.PropertyID, sec.SecretType, sec.Label,
		sec.EncryptedValue, sec.EncryptionKeyID, sec.Metadata)
	return err
}

func (s *AccessStore) GetSecret(ctx context.Context, tenantID, secretID string) (*PropertyAccessSecret, error) {
	var sec PropertyAccessSecret
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, secret_type, label,
			encrypted_value, encryption_key_id, metadata, version,
			created_at, updated_at
		FROM property_access_secrets
		WHERE id = $1 AND tenant_id = $2
	`, secretID, tenantID).Scan(
		&sec.ID, &sec.TenantID, &sec.PropertyID, &sec.SecretType, &sec.Label,
		&sec.EncryptedValue, &sec.EncryptionKeyID, &sec.Metadata, &sec.Version,
		&sec.CreatedAt, &sec.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSecretNotFound
	}
	return &sec, err
}

func (s *AccessStore) ListSecrets(ctx context.Context, tenantID, propertyID string) ([]PropertyAccessSecret, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, secret_type, label,
			encrypted_value, encryption_key_id, metadata, version,
			created_at, updated_at
		FROM property_access_secrets
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY created_at
	`, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PropertyAccessSecret
	for rows.Next() {
		var sec PropertyAccessSecret
		if err := rows.Scan(
			&sec.ID, &sec.TenantID, &sec.PropertyID, &sec.SecretType, &sec.Label,
			&sec.EncryptedValue, &sec.EncryptionKeyID, &sec.Metadata, &sec.Version,
			&sec.CreatedAt, &sec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, rows.Err()
}

func (s *AccessStore) InsertGrant(ctx context.Context, q querier, g *AccessGrant) error {
	g.ID = newID("grt")
	_, err := q.Exec(ctx, `
		INSERT INTO access_grants (
			id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status, version,
			is_emergency, emergency_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, g.ID, g.TenantID, g.PropertyID, g.SecretID, g.GranteeID, g.GranterID,
		g.WindowStart, g.WindowEnd, g.Reason, g.Status, g.Version,
		g.IsEmergency, g.EmergencyReason)
	return err
}

func (s *AccessStore) GetGrant(ctx context.Context, tenantID, grantID string) (*AccessGrant, error) {
	var g AccessGrant
	var acknowledgedAt, returnedAt, revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
		FROM access_grants
		WHERE id = $1 AND tenant_id = $2
	`, grantID, tenantID).Scan(
		&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
		&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
		&acknowledgedAt, &returnedAt, &revokedAt,
		&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
		&g.Version, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGrantNotFound
	}
	if acknowledgedAt != nil {
		g.AcknowledgedAt = acknowledgedAt
	}
	if returnedAt != nil {
		g.ReturnedAt = returnedAt
	}
	if revokedAt != nil {
		g.RevokedAt = revokedAt
	}
	return &g, err
}

func (s *AccessStore) GetActiveGrantForGrantee(ctx context.Context, tenantID, propertyID, granteeID string) (*AccessGrant, error) {
	var g AccessGrant
	var acknowledgedAt, returnedAt, revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
		FROM access_grants
		WHERE tenant_id = $1 AND property_id = $2 AND grantee_id = $3
			AND status IN ('active', 'acknowledged')
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, propertyID, granteeID).Scan(
		&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
		&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
		&acknowledgedAt, &returnedAt, &revokedAt,
		&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
		&g.Version, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGrantNotFound
	}
	if acknowledgedAt != nil {
		g.AcknowledgedAt = acknowledgedAt
	}
	if returnedAt != nil {
		g.ReturnedAt = returnedAt
	}
	if revokedAt != nil {
		g.RevokedAt = revokedAt
	}
	return &g, err
}

func (s *AccessStore) ListGrants(ctx context.Context, tenantID, propertyID string) ([]AccessGrant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
		FROM access_grants
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY created_at DESC
	`, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AccessGrant
	for rows.Next() {
		var g AccessGrant
		var acknowledgedAt, returnedAt, revokedAt *time.Time
		if err := rows.Scan(
			&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
			&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
			&acknowledgedAt, &returnedAt, &revokedAt,
			&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
			&g.Version, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if acknowledgedAt != nil {
			g.AcknowledgedAt = acknowledgedAt
		}
		if returnedAt != nil {
			g.ReturnedAt = returnedAt
		}
		if revokedAt != nil {
			g.RevokedAt = revokedAt
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *AccessStore) UpdateGrantStatus(ctx context.Context, q querier, tenantID, grantID, status string) (*AccessGrant, error) {
	var g AccessGrant
	var acknowledgedAt, returnedAt, revokedAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE access_grants
		SET status = $3, updated_at = $4, version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
	`, grantID, tenantID, status, now).Scan(
		&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
		&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
		&acknowledgedAt, &returnedAt, &revokedAt,
		&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
		&g.Version, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGrantNotFound
	}
	if acknowledgedAt != nil {
		g.AcknowledgedAt = acknowledgedAt
	}
	if returnedAt != nil {
		g.ReturnedAt = returnedAt
	}
	if revokedAt != nil {
		g.RevokedAt = revokedAt
	}
	return &g, err
}

func (s *AccessStore) AcknowledgeGrant(ctx context.Context, q querier, tenantID, grantID string) (*AccessGrant, error) {
	var g AccessGrant
	var acknowledgedAt, returnedAt, revokedAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE access_grants
		SET status = $3, acknowledged_at = $4, updated_at = $4, version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status = 'active'
		RETURNING id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
	`, grantID, tenantID, GrantStatusAcknowledged, now).Scan(
		&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
		&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
		&acknowledgedAt, &returnedAt, &revokedAt,
		&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
		&g.Version, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGrantNotFound
	}
	if acknowledgedAt != nil {
		g.AcknowledgedAt = acknowledgedAt
	}
	if returnedAt != nil {
		g.ReturnedAt = returnedAt
	}
	if revokedAt != nil {
		g.RevokedAt = revokedAt
	}
	return &g, err
}

func (s *AccessStore) ReturnGrant(ctx context.Context, q querier, tenantID, grantID string) (*AccessGrant, error) {
	var g AccessGrant
	var acknowledgedAt, returnedAt, revokedAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE access_grants
		SET status = $3, returned_at = $4, updated_at = $4, version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND status IN ('active', 'acknowledged')
		RETURNING id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
	`, grantID, tenantID, GrantStatusReturned, now).Scan(
		&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
		&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
		&acknowledgedAt, &returnedAt, &revokedAt,
		&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
		&g.Version, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGrantNotFound
	}
	if acknowledgedAt != nil {
		g.AcknowledgedAt = acknowledgedAt
	}
	if returnedAt != nil {
		g.ReturnedAt = returnedAt
	}
	if revokedAt != nil {
		g.RevokedAt = revokedAt
	}
	return &g, err
}

func (s *AccessStore) RevokeGrant(ctx context.Context, q querier, tenantID, grantID, revokedBy, revokeReason string) (*AccessGrant, error) {
	var g AccessGrant
	var acknowledgedAt, returnedAt, revokedAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE access_grants
		SET status = $3, revoked_at = $4, revoked_by = $5, revoke_reason = $6,
			updated_at = $4, version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, property_id, secret_id, grantee_id, granter_id,
			window_start, window_end, reason, status,
			acknowledged_at, returned_at, revoked_at,
			revoked_by, revoke_reason, is_emergency, emergency_reason,
			version, created_at, updated_at
	`, grantID, tenantID, GrantStatusRevoked, now, revokedBy, revokeReason).Scan(
		&g.ID, &g.TenantID, &g.PropertyID, &g.SecretID, &g.GranteeID, &g.GranterID,
		&g.WindowStart, &g.WindowEnd, &g.Reason, &g.Status,
		&acknowledgedAt, &returnedAt, &revokedAt,
		&g.RevokedBy, &g.RevokeReason, &g.IsEmergency, &g.EmergencyReason,
		&g.Version, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGrantNotFound
	}
	if acknowledgedAt != nil {
		g.AcknowledgedAt = acknowledgedAt
	}
	if returnedAt != nil {
		g.ReturnedAt = returnedAt
	}
	if revokedAt != nil {
		g.RevokedAt = revokedAt
	}
	return &g, err
}

func (s *AccessStore) InsertDisclosure(ctx context.Context, q querier, d *AccessDisclosure) error {
	d.ID = newID("dis")
	_, err := q.Exec(ctx, `
		INSERT INTO access_disclosures (
			id, grant_id, tenant_id, property_id, secret_id,
			requestor_id, result, denial_reason, disclosed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, d.ID, d.GrantID, d.TenantID, d.PropertyID, d.SecretID,
		d.RequestorID, d.Result, d.DenialReason, d.DisclosedAt)
	return err
}

func (s *AccessStore) InsertCustodyEvent(ctx context.Context, q querier, ce *AccessCustodyEvent) error {
	ce.ID = newID("cev")
	_, err := q.Exec(ctx, `
		INSERT INTO access_custody_events (
			id, tenant_id, property_id, grant_id, secret_id,
			event_type, actor_id, grantee_id, reason, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, ce.ID, ce.TenantID, ce.PropertyID, nullString(ce.GrantID), nullString(ce.SecretID),
		ce.EventType, ce.ActorID, nullString(ce.GranteeID), ce.Reason, ce.Metadata)
	return err
}

func (s *AccessStore) ListCustodyEvents(ctx context.Context, tenantID, propertyID string) ([]AccessCustodyEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id,
			COALESCE(grant_id, ''), COALESCE(secret_id, ''),
			event_type, actor_id, COALESCE(grantee_id, ''),
			reason, metadata, created_at
		FROM access_custody_events
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY created_at DESC
		LIMIT 100
	`, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AccessCustodyEvent
	for rows.Next() {
		var ce AccessCustodyEvent
		if err := rows.Scan(
			&ce.ID, &ce.TenantID, &ce.PropertyID,
			&ce.GrantID, &ce.SecretID,
			&ce.EventType, &ce.ActorID, &ce.GranteeID,
			&ce.Reason, &ce.Metadata, &ce.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	return out, rows.Err()
}

func (s *AccessStore) HasActiveHold(ctx context.Context, tenantID, propertyID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM access_holds
		WHERE tenant_id = $1 AND property_id = $2 AND status = 'active'
	`, tenantID, propertyID).Scan(&count)
	return count > 0, err
}

func (s *AccessStore) InsertHold(ctx context.Context, q querier, hold *AccessHold) error {
	hold.ID = newID("hld")
	_, err := q.Exec(ctx, `
		INSERT INTO access_holds (
			id, tenant_id, property_id, reason, placed_by, status
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, hold.ID, hold.TenantID, hold.PropertyID, hold.Reason, hold.PlacedBy, hold.Status)
	return err
}

func (s *AccessStore) GetHold(ctx context.Context, tenantID, holdID string) (*AccessHold, error) {
	var h AccessHold
	var releasedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, reason, placed_by, status,
			released_at, released_by, created_at, updated_at
		FROM access_holds
		WHERE id = $1 AND tenant_id = $2
	`, holdID, tenantID).Scan(
		&h.ID, &h.TenantID, &h.PropertyID, &h.Reason, &h.PlacedBy, &h.Status,
		&releasedAt, &h.ReleasedBy, &h.CreatedAt, &h.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrHoldNotFound
	}
	if releasedAt != nil {
		h.ReleasedAt = releasedAt
	}
	return &h, err
}

func (s *AccessStore) ReleaseHold(ctx context.Context, q querier, tenantID, holdID, releasedBy string) (*AccessHold, error) {
	var h AccessHold
	var releasedAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE access_holds
		SET status = $3, released_at = $4, released_by = $5, updated_at = $4
		WHERE id = $1 AND tenant_id = $2 AND status = 'active'
		RETURNING id, tenant_id, property_id, reason, placed_by, status,
			released_at, released_by, created_at, updated_at
	`, holdID, tenantID, HoldStatusReleased, now, releasedBy).Scan(
		&h.ID, &h.TenantID, &h.PropertyID, &h.Reason, &h.PlacedBy, &h.Status,
		&releasedAt, &h.ReleasedBy, &h.CreatedAt, &h.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrHoldNotFound
	}
	if releasedAt != nil {
		h.ReleasedAt = releasedAt
	}
	return &h, err
}

func (s *AccessStore) ListDisclosures(ctx context.Context, tenantID, grantID string) ([]AccessDisclosure, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, grant_id, tenant_id, property_id, secret_id,
			requestor_id, result, denial_reason, disclosed_at
		FROM access_disclosures
		WHERE tenant_id = $1 AND grant_id = $2
		ORDER BY disclosed_at DESC
	`, tenantID, grantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AccessDisclosure
	for rows.Next() {
		var d AccessDisclosure
		if err := rows.Scan(
			&d.ID, &d.GrantID, &d.TenantID, &d.PropertyID, &d.SecretID,
			&d.RequestorID, &d.Result, &d.DenialReason, &d.DisclosedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
