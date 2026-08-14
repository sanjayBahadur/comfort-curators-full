package catalog

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

type CatalogStore struct {
	pool *pgxpool.Pool
}

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool}
}

const catalogItemColumns = `id, tenant_id, sku, name, category, brand, pack_size,
	unit_cost_minor_units, unit_cost_currency, owner_price_minor_units, owner_price_currency,
	tax_class, supplier, country_of_origin, status, shelf_life_rule,
	substitution_group, operational_suitability, label, version, created_at, updated_at`

func scanCatalogItem(row pgx.Row) (*CatalogItem, error) {
	var it CatalogItem
	err := row.Scan(
		&it.ID, &it.TenantID, &it.SKU, &it.Name, &it.Category, &it.Brand, &it.PackSize,
		&it.UnitCostMinorUnits, &it.UnitCostCurrency, &it.OwnerPriceMinorUnits, &it.OwnerPriceCurrency,
		&it.TaxClass, &it.Supplier, &it.CountryOfOrigin, &it.Status, &it.ShelfLifeRule,
		&it.SubstitutionGroup, &it.OperationalSuitability, &it.Label, &it.Version,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *CatalogStore) InsertCatalogItem(ctx context.Context, q querier, it *CatalogItem) error {
	it.ID = newID("cit")
	_, err := q.Exec(ctx, `
		INSERT INTO catalog_items (
			id, tenant_id, sku, name, category, brand, pack_size,
			unit_cost_minor_units, unit_cost_currency, owner_price_minor_units, owner_price_currency,
			tax_class, supplier, country_of_origin, status, shelf_life_rule,
			substitution_group, operational_suitability, label
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`, it.ID, it.TenantID, it.SKU, it.Name, it.Category, it.Brand, it.PackSize,
		it.UnitCostMinorUnits, it.UnitCostCurrency, it.OwnerPriceMinorUnits, it.OwnerPriceCurrency,
		it.TaxClass, it.Supplier, it.CountryOfOrigin, it.Status, it.ShelfLifeRule,
		it.SubstitutionGroup, it.OperationalSuitability, it.Label)
	return err
}

func (s *CatalogStore) GetCatalogItem(ctx context.Context, tenantID, id string) (*CatalogItem, error) {
	return scanCatalogItem(s.pool.QueryRow(ctx, `
		SELECT `+catalogItemColumns+`
		FROM catalog_items
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID))
}

func (s *CatalogStore) GetCatalogItemBySKU(ctx context.Context, tenantID, sku string) (*CatalogItem, error) {
	return scanCatalogItem(s.pool.QueryRow(ctx, `
		SELECT `+catalogItemColumns+`
		FROM catalog_items
		WHERE sku = $1 AND tenant_id = $2
	`, sku, tenantID))
}

func (s *CatalogStore) ListCatalogItems(ctx context.Context, tenantID string) ([]CatalogItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+catalogItemColumns+`
		FROM catalog_items
		WHERE tenant_id = $1
		ORDER BY sku ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CatalogItem
	for rows.Next() {
		it, err := scanCatalogItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

func (s *CatalogStore) InsertClaimEvidence(ctx context.Context, q querier, ev *ClaimEvidence) error {
	ev.ID = newID("evd")
	_, err := q.Exec(ctx, `
		INSERT INTO catalog_claim_evidence (
			id, tenant_id, catalog_item_id, claim_type,
			claim_statement, evidence_ref, evidence_retained_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, ev.ID, ev.TenantID, ev.CatalogItemID, ev.ClaimType,
		ev.ClaimStatement, ev.EvidenceRef, ev.EvidenceRetainedAt)
	return err
}

func (s *CatalogStore) ListClaimEvidence(ctx context.Context, tenantID, itemID string) ([]ClaimEvidence, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, catalog_item_id, claim_type,
			claim_statement, evidence_ref, evidence_retained_at, created_at
		FROM catalog_claim_evidence
		WHERE tenant_id = $1 AND catalog_item_id = $2
		ORDER BY created_at ASC
	`, tenantID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ClaimEvidence
	for rows.Next() {
		var ev ClaimEvidence
		if err := rows.Scan(
			&ev.ID, &ev.TenantID, &ev.CatalogItemID, &ev.ClaimType,
			&ev.ClaimStatement, &ev.EvidenceRef, &ev.EvidenceRetainedAt, &ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *CatalogStore) InsertPackageTemplate(ctx context.Context, q querier, tpl *PackageTemplate) error {
	tpl.ID = newID("tpl")
	items, err := json.Marshal(tpl.Items)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO package_templates (
			id, tenant_id, name, description, status, items
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, tpl.ID, tpl.TenantID, tpl.Name, tpl.Description, tpl.Status, items)
	return err
}

func (s *CatalogStore) GetPackageTemplate(ctx context.Context, tenantID, id string) (*PackageTemplate, error) {
	return scanPackageTemplate(s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, description, status, items, version, created_at, updated_at
		FROM package_templates
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID))
}

func (s *CatalogStore) ListPackageTemplates(ctx context.Context, tenantID string) ([]PackageTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, description, status, items, version, created_at, updated_at
		FROM package_templates
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PackageTemplate
	for rows.Next() {
		tpl, err := scanPackageTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *tpl)
	}
	return out, rows.Err()
}

func scanPackageTemplate(row pgx.Row) (*PackageTemplate, error) {
	var tpl PackageTemplate
	var itemsJSON []byte
	err := row.Scan(
		&tpl.ID, &tpl.TenantID, &tpl.Name, &tpl.Description, &tpl.Status, &itemsJSON,
		&tpl.Version, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(itemsJSON, &tpl.Items); err != nil {
		return nil, err
	}
	return &tpl, nil
}

type packageVersionRow struct {
	ID                              string
	TenantID                        string
	PropertyID                      string
	VersionNumber                   int
	Status                          string
	EffectiveDate                   time.Time
	MonthlyBudgetLimitMinorUnits    *int64
	SubstitutionPolicy              string
	RequireApprovalForPriceIncrease bool
	RequireApprovalForNewSKU        bool
	SetupCostMinorUnits             int64
	MonthlyCostMinorUnits           int64
	MonthlyConsumptionUnits         int64
	Currency                        string
	ReviewSummaryJSON               []byte
	CreatedBy                       string
	ActivatedAt                     *time.Time
	Version                         int
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

func (s *CatalogStore) InsertPackageVersion(ctx context.Context, q querier, v *PropertyPackageVersion) error {
	v.ID = newID("pkg")
	summary, err := json.Marshal(v.ReviewSummary)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO property_package_versions (
			id, tenant_id, property_id, version_number, status, effective_date,
			monthly_budget_limit_minor_units, substitution_policy,
			require_approval_for_price_increase, require_approval_for_new_sku,
			setup_cost_minor_units, monthly_cost_minor_units, monthly_consumption_units,
			currency, review_summary, created_by, activated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, v.ID, v.TenantID, v.PropertyID, v.VersionNumber, v.Status, v.EffectiveDate,
		v.MonthlyBudgetLimitMinorUnits, v.SubstitutionPolicy,
		v.RequireApprovalForPriceIncrease, v.RequireApprovalForNewSKU,
		v.SetupCostMinorUnits, v.MonthlyCostMinorUnits, v.MonthlyConsumptionUnits,
		v.Currency, summary, v.CreatedBy, nullTime(v.ActivatedAt))
	return err
}

func (s *CatalogStore) NextVersionNumber(ctx context.Context, q querier, tenantID, propertyID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM property_package_versions
		WHERE tenant_id = $1 AND property_id = $2
	`, tenantID, propertyID).Scan(&n)
	return n, err
}

func (s *CatalogStore) InsertPackageItem(ctx context.Context, q querier, it *PropertyPackageItem) error {
	it.ID = newID("pki")
	_, err := q.Exec(ctx, `
		INSERT INTO property_package_items (
			id, tenant_id, package_version_id, catalog_item_id, sku, name, label,
			substitution_group, quantity, order_index, expected_monthly_consumption,
			setup_cost_minor_units, monthly_cost_minor_units
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, it.ID, it.TenantID, it.PackageVersionID, it.CatalogItemID, it.SKU, it.Name, it.Label,
		it.SubstitutionGroup, it.Quantity, it.OrderIndex, it.ExpectedMonthlyConsumption,
		it.SetupCostMinorUnits, it.MonthlyCostMinorUnits)
	return err
}

func (s *CatalogStore) GetPackageVersion(ctx context.Context, tenantID, versionID string) (*PropertyPackageVersion, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, version_number, status, effective_date,
			monthly_budget_limit_minor_units, substitution_policy,
			require_approval_for_price_increase, require_approval_for_new_sku,
			setup_cost_minor_units, monthly_cost_minor_units, monthly_consumption_units,
			currency, review_summary, created_by, activated_at, version, created_at, updated_at
		FROM property_package_versions
		WHERE id = $1 AND tenant_id = $2
	`, versionID, tenantID)
	v, err := scanPackageVersion(row)
	if err != nil {
		return nil, err
	}
	items, err := s.ListPackageVersionItems(ctx, tenantID, v.ID)
	if err != nil {
		return nil, err
	}
	v.Items = items
	return v, nil
}

func (s *CatalogStore) GetPackageVersionForProperty(ctx context.Context, tenantID, propertyID, versionID string) (*PropertyPackageVersion, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, version_number, status, effective_date,
			monthly_budget_limit_minor_units, substitution_policy,
			require_approval_for_price_increase, require_approval_for_new_sku,
			setup_cost_minor_units, monthly_cost_minor_units, monthly_consumption_units,
			currency, review_summary, created_by, activated_at, version, created_at, updated_at
		FROM property_package_versions
		WHERE id = $1 AND tenant_id = $2 AND property_id = $3
	`, versionID, tenantID, propertyID)
	v, err := scanPackageVersion(row)
	if err != nil {
		return nil, err
	}
	items, err := s.ListPackageVersionItems(ctx, tenantID, v.ID)
	if err != nil {
		return nil, err
	}
	v.Items = items
	return v, nil
}

func (s *CatalogStore) ListPackageVersions(ctx context.Context, tenantID, propertyID string) ([]PropertyPackageVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, version_number, status, effective_date,
			monthly_budget_limit_minor_units, substitution_policy,
			require_approval_for_price_increase, require_approval_for_new_sku,
			setup_cost_minor_units, monthly_cost_minor_units, monthly_consumption_units,
			currency, review_summary, created_by, activated_at, version, created_at, updated_at
		FROM property_package_versions
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY version_number ASC
	`, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PropertyPackageVersion
	for rows.Next() {
		v, err := scanPackageVersion(rows)
		if err != nil {
			return nil, err
		}
		items, err := s.ListPackageVersionItems(ctx, tenantID, v.ID)
		if err != nil {
			return nil, err
		}
		v.Items = items
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (s *CatalogStore) ListPackageVersionItems(ctx context.Context, tenantID, packageVersionID string) ([]PropertyPackageItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, package_version_id, catalog_item_id, sku, name, label,
			substitution_group, quantity, order_index, expected_monthly_consumption,
			setup_cost_minor_units, monthly_cost_minor_units, created_at
		FROM property_package_items
		WHERE tenant_id = $1 AND package_version_id = $2
		ORDER BY order_index ASC
	`, tenantID, packageVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PropertyPackageItem
	for rows.Next() {
		var it PropertyPackageItem
		if err := rows.Scan(
			&it.ID, &it.TenantID, &it.PackageVersionID, &it.CatalogItemID, &it.SKU, &it.Name, &it.Label,
			&it.SubstitutionGroup, &it.Quantity, &it.OrderIndex, &it.ExpectedMonthlyConsumption,
			&it.SetupCostMinorUnits, &it.MonthlyCostMinorUnits, &it.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanPackageVersion(row pgx.Row) (*PropertyPackageVersion, error) {
	var v PropertyPackageVersion
	var summaryJSON []byte
	err := row.Scan(
		&v.ID, &v.TenantID, &v.PropertyID, &v.VersionNumber, &v.Status, &v.EffectiveDate,
		&v.MonthlyBudgetLimitMinorUnits, &v.SubstitutionPolicy,
		&v.RequireApprovalForPriceIncrease, &v.RequireApprovalForNewSKU,
		&v.SetupCostMinorUnits, &v.MonthlyCostMinorUnits, &v.MonthlyConsumptionUnits,
		&v.Currency, &summaryJSON, &v.CreatedBy, &v.ActivatedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPackageVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(summaryJSON, &v.ReviewSummary); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *CatalogStore) ActivatePackageVersion(ctx context.Context, q querier, tenantID, propertyID, versionID string, activatedAt time.Time) (*PropertyPackageVersion, error) {
	row := q.QueryRow(ctx, `
		UPDATE property_package_versions
		SET status = $4, activated_at = $5, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND property_id = $3 AND status = 'draft'
		RETURNING id, tenant_id, property_id, version_number, status, effective_date,
			monthly_budget_limit_minor_units, substitution_policy,
			require_approval_for_price_increase, require_approval_for_new_sku,
			setup_cost_minor_units, monthly_cost_minor_units, monthly_consumption_units,
			currency, review_summary, created_by, activated_at, version, created_at, updated_at
	`, versionID, tenantID, propertyID, PackageStatusActive, activatedAt)
	v, err := scanPackageVersion(row)
	if err != nil {
		if errors.Is(err, ErrPackageVersionNotFound) {
			return nil, ErrPackageVersionNotDraft
		}
		return nil, err
	}
	items, err := s.ListPackageVersionItems(ctx, tenantID, v.ID)
	if err != nil {
		return nil, err
	}
	v.Items = items
	return v, nil
}

func (s *CatalogStore) RejectPackageVersion(ctx context.Context, q querier, tenantID, propertyID, versionID string) (*PropertyPackageVersion, error) {
	row := q.QueryRow(ctx, `
		UPDATE property_package_versions
		SET status = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND property_id = $3 AND status = 'draft'
		RETURNING id, tenant_id, property_id, version_number, status, effective_date,
			monthly_budget_limit_minor_units, substitution_policy,
			require_approval_for_price_increase, require_approval_for_new_sku,
			setup_cost_minor_units, monthly_cost_minor_units, monthly_consumption_units,
			currency, review_summary, created_by, activated_at, version, created_at, updated_at
	`, versionID, tenantID, propertyID, PackageStatusRejected)
	v, err := scanPackageVersion(row)
	if err != nil {
		if errors.Is(err, ErrPackageVersionNotFound) {
			return nil, ErrPackageVersionNotDraft
		}
		return nil, err
	}
	items, err := s.ListPackageVersionItems(ctx, tenantID, v.ID)
	if err != nil {
		return nil, err
	}
	v.Items = items
	return v, nil
}

func (s *CatalogStore) SupersedeActiveVersions(ctx context.Context, q querier, tenantID, propertyID string) error {
	_, err := q.Exec(ctx, `
		UPDATE property_package_versions
		SET status = $3, updated_at = NOW(), version = version + 1
		WHERE tenant_id = $1 AND property_id = $2 AND status = 'active'
	`, tenantID, propertyID, PackageStatusSuperseded)
	return err
}

func nullTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
