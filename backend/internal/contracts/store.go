package contracts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateAgreement inserts the agreement and its first immutable version inside
// the caller's transaction so the aggregate, version, and the caller's audit
// entry commit atomically.
func (s *Store) CreateAgreement(ctx context.Context, q querier, a *Agreement, terms []byte, now time.Time) (*AgreementVersion, error) {
	if a.ID == "" {
		a.ID = newID("agree")
	}
	if a.Status == "" {
		a.Status = AgreementStatusDraft
	}
	if a.Version == 0 {
		a.Version = 1
	}

	contentHash, err := HashTerms(terms)
	if err != nil {
		return nil, err
	}
	a.CurrentVersion = 1
	version := &AgreementVersion{
		AgreementID:   a.ID,
		TenantID:      a.TenantID,
		VersionNumber: 1,
		ContentHash:   contentHash,
		Terms:         terms,
		CreatedAt:     now,
	}
	if version.ID == "" {
		version.ID = newID("agreever")
	}

	if err := insertAgreement(ctx, q, a); err != nil {
		return nil, err
	}
	if err := insertAgreementVersion(ctx, q, version); err != nil {
		return nil, err
	}
	a.Versions = append(a.Versions, *version)
	return version, nil
}

// GetAgreement returns the full agreement aggregate with every immutable
// version and the acceptance record. Reads verify every version content hash
// against its terms so accepted terms cannot silently drift.
func (s *Store) GetAgreement(ctx context.Context, tenantID, agreementID string) (*Agreement, error) {
	a, err := s.getByID(ctx, s.pool, tenantID, agreementID)
	if err != nil {
		return nil, err
	}
	if err := a.VerifyContentHash(); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) getByID(ctx context.Context, q querier, tenantID, agreementID string) (*Agreement, error) {
	var a Agreement
	err := q.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, status, current_version, version, created_at, updated_at
		FROM service_contracts
		WHERE id = $1 AND tenant_id = $2
	`, agreementID, tenantID).Scan(
		&a.ID, &a.TenantID, &a.PropertyID, &a.Status, &a.CurrentVersion, &a.Version,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrAgreementNotFound
		}
		return nil, fmt.Errorf("get service agreement: %w", err)
	}

	if a.Versions, err = s.listVersions(ctx, q, tenantID, agreementID); err != nil {
		return nil, err
	}
	if a.Acceptance, err = s.getAcceptance(ctx, q, tenantID, agreementID); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) listVersions(ctx context.Context, q querier, tenantID, agreementID string) ([]AgreementVersion, error) {
	rows, err := q.Query(ctx, `
		SELECT id, agreement_id, tenant_id, version_number, content_hash, terms, created_at
		FROM service_contract_versions
		WHERE agreement_id = $1 AND tenant_id = $2
		ORDER BY version_number ASC
	`, agreementID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list service agreement versions: %w", err)
	}
	defer rows.Close()

	var versions []AgreementVersion
	for rows.Next() {
		var v AgreementVersion
		if err := rows.Scan(
			&v.ID, &v.AgreementID, &v.TenantID, &v.VersionNumber, &v.ContentHash, &v.Terms, &v.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan service agreement version: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (s *Store) getAcceptance(ctx context.Context, q querier, tenantID, agreementID string) (*ContractAcceptance, error) {
	var acc ContractAcceptance
	err := q.QueryRow(ctx, `
		SELECT id, agreement_id, tenant_id, version_number, content_hash, accepted_by, accepted_at
		FROM contract_acceptances
		WHERE agreement_id = $1 AND tenant_id = $2
	`, agreementID, tenantID).Scan(
		&acc.ID, &acc.AgreementID, &acc.TenantID, &acc.VersionNumber, &acc.ContentHash,
		&acc.AcceptedBy, &acc.AcceptedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get contract acceptance: %w", err)
	}
	return &acc, nil
}

func (s *Store) ListAgreements(ctx context.Context, tenantID string) ([]Agreement, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, status, current_version, version, created_at, updated_at
		FROM service_contracts
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list service agreements: %w", err)
	}
	defer rows.Close()

	var agreements []Agreement
	for rows.Next() {
		var a Agreement
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.PropertyID, &a.Status, &a.CurrentVersion, &a.Version,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan service agreement: %w", err)
		}
		agreements = append(agreements, a)
	}
	return agreements, rows.Err()
}

// AddVersion persists a new immutable version of a draft agreement and bumps
// the aggregate using an optimistic concurrency check. The caller performs the
// accepted-agreement immutability check in the same transaction.
func (s *Store) AddVersion(ctx context.Context, q querier, a *Agreement, version *AgreementVersion) error {
	if err := insertAgreementVersion(ctx, q, version); err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE service_contracts
		SET current_version = $3, version = $4, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND version = $5
	`, a.ID, a.TenantID, a.CurrentVersion, a.Version, a.Version-1)
	if err != nil {
		return fmt.Errorf("update service agreement version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("service agreement update lost a concurrent write (optimistic version)")
	}
	return nil
}

// Accept persists the acceptance record and marks the agreement accepted with
// an optimistic concurrency check, atomically in the caller's transaction.
func (s *Store) Accept(ctx context.Context, q querier, a *Agreement, acc *ContractAcceptance) error {
	if acc.ID == "" {
		acc.ID = newID("accept")
	}
	if acc.AcceptedAt.IsZero() {
		acc.AcceptedAt = time.Now().UTC()
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO contract_acceptances (
			id, agreement_id, tenant_id, version_number, content_hash, accepted_by, accepted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, acc.ID, acc.AgreementID, acc.TenantID, acc.VersionNumber, acc.ContentHash, acc.AcceptedBy, acc.AcceptedAt); err != nil {
		return fmt.Errorf("insert contract acceptance: %w", err)
	}

	tag, err := q.Exec(ctx, `
		UPDATE service_contracts
		SET status = $3, version = $4, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND version = $5
	`, a.ID, a.TenantID, string(a.Status), a.Version, a.Version-1)
	if err != nil {
		return fmt.Errorf("accept service agreement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("service agreement acceptance lost a concurrent write (optimistic version)")
	}
	return nil
}

// SaveFeeRule persists a versioned fee rule. Fee rules are global reference
// data seeded by the operator; the unique key is (rule_version, currency,
// service_tier).
func (s *Store) SaveFeeRule(ctx context.Context, rule *FeeRule) error {
	if rule.ID == "" {
		rule.ID = newID("feerule")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO fee_rules (
			id, rule_version, currency, service_tier, percentage_basis_points,
			minimum_monthly_fee_minor_units, setup_fee_minor_units,
			effective_from, effective_to, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, rule.ID, rule.Version, rule.Currency, rule.ServiceTier, rule.PercentageBasisPoints,
		rule.MinimumMonthlyFeeMinorUnits, rule.SetupFeeMinorUnits, nilString(rule.EffectiveFrom), nilString(rule.EffectiveTo))
	if err != nil {
		return fmt.Errorf("save fee rule: %w", err)
	}
	return nil
}

// GetFeeRule loads the fee rule for the exact rule version, currency and
// service tier.
func (s *Store) GetFeeRule(ctx context.Context, ruleVersion, currency, serviceTier string) (*FeeRule, error) {
	var rule FeeRule
	var effectiveFrom, effectiveTo *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, rule_version, currency, service_tier, percentage_basis_points,
			minimum_monthly_fee_minor_units, setup_fee_minor_units,
			effective_from, effective_to, created_at
		FROM fee_rules
		WHERE rule_version = $1 AND currency = $2 AND service_tier = $3
	`, ruleVersion, currency, serviceTier).Scan(
		&rule.ID, &rule.Version, &rule.Currency, &rule.ServiceTier, &rule.PercentageBasisPoints,
		&rule.MinimumMonthlyFeeMinorUnits, &rule.SetupFeeMinorUnits,
		&effectiveFrom, &effectiveTo, &rule.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrFeeRuleNotFound
		}
		return nil, fmt.Errorf("get fee rule: %w", err)
	}
	if effectiveFrom != nil {
		rule.EffectiveFrom = *effectiveFrom
	}
	if effectiveTo != nil {
		rule.EffectiveTo = *effectiveTo
	}
	return &rule, nil
}

func insertAgreement(ctx context.Context, q querier, a *Agreement) error {
	if _, err := q.Exec(ctx, `
		INSERT INTO service_contracts (
			id, tenant_id, property_id, status, current_version, version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
	`, a.ID, a.TenantID, a.PropertyID, string(a.Status), a.CurrentVersion, a.Version, a.CreatedAt); err != nil {
		return fmt.Errorf("insert service agreement: %w", err)
	}
	return nil
}

func insertAgreementVersion(ctx context.Context, q querier, v *AgreementVersion) error {
	if v.ID == "" {
		v.ID = newID("agreever")
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO service_contract_versions (
			id, agreement_id, tenant_id, version_number, content_hash, terms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, v.ID, v.AgreementID, v.TenantID, v.VersionNumber, v.ContentHash, v.Terms, v.CreatedAt); err != nil {
		return fmt.Errorf("insert service agreement version: %w", err)
	}
	return nil
}

func nilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
