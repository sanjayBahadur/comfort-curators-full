package catalog

import (
	"context"
	"fmt"
	"sort"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultCurrency is the V0 launch-currency for catalog pricing (INR, integer
// minor units).
const DefaultCurrency = "INR"

type Service struct {
	pool       *pgxpool.Pool
	store      *CatalogStore
	auditStore *audit.AuditStore
	authorizer ResourceAuthorizer
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:       pool,
		store:      NewCatalogStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *Service) WithAuthorizer(a ResourceAuthorizer) *Service {
	s.authorizer = a
	return s
}

func (s *Service) WithAudit(a *audit.AuditStore) *Service {
	s.auditStore = a
	return s
}

func (s *Service) authorizeProperty(ctx context.Context, tenantID, propertyID string) error {
	if s.authorizer == nil {
		return ErrCrossTenantDenied
	}
	if err := s.authorizer.RequireResourceAccess(ctx, tenantID, "property", propertyID); err != nil {
		return ErrCrossTenantDenied
	}
	return nil
}

func (s *Service) appendAudit(ctx context.Context, event audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, event); err != nil {
		logging.Error(ctx, "failed to append audit event", "error", err)
	}
}

func normalizeCurrency(currency string) string {
	if currency == "" {
		return DefaultCurrency
	}
	return currency
}

// CreateCatalogItem registers a catalog item with its full SKU profile
// (CAT-001). The operational label (CAT-002) is validated; the sponsored label
// is refused because sponsored placement remains disabled (CAT-003, V0 scope).
func (s *Service) CreateCatalogItem(ctx context.Context, tenantID string, params CreateItemParams, actorID string) (*CatalogItem, error) {
	if params.SKU == "" || params.Name == "" || params.Category == "" {
		return nil, fmt.Errorf("%w: sku, name, and category are required", ErrInvalidItem)
	}
	if params.UnitCostMinorUnits < 0 || params.OwnerPriceMinorUnits < 0 {
		return nil, fmt.Errorf("%w: costs must not be negative", ErrInvalidItem)
	}

	unitCurrency := normalizeCurrency(params.UnitCostCurrency)
	priceCurrency := normalizeCurrency(params.OwnerPriceCurrency)
	if !ValidCurrency(unitCurrency) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCurrency, unitCurrency)
	}
	if !ValidCurrency(priceCurrency) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCurrency, priceCurrency)
	}
	if unitCurrency != priceCurrency {
		return nil, fmt.Errorf("%w: unit cost and owner price currencies must match", ErrInvalidItem)
	}
	if !ValidLabel(params.Label) {
		if params.Label == LabelSponsored {
			return nil, ErrSponsoredDisabled
		}
		return nil, fmt.Errorf("%w: %q", ErrInvalidLabel, params.Label)
	}
	status := params.Status
	if status == "" {
		status = ItemStatusActive
	}
	if status != ItemStatusActive && status != ItemStatusDisabled {
		return nil, fmt.Errorf("%w: %q", ErrInvalidItem, status)
	}

	item := &CatalogItem{
		TenantID:               tenantID,
		SKU:                    params.SKU,
		Name:                   params.Name,
		Category:               params.Category,
		Brand:                  params.Brand,
		PackSize:               params.PackSize,
		UnitCostMinorUnits:     params.UnitCostMinorUnits,
		UnitCostCurrency:       unitCurrency,
		OwnerPriceMinorUnits:   params.OwnerPriceMinorUnits,
		OwnerPriceCurrency:     priceCurrency,
		TaxClass:               params.TaxClass,
		Supplier:               params.Supplier,
		CountryOfOrigin:        params.CountryOfOrigin,
		Status:                 status,
		ShelfLifeRule:          params.ShelfLifeRule,
		SubstitutionGroup:      params.SubstitutionGroup,
		OperationalSuitability: params.OperationalSuitability,
		Label:                  params.Label,
		Version:                1,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCatalogItem(ctx, tx, item); err != nil {
			if isUniqueViolation(err) {
				return ErrSKUAlreadyExists
			}
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "catalog.item.created",
			ResourceType: "catalog_item",
			ResourceID:   item.ID,
			NewState:     marshalJSON(item),
		})
	})
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (s *Service) GetCatalogItem(ctx context.Context, tenantID, itemID string) (*CatalogItem, error) {
	return s.store.GetCatalogItem(ctx, tenantID, itemID)
}

func (s *Service) ListCatalogItems(ctx context.Context, tenantID string) ([]CatalogItem, error) {
	return s.store.ListCatalogItems(ctx, tenantID)
}

// AddClaimEvidence records the retained source evidence behind a quality,
// sustainability, performance, or origin claim (CAT-010). Evidence references
// are required and the records are append-only: they are never updated or
// deleted, so retained evidence cannot be lost or rewritten.
func (s *Service) AddClaimEvidence(ctx context.Context, tenantID, itemID string, params ClaimEvidenceParams, actorID string) (*ClaimEvidence, error) {
	if !ValidClaimType(params.ClaimType) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidClaimType, params.ClaimType)
	}
	if params.EvidenceRef == "" {
		return nil, ErrClaimEvidenceRequired
	}

	if _, err := s.store.GetCatalogItem(ctx, tenantID, itemID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	evidence := &ClaimEvidence{
		TenantID:           tenantID,
		CatalogItemID:      itemID,
		ClaimType:          params.ClaimType,
		ClaimStatement:     params.ClaimStatement,
		EvidenceRef:        params.EvidenceRef,
		EvidenceRetainedAt: now,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertClaimEvidence(ctx, tx, evidence); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "catalog.claim_evidence.retained",
			ResourceType: "catalog_claim_evidence",
			ResourceID:   evidence.ID,
			NewState:     marshalJSON(evidence),
		})
	})
	if err != nil {
		return nil, err
	}

	return evidence, nil
}

func (s *Service) ListClaimEvidence(ctx context.Context, tenantID, itemID string) ([]ClaimEvidence, error) {
	if _, err := s.store.GetCatalogItem(ctx, tenantID, itemID); err != nil {
		return nil, err
	}
	return s.store.ListClaimEvidence(ctx, tenantID, itemID)
}

// CreatePackageTemplate registers a reusable bundle (CAT-004). Every bundle
// item references an existing catalog item in the same tenant.
func (s *Service) CreatePackageTemplate(ctx context.Context, tenantID string, params CreateTemplateParams, actorID string) (*PackageTemplate, error) {
	if params.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidTemplate)
	}
	if len(params.Items) == 0 {
		return nil, ErrNoPackageItems
	}

	seen := map[string]bool{}
	for _, it := range params.Items {
		if it.Quantity <= 0 {
			return nil, fmt.Errorf("%w: bundle item quantity must be positive", ErrInvalidTemplate)
		}
		catalogItem, err := s.store.GetCatalogItem(ctx, tenantID, it.CatalogItemID)
		if err != nil {
			return nil, err
		}
		if seen[catalogItem.SKU] {
			return nil, ErrDuplicatePackageSKU
		}
		seen[catalogItem.SKU] = true
	}

	tpl := &PackageTemplate{
		TenantID:    tenantID,
		Name:        params.Name,
		Description: params.Description,
		Status:      ItemStatusActive,
		Items:       params.Items,
		Version:     1,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertPackageTemplate(ctx, tx, tpl); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "catalog.package_template.created",
			ResourceType: "package_template",
			ResourceID:   tpl.ID,
			NewState:     marshalJSON(tpl),
		})
	})
	if err != nil {
		return nil, err
	}

	return tpl, nil
}

func (s *Service) GetPackageTemplate(ctx context.Context, tenantID, templateID string) (*PackageTemplate, error) {
	return s.store.GetPackageTemplate(ctx, tenantID, templateID)
}

func (s *Service) ListPackageTemplates(ctx context.Context, tenantID string) ([]PackageTemplate, error) {
	return s.store.ListPackageTemplates(ctx, tenantID)
}

type resolvedItem struct {
	item        *CatalogItem
	quantity    int
	consumption int
	order       int
}

// CreatePropertyPackageVersion creates a new draft package version for a
// property. Changes are always versioned (CAT-008): the version number is the
// next sequence number for the property and prior versions are retained. The
// computed review summary with one-time setup cost, estimated monthly
// consumption, estimated monthly cost, and substitution behavior is stored with
// the version and returned, so the owner sees cost and substitution before the
// version can be activated (CAT-005, CAT-009).
func (s *Service) CreatePropertyPackageVersion(ctx context.Context, tenantID, propertyID string, params CreatePackageVersionParams, actorID string) (*PropertyPackageVersion, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	if params.EffectiveDate.IsZero() {
		return nil, ErrEffectiveDateRequired
	}
	policy := params.SubstitutionPolicy
	if policy == "" {
		policy = SubstitutionOwnerApproval
	}
	if !ValidSubstitutionPolicy(policy) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSubstitutionPolicy, policy)
	}
	if params.MonthlyBudgetLimitMinorUnits != nil && *params.MonthlyBudgetLimitMinorUnits < 0 {
		return nil, fmt.Errorf("%w: monthly budget must not be negative", ErrInvalidPackageVersion)
	}
	if len(params.Items) == 0 && len(params.Bundles) == 0 {
		return nil, ErrNoPackageItems
	}

	lines, err := s.resolvePackageLines(ctx, tenantID, params)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, ErrNoPackageItems
	}

	reviewItems, currency, err := buildReviewItems(lines)
	if err != nil {
		return nil, err
	}
	summary, err := BuildReviewSummary(policy, params.MonthlyBudgetLimitMinorUnits,
		params.RequireApprovalForPriceIncrease, params.RequireApprovalForNewSKU,
		reviewItems, currency)
	if err != nil {
		return nil, err
	}

	version := &PropertyPackageVersion{
		TenantID:                        tenantID,
		PropertyID:                      propertyID,
		Status:                          PackageStatusDraft,
		EffectiveDate:                   params.EffectiveDate,
		MonthlyBudgetLimitMinorUnits:    params.MonthlyBudgetLimitMinorUnits,
		SubstitutionPolicy:              policy,
		RequireApprovalForPriceIncrease: params.RequireApprovalForPriceIncrease,
		RequireApprovalForNewSKU:        params.RequireApprovalForNewSKU,
		SetupCostMinorUnits:             summary.SetupCostMinorUnits,
		MonthlyCostMinorUnits:           summary.MonthlyCostMinorUnits,
		MonthlyConsumptionUnits:         summary.MonthlyConsumptionUnits,
		Currency:                        currency,
		ReviewSummary:                   summary,
		CreatedBy:                       actorID,
		Version:                         1,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		n, err := s.store.NextVersionNumber(ctx, tx, tenantID, propertyID)
		if err != nil {
			return err
		}
		version.VersionNumber = n

		if err := s.store.InsertPackageVersion(ctx, tx, version); err != nil {
			return err
		}
		for _, line := range lines {
			item := &PropertyPackageItem{
				TenantID:                   tenantID,
				PackageVersionID:           version.ID,
				CatalogItemID:              line.item.ID,
				SKU:                        line.item.SKU,
				Name:                       line.item.Name,
				Label:                      line.item.Label,
				SubstitutionGroup:          line.item.SubstitutionGroup,
				Quantity:                   line.quantity,
				OrderIndex:                 line.order,
				ExpectedMonthlyConsumption: line.consumption,
				SetupCostMinorUnits:        int64(line.quantity) * line.item.OwnerPriceMinorUnits,
				MonthlyCostMinorUnits:      int64(line.consumption) * line.item.OwnerPriceMinorUnits,
			}
			if err := s.store.InsertPackageItem(ctx, tx, item); err != nil {
				return err
			}
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "catalog.package_version.created",
			ResourceType: "property_package_version",
			ResourceID:   version.ID,
			NewState:     marshalJSON(version),
		})
	})
	if err != nil {
		return nil, err
	}

	items, err := s.store.ListPackageVersionItems(ctx, tenantID, version.ID)
	if err != nil {
		return nil, err
	}
	version.Items = items

	return version, nil
}

func (s *Service) resolvePackageLines(ctx context.Context, tenantID string, params CreatePackageVersionParams) ([]resolvedItem, error) {
	var lines []resolvedItem
	nextOrder := 0

	seenSKU := map[string]bool{}
	addLine := func(catalogItem *CatalogItem, quantity, consumption, order int) error {
		if quantity <= 0 {
			return fmt.Errorf("%w: item quantity must be positive", ErrInvalidPackageVersion)
		}
		if consumption < 0 {
			return fmt.Errorf("%w: expected monthly consumption must not be negative", ErrInvalidPackageVersion)
		}
		if seenSKU[catalogItem.SKU] {
			return ErrDuplicatePackageSKU
		}
		seenSKU[catalogItem.SKU] = true
		lines = append(lines, resolvedItem{item: catalogItem, quantity: quantity, consumption: consumption, order: order})
		return nil
	}

	direct := append([]PackageItemInput(nil), params.Items...)
	sort.SliceStable(direct, func(i, j int) bool { return direct[i].OrderIndex < direct[j].OrderIndex })
	for _, in := range direct {
		catalogItem, err := s.store.GetCatalogItem(ctx, tenantID, in.CatalogItemID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrPackageVersionItemNotFound, in.CatalogItemID)
		}
		if err := ensureItemUsable(catalogItem); err != nil {
			return nil, err
		}
		if err := addLine(catalogItem, in.Quantity, in.ExpectedMonthlyConsumption, nextOrder); err != nil {
			return nil, err
		}
		nextOrder++
	}

	for _, bundle := range params.Bundles {
		tpl, err := s.store.GetPackageTemplate(ctx, tenantID, bundle.PackageTemplateID)
		if err != nil {
			return nil, err
		}
		for _, tplItem := range tpl.Items {
			catalogItem, err := s.store.GetCatalogItem(ctx, tenantID, tplItem.CatalogItemID)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrPackageVersionItemNotFound, tplItem.CatalogItemID)
			}
			if err := ensureItemUsable(catalogItem); err != nil {
				return nil, err
			}
			// Bundle items default to consuming their bundled quantity per month
			// as a deterministic baseline estimate.
			if err := addLine(catalogItem, tplItem.Quantity, tplItem.Quantity, nextOrder); err != nil {
				return nil, err
			}
			nextOrder++
		}
	}

	return lines, nil
}

func ensureItemUsable(catalogItem *CatalogItem) error {
	if catalogItem.Status != ItemStatusActive {
		return fmt.Errorf("%w: %s", ErrPackageItemDisabled, catalogItem.SKU)
	}
	return nil
}

func buildReviewItems(lines []resolvedItem) ([]ReviewItem, string, error) {
	currency := ""
	items := make([]ReviewItem, 0, len(lines))
	for _, line := range lines {
		c := line.item.OwnerPriceCurrency
		if currency == "" {
			currency = c
		} else if c != currency {
			return nil, "", fmt.Errorf("%w: all package items must share the same owner price currency", ErrInvalidPackageVersion)
		}
		items = append(items, ReviewItem{
			CatalogItemID:              line.item.ID,
			SKU:                        line.item.SKU,
			Name:                       line.item.Name,
			Label:                      line.item.Label,
			SubstitutionGroup:          line.item.SubstitutionGroup,
			Quantity:                   line.quantity,
			ExpectedMonthlyConsumption: line.consumption,
			SetupCostMinorUnits:        int64(line.quantity) * line.item.OwnerPriceMinorUnits,
			MonthlyCostMinorUnits:      int64(line.consumption) * line.item.OwnerPriceMinorUnits,
		})
	}
	return items, currency, nil
}

func (s *Service) GetPropertyPackageVersion(ctx context.Context, tenantID, propertyID, versionID string) (*PropertyPackageVersion, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.GetPackageVersionForProperty(ctx, tenantID, propertyID, versionID)
}

func (s *Service) ListPropertyPackageVersions(ctx context.Context, tenantID, propertyID string) ([]PropertyPackageVersion, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}
	return s.store.ListPackageVersions(ctx, tenantID, propertyID)
}

// ActivatePropertyPackageVersion activates a draft package version. Activation
// is only possible from the draft state in which the review summary (costs,
// consumption, and substitution behavior) was already computed and retained, so
// the owner necessarily saw cost and substitution before activation (CAT-005,
// CAT-009). Any previously active version for the property is superseded; every
// version remains retained (CAT-008).
func (s *Service) ActivatePropertyPackageVersion(ctx context.Context, tenantID, propertyID, versionID string, actorID string) (*PropertyPackageVersion, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}

	existing, err := s.store.GetPackageVersionForProperty(ctx, tenantID, propertyID, versionID)
	if err != nil {
		return nil, err
	}
	if existing.Status != PackageStatusDraft {
		if existing.Status == PackageStatusActive {
			return nil, ErrPackageVersionAlreadyActive
		}
		return nil, ErrPackageVersionNotDraft
	}

	var version *PropertyPackageVersion
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.SupersedeActiveVersions(ctx, tx, tenantID, propertyID); err != nil {
			return err
		}
		activated, err := s.store.ActivatePackageVersion(ctx, tx, tenantID, propertyID, versionID, time.Now().UTC())
		if err != nil {
			return err
		}
		version = activated
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "catalog.package_version.activated",
			ResourceType: "property_package_version",
			ResourceID:   version.ID,
			NewState:     marshalJSON(version),
		})
	})
	if err != nil {
		return nil, err
	}

	return version, nil
}

// RejectPropertyPackageVersion rejects a draft package version. Rejected
// versions are retained for the record and can never be activated.
func (s *Service) RejectPropertyPackageVersion(ctx context.Context, tenantID, propertyID, versionID string, actorID string) (*PropertyPackageVersion, error) {
	if err := s.authorizeProperty(ctx, tenantID, propertyID); err != nil {
		return nil, err
	}

	existing, err := s.store.GetPackageVersionForProperty(ctx, tenantID, propertyID, versionID)
	if err != nil {
		return nil, err
	}
	if existing.Status != PackageStatusDraft {
		return nil, ErrPackageVersionNotDraft
	}

	var version *PropertyPackageVersion
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		rejected, err := s.store.RejectPackageVersion(ctx, tx, tenantID, propertyID, versionID)
		if err != nil {
			return err
		}
		version = rejected
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "catalog.package_version.rejected",
			ResourceType: "property_package_version",
			ResourceID:   version.ID,
			NewState:     marshalJSON(version),
		})
	})
	if err != nil {
		return nil, err
	}

	return version, nil
}
