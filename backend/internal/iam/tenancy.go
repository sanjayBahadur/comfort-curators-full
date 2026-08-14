package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/security"
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type TenancyService struct {
	pool       *pgxpool.Pool
	auditStore *audit.AuditStore
}

func NewTenancyService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *TenancyService {
	return &TenancyService{
		pool:       pool,
		auditStore: auditStore,
	}
}

func (s *TenancyService) CreateTenant(ctx context.Context, params CreateTenantParams) (*Tenant, error) {
	if params.Name == "" {
		return nil, fmt.Errorf("tenant name is required")
	}

	var tenant Tenant
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO tenants (name, state, created_at, updated_at)
			VALUES ($1, 'active', NOW(), NOW())
			RETURNING id, name, state, created_at, updated_at
		`, params.Name).Scan(&tenant.ID, &tenant.Name, &tenant.State, &tenant.CreatedAt, &tenant.UpdatedAt); err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenant.ID,
			ActorID:      "system",
			Action:       "tenant.create",
			ResourceType: "tenant",
			ResourceID:   tenant.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	return &tenant, nil
}

func (s *TenancyService) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	var tenant Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, state, created_at, updated_at
		FROM tenants
		WHERE id = $1
	`, tenantID).Scan(&tenant.ID, &tenant.Name, &tenant.State, &tenant.CreatedAt, &tenant.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &tenant, nil
}

func (s *TenancyService) EnsureTenant(ctx context.Context, params CreateTenantParams) (*Tenant, error) {
	var existing Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, state, created_at, updated_at
		FROM tenants WHERE name = $1
	`, params.Name).Scan(&existing.ID, &existing.Name, &existing.State, &existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		return &existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("lookup tenant: %w", err)
	}
	return s.CreateTenant(ctx, params)
}

func (s *TenancyService) AddMembership(ctx context.Context, tenantID, userID, role string) (*Membership, error) {
	if !ValidRole(role) {
		return nil, ErrRoleNotAllowed
	}

	_, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var membership Membership
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO memberships (tenant_id, user_id, role, created_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = $3
			RETURNING id, tenant_id, user_id, role, created_at
		`, tenantID, userID, role).Scan(
			&membership.ID, &membership.TenantID, &membership.UserID, &membership.Role, &membership.CreatedAt,
		); err != nil {
			return fmt.Errorf("add membership: %w", err)
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      userID,
			Action:       "membership.add",
			ResourceType: "membership",
			ResourceID:   membership.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	return &membership, nil
}

func (s *TenancyService) RemoveMembership(ctx context.Context, tenantID, userID string) error {
	return database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM memberships WHERE tenant_id = $1 AND user_id = $2
		`, tenantID, userID)
		if err != nil {
			return fmt.Errorf("remove membership: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrMembershipNotFound
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      userID,
			Action:       "membership.remove",
			ResourceType: "membership",
		})
	})
}

func (s *TenancyService) IsMember(ctx context.Context, tenantID, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM memberships WHERE tenant_id = $1 AND user_id = $2)
	`, tenantID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return exists, nil
}

func (s *TenancyService) GetMemberships(ctx context.Context, userID string) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()

	var memberships []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, nil
}

func (s *TenancyService) CreateSupportAccessGrant(ctx context.Context, params CreateSupportAccessGrantParams) (*SupportAccessGrant, error) {
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if params.GrantedToUserID == "" {
		return nil, fmt.Errorf("granted_to_user_id is required")
	}
	if params.Reason == "" {
		return nil, fmt.Errorf("reason is required")
	}
	if params.TTL <= 0 {
		params.TTL = 1 * time.Hour
	}

	scope := params.Scope
	if scope == "" {
		scope = SupportAccessScopeTenant
	}

	expiresAt := time.Now().UTC().Add(params.TTL)

	var grant SupportAccessGrant
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO support_access_grants (tenant_id, granted_by_user_id, granted_to_user_id, reason, scope, expires_at, active, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, true, NOW())
			RETURNING id, tenant_id, granted_by_user_id, granted_to_user_id, reason, scope, expires_at, active, created_at
		`, params.TenantID, params.GrantedByUserID, params.GrantedToUserID, params.Reason, scope, expiresAt).Scan(
			&grant.ID, &grant.TenantID, &grant.GrantedByUserID, &grant.GrantedToUserID,
			&grant.Reason, &grant.Scope, &grant.ExpiresAt, &grant.Active, &grant.CreatedAt,
		); err != nil {
			return fmt.Errorf("create support access grant: %w", err)
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeSecurity,
			TenantID:     params.TenantID,
			ActorID:      params.GrantedByUserID,
			Action:       "support_access.grant",
			ResourceType: "support_access_grant",
			ResourceID:   grant.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	return &grant, nil
}

func (s *TenancyService) RevokeSupportAccessGrant(ctx context.Context, grantID, revokedBy string) error {
	return database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE support_access_grants SET active = false WHERE id = $1 AND active = true
		`, grantID)
		if err != nil {
			return fmt.Errorf("revoke support access grant: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSupportAccessNotFound
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeSecurity,
			ActorID:      revokedBy,
			Action:       "support_access.revoke",
			ResourceType: "support_access_grant",
			ResourceID:   grantID,
		})
	})
}

func (s *TenancyService) ValidateSupportAccess(ctx context.Context, granteeUserID, targetTenantID string) error {
	return s.validateSupportAccess(ctx, s.pool, granteeUserID, targetTenantID)
}

func (s *TenancyService) validateSupportAccess(ctx context.Context, q querier, granteeUserID, targetTenantID string) error {
	var grant SupportAccessGrant
	err := q.QueryRow(ctx, `
		SELECT id, tenant_id, granted_by_user_id, granted_to_user_id, reason, scope, expires_at, active, created_at
		FROM support_access_grants
		WHERE granted_to_user_id = $1 AND tenant_id = $2 AND active = true
		ORDER BY created_at DESC
		LIMIT 1
	`, granteeUserID, targetTenantID).Scan(
		&grant.ID, &grant.TenantID, &grant.GrantedByUserID, &grant.GrantedToUserID,
		&grant.Reason, &grant.Scope, &grant.ExpiresAt, &grant.Active, &grant.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrSupportAccessNotFound
		}
		return fmt.Errorf("lookup support access: %w", err)
	}

	if !grant.Active {
		return ErrSupportAccessRevoked
	}

	if time.Now().UTC().After(grant.ExpiresAt) {
		return ErrSupportAccessExpired
	}

	if err := s.auditStore.AppendTx(ctx, q, audit.AuditEvent{
		ID:           newID("aud"),
		EventType:    audit.EventTypeAccess,
		TenantID:     targetTenantID,
		ActorID:      granteeUserID,
		Action:       "support_access.use",
		ResourceType: "support_access_grant",
		ResourceID:   grant.ID,
	}); err != nil {
		return err
	}

	return nil
}

func (s *TenancyService) RequireTenantScope(ctx context.Context, resourceTenantID string) error {
	subject, ok := SubjectFromContext(ctx)
	if !ok {
		return ErrCrossTenantDenied
	}

	if subject.TenantID == "" {
		return ErrCrossTenantDenied
	}

	if subject.TenantID == resourceTenantID {
		return nil
	}

	if err := s.ValidateSupportAccess(ctx, subject.ActorID, resourceTenantID); err != nil {
		return ErrCrossTenantDenied
	}

	return nil
}

func (s *TenancyService) RequireResourceAccess(ctx context.Context, resourceTenantID string, resourceType string, resourceID string) error {
	subject, ok := SubjectFromContext(ctx)
	if !ok {
		s.appendAudit(ctx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     resourceTenantID,
			ActorID:      "anonymous",
			Action:       "access.denied",
			ResourceType: resourceType,
			ResourceID:   resourceID,
		})
		return ErrCrossTenantDenied
	}

	if subject.TenantID == "" {
		s.appendAudit(ctx, audit.AuditEvent{
			EventType:    audit.EventTypeAccess,
			TenantID:     resourceTenantID,
			ActorID:      subject.ActorID,
			Action:       "access.denied",
			ResourceType: resourceType,
			ResourceID:   resourceID,
		})
		return ErrCrossTenantDenied
	}

	if subject.TenantID == resourceTenantID {
		return nil
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		return s.validateSupportAccess(ctx, tx, subject.ActorID, resourceTenantID)
	})
	if err != nil {
		s.appendAudit(ctx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeAccess,
			TenantID:     resourceTenantID,
			ActorID:      subject.ActorID,
			Action:       "access.denied",
			ResourceType: resourceType,
			ResourceID:   resourceID,
		})
		return ErrCrossTenantDenied
	}

	return nil
}

func (s *TenancyService) RequireTenantExistence(ctx context.Context, tenantID string) error {
	_, err := s.GetTenant(ctx, tenantID)
	if err == ErrTenantNotFound {
		return ErrTenantNotFound
	}
	return err
}

func (s *TenancyService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if evt.ID == "" {
		evt.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, evt); err != nil {
		return
	}
}

func RequireTenantMatch(subject security.Subject, resourceTenantID string) error {
	if subject.TenantID == "" {
		return ErrCrossTenantDenied
	}
	if subject.TenantID != resourceTenantID {
		return ErrCrossTenantDenied
	}
	return nil
}

func RequireRoleMatch(subject security.Subject, requiredRole string) error {
	for _, r := range subject.Roles {
		if r == requiredRole {
			return nil
		}
	}
	return ErrRoleNotAllowed
}

func ValidateAttributePolicy(policy AttributePolicy, subject security.Subject) error {
	if policy.TenantID != nil && subject.TenantID != *policy.TenantID {
		return ErrCrossTenantDenied
	}

	if policy.UserID != nil && subject.ActorID != *policy.UserID {
		return ErrCrossTenantDenied
	}

	if policy.Role != nil {
		found := false
		for _, r := range subject.Roles {
			if r == *policy.Role {
				found = true
				break
			}
		}
		if !found {
			return ErrRoleNotAllowed
		}
	}

	if policy.TimeWindow != nil {
		now := time.Now().UTC()
		if policy.TimeWindow.After != nil && now.Before(*policy.TimeWindow.After) {
			return fmt.Errorf("access not allowed before time window")
		}
		if policy.TimeWindow.Before != nil && now.After(*policy.TimeWindow.Before) {
			return fmt.Errorf("access not allowed after time window")
		}
	}

	return nil
}
