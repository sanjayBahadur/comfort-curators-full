package catalog

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/catalog/items", h.handleCreateItem)
	mux.HandleFunc("GET /v1/catalog/items", h.handleListItems)
	mux.HandleFunc("GET /v1/catalog/items/{item_id}", h.handleGetItem)
	mux.HandleFunc("POST /v1/catalog/items/{item_id}/claims", h.handleAddClaimEvidence)
	mux.HandleFunc("GET /v1/catalog/items/{item_id}/claims", h.handleListClaimEvidence)
	mux.HandleFunc("POST /v1/catalog/templates", h.handleCreateTemplate)
	mux.HandleFunc("GET /v1/catalog/templates", h.handleListTemplates)
	mux.HandleFunc("GET /v1/catalog/templates/{template_id}", h.handleGetTemplate)
	mux.HandleFunc("POST /v1/properties/{property_id}/packages", h.handleCreatePackageVersion)
	mux.HandleFunc("GET /v1/properties/{property_id}/packages", h.handleListPackageVersions)
	mux.HandleFunc("GET /v1/properties/{property_id}/packages/{version_id}", h.handleGetPackageVersion)
	mux.HandleFunc("POST /v1/properties/{property_id}/packages/{version_id}/activate", h.handleActivatePackageVersion)
	mux.HandleFunc("POST /v1/properties/{property_id}/packages/{version_id}/reject", h.handleRejectPackageVersion)
}

type catalogResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

type catalogError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func apiError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(catalogError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiResource(w http.ResponseWriter, status int, id string, version int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(catalogResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiCollection(w http.ResponseWriter, items []catalogResource) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

// --- Catalog Items -----------------------------------------------------------

type createItemRequest struct {
	SKU                    string `json:"sku"`
	Name                   string `json:"name"`
	Category               string `json:"category"`
	Brand                  string `json:"brand"`
	PackSize               string `json:"pack_size"`
	UnitCostMinorUnits     int64  `json:"unit_cost_minor_units"`
	UnitCostCurrency       string `json:"unit_cost_currency"`
	OwnerPriceMinorUnits   int64  `json:"owner_price_minor_units"`
	OwnerPriceCurrency     string `json:"owner_price_currency"`
	TaxClass               string `json:"tax_class"`
	Supplier               string `json:"supplier"`
	CountryOfOrigin        string `json:"country_of_origin"`
	Status                 string `json:"status"`
	ShelfLifeRule          string `json:"shelf_life_rule"`
	SubstitutionGroup      string `json:"substitution_group"`
	OperationalSuitability string `json:"operational_suitability"`
	Label                  string `json:"label"`
}

func (h *Handler) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req createItemRequest
	body, _ := io.ReadAll(r.Body)
	if json.Unmarshal(body, &req) != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	item, err := h.svc.CreateCatalogItem(r.Context(), tenantID, CreateItemParams{
		SKU:                    req.SKU,
		Name:                   req.Name,
		Category:               req.Category,
		Brand:                  req.Brand,
		PackSize:               req.PackSize,
		UnitCostMinorUnits:     req.UnitCostMinorUnits,
		UnitCostCurrency:       req.UnitCostCurrency,
		OwnerPriceMinorUnits:   req.OwnerPriceMinorUnits,
		OwnerPriceCurrency:     req.OwnerPriceCurrency,
		TaxClass:               req.TaxClass,
		Supplier:               req.Supplier,
		CountryOfOrigin:        req.CountryOfOrigin,
		Status:                 req.Status,
		ShelfLifeRule:          req.ShelfLifeRule,
		SubstitutionGroup:      req.SubstitutionGroup,
		OperationalSuitability: req.OperationalSuitability,
		Label:                  req.Label,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidItem),
			errors.Is(err, ErrInvalidCurrency),
			errors.Is(err, ErrInvalidLabel),
			errors.Is(err, ErrSponsoredDisabled):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrSKUAlreadyExists):
			status = http.StatusConflict
			code = "SKU_ALREADY_EXISTS"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, item.ID, item.Version, item)
}

func (h *Handler) handleGetItem(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	itemID := r.PathValue("item_id")
	item, err := h.svc.GetCatalogItem(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "catalog item not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, item.ID, item.Version, item)
}

func (h *Handler) handleListItems(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	items, err := h.svc.ListCatalogItems(r.Context(), tenantID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resources := make([]catalogResource, 0, len(items))
	for i := range items {
		resources = append(resources, catalogResource{
			ID:      items[i].ID,
			Version: items[i].Version,
			Data:    &items[i],
		})
	}
	apiCollection(w, resources)
}

// --- Claim Evidence -----------------------------------------------------------

type addClaimEvidenceRequest struct {
	ClaimType      string `json:"claim_type"`
	ClaimStatement string `json:"claim_statement"`
	EvidenceRef    string `json:"evidence_ref"`
}

func (h *Handler) handleAddClaimEvidence(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	itemID := r.PathValue("item_id")

	var req addClaimEvidenceRequest
	body, _ := io.ReadAll(r.Body)
	if json.Unmarshal(body, &req) != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	evidence, err := h.svc.AddClaimEvidence(r.Context(), tenantID, itemID, ClaimEvidenceParams{
		ClaimType:      req.ClaimType,
		ClaimStatement: req.ClaimStatement,
		EvidenceRef:    req.EvidenceRef,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidClaimType),
			errors.Is(err, ErrClaimEvidenceRequired):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrItemNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, evidence.ID, 1, evidence)
}

func (h *Handler) handleListClaimEvidence(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	itemID := r.PathValue("item_id")
	evidence, err := h.svc.ListClaimEvidence(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "catalog item not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resources := make([]catalogResource, 0, len(evidence))
	for i := range evidence {
		resources = append(resources, catalogResource{
			ID:      evidence[i].ID,
			Version: 1,
			Data:    &evidence[i],
		})
	}
	apiCollection(w, resources)
}

// --- Package Templates --------------------------------------------------------

type createTemplateRequest struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Items       []PackageTemplateItem `json:"items"`
}

func (h *Handler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req createTemplateRequest
	body, _ := io.ReadAll(r.Body)
	if json.Unmarshal(body, &req) != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	tpl, err := h.svc.CreatePackageTemplate(r.Context(), tenantID, CreateTemplateParams{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrInvalidTemplate),
			errors.Is(err, ErrNoPackageItems),
			errors.Is(err, ErrDuplicatePackageSKU):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrItemNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, tpl.ID, tpl.Version, tpl)
}

func (h *Handler) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	templateID := r.PathValue("template_id")
	tpl, err := h.svc.GetPackageTemplate(r.Context(), tenantID, templateID)
	if err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "package template not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, tpl.ID, tpl.Version, tpl)
}

func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	templates, err := h.svc.ListPackageTemplates(r.Context(), tenantID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resources := make([]catalogResource, 0, len(templates))
	for i := range templates {
		resources = append(resources, catalogResource{
			ID:      templates[i].ID,
			Version: templates[i].Version,
			Data:    &templates[i],
		})
	}
	apiCollection(w, resources)
}

// --- Property Package Versions ------------------------------------------------

type createPackageVersionRequest struct {
	EffectiveDate                   string             `json:"effective_date"`
	MonthlyBudgetLimitMinorUnits    *int64             `json:"monthly_budget_limit_minor_units"`
	SubstitutionPolicy              string             `json:"substitution_policy"`
	RequireApprovalForPriceIncrease bool               `json:"require_approval_for_price_increase"`
	RequireApprovalForNewSKU        bool               `json:"require_approval_for_new_sku"`
	Items                           []packageItemReq   `json:"items"`
	Bundles                         []packageBundleReq `json:"bundles"`
}

type packageItemReq struct {
	CatalogItemID              string `json:"catalog_item_id"`
	Quantity                   int    `json:"quantity"`
	ExpectedMonthlyConsumption int    `json:"expected_monthly_consumption"`
	OrderIndex                 int    `json:"order_index"`
}

type packageBundleReq struct {
	PackageTemplateID string `json:"package_template_id"`
	OrderIndex        int    `json:"order_index"`
}

func (h *Handler) handleCreatePackageVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	var req createPackageVersionRequest
	body, _ := io.ReadAll(r.Body)
	if json.Unmarshal(body, &req) != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	effectiveDate, err := time.Parse(time.RFC3339, req.EffectiveDate)
	if err != nil {
		apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid effective_date, use RFC3339")
		return
	}

	items := make([]PackageItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = PackageItemInput{
			CatalogItemID:              it.CatalogItemID,
			Quantity:                   it.Quantity,
			ExpectedMonthlyConsumption: it.ExpectedMonthlyConsumption,
			OrderIndex:                 it.OrderIndex,
		}
	}

	bundles := make([]PackageBundleInput, len(req.Bundles))
	for i, b := range req.Bundles {
		bundles[i] = PackageBundleInput{
			PackageTemplateID: b.PackageTemplateID,
			OrderIndex:        b.OrderIndex,
		}
	}

	version, err := h.svc.CreatePropertyPackageVersion(r.Context(), tenantID, propertyID, CreatePackageVersionParams{
		EffectiveDate:                   effectiveDate,
		MonthlyBudgetLimitMinorUnits:    req.MonthlyBudgetLimitMinorUnits,
		SubstitutionPolicy:              req.SubstitutionPolicy,
		RequireApprovalForPriceIncrease: req.RequireApprovalForPriceIncrease,
		RequireApprovalForNewSKU:        req.RequireApprovalForNewSKU,
		Items:                           items,
		Bundles:                         bundles,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrEffectiveDateRequired),
			errors.Is(err, ErrInvalidSubstitutionPolicy),
			errors.Is(err, ErrInvalidPackageVersion),
			errors.Is(err, ErrNoPackageItems),
			errors.Is(err, ErrDuplicatePackageSKU),
			errors.Is(err, ErrPackageItemDisabled):
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		case errors.Is(err, ErrPackageVersionItemNotFound),
			errors.Is(err, ErrItemNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
		case errors.Is(err, ErrCrossTenantDenied),
			errors.Is(err, ErrUnauthorized):
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, version.ID, version.Version, version)
}

func (h *Handler) handleGetPackageVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	versionID := r.PathValue("version_id")

	version, err := h.svc.GetPropertyPackageVersion(r.Context(), tenantID, propertyID, versionID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		msg := err.Error()
		switch {
		case errors.Is(err, ErrPackageVersionNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
			msg = "property package version not found"
		case errors.Is(err, ErrCrossTenantDenied),
			errors.Is(err, ErrUnauthorized):
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		apiError(w, r, status, code, msg)
		return
	}

	apiResource(w, http.StatusOK, version.ID, version.Version, version)
}

func (h *Handler) handleListPackageVersions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")

	versions, err := h.svc.ListPropertyPackageVersions(r.Context(), tenantID, propertyID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		switch {
		case errors.Is(err, ErrCrossTenantDenied),
			errors.Is(err, ErrUnauthorized):
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	resources := make([]catalogResource, 0, len(versions))
	for i := range versions {
		resources = append(resources, catalogResource{
			ID:      versions[i].ID,
			Version: versions[i].Version,
			Data:    &versions[i],
		})
	}
	apiCollection(w, resources)
}

func (h *Handler) handleActivatePackageVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	versionID := r.PathValue("version_id")

	version, err := h.svc.ActivatePropertyPackageVersion(r.Context(), tenantID, propertyID, versionID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		msg := err.Error()
		switch {
		case errors.Is(err, ErrPackageVersionNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
			msg = "property package version not found"
		case errors.Is(err, ErrPackageVersionNotDraft),
			errors.Is(err, ErrPackageVersionAlreadyActive):
			status = http.StatusConflict
			code = "INVALID_TRANSITION"
		case errors.Is(err, ErrCrossTenantDenied),
			errors.Is(err, ErrUnauthorized):
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		apiError(w, r, status, code, msg)
		return
	}

	apiResource(w, http.StatusOK, version.ID, version.Version, version)
}

func (h *Handler) handleRejectPackageVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.PathValue("property_id")
	versionID := r.PathValue("version_id")

	version, err := h.svc.RejectPropertyPackageVersion(r.Context(), tenantID, propertyID, versionID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		msg := err.Error()
		switch {
		case errors.Is(err, ErrPackageVersionNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
			msg = "property package version not found"
		case errors.Is(err, ErrPackageVersionNotDraft):
			status = http.StatusConflict
			code = "INVALID_TRANSITION"
		case errors.Is(err, ErrCrossTenantDenied),
			errors.Is(err, ErrUnauthorized):
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		apiError(w, r, status, code, msg)
		return
	}

	apiResource(w, http.StatusOK, version.ID, version.Version, version)
}
