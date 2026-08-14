import { api, type Envelope } from "./client";

export type CatalogLabel = "curators_standard" | "owner_preferred" | "alternative";

export type CatalogItem = {
  sku: string;
  name: string;
  category: string;
  brand: string;
  pack_size: string;
  owner_price_minor_units: number;
  owner_price_currency: string;
  label: CatalogLabel;
  operational_suitability: string;
  status: "active" | "disabled";
};

export type CatalogResource = Envelope<CatalogItem>;

export type PropertyData = {
  service_address: {
    line1: string;
    line2?: string;
    city: string;
    state: string;
    postal_code: string;
    country: string;
  };
  state: string;
};

export type PropertyResource = Envelope<PropertyData>;

export type PackagePolicy = "owner_approval" | "automatic" | "restricted";

export type PackageDraftLine = {
  catalog_item_id: string;
  quantity: number;
  expected_monthly_consumption: number;
  order_index: number;
};

export type PackageDraftInput = {
  effective_date: string;
  substitution_policy: PackagePolicy;
  require_approval_for_price_increase: boolean;
  require_approval_for_new_sku: boolean;
  items: PackageDraftLine[];
  bundles: [];
};

export type PackageVersion = {
  status: "draft" | "active" | "superseded" | "rejected";
  version_number: number;
  setup_cost_minor_units: number;
  monthly_cost_minor_units: number;
  monthly_consumption_units: number;
  currency: string;
};

export type PackageResource = Envelope<PackageVersion>;

type CatalogResponse = { items: CatalogResource[]; total: number };
type PropertiesResponse = { items: PropertyResource[]; next_cursor?: string | null };

export const getCatalog = () => api<CatalogResponse>("/v1/catalog/items");
export const getProperties = () => api<PropertiesResponse>("/v1/properties");

export const createPackageDraft = (
  propertyId: string,
  input: PackageDraftInput,
  signal?: AbortSignal,
) =>
  api<PackageResource>(`/v1/properties/${propertyId}/packages`, {
    method: "POST",
    body: JSON.stringify(input),
    signal,
  });

export const activatePackage = (propertyId: string, versionId: string) =>
  api<PackageResource>(`/v1/properties/${propertyId}/packages/${versionId}/activate`, {
    method: "POST",
    body: JSON.stringify({}),
  });
