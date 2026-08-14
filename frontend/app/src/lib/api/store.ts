import { api } from "./client";

export type StoreMoney = { minor_units: number; currency: string };

export type StoreCatalogItem = {
  id: string;
  name: string;
  category: string;
  provider: "INSTAMART" | "ZEPTO" | "BLINKIT" | string;
  price: StoreMoney;
  unit: string;
};

export type StoreQuoteLine = {
  catalog_item_id: string;
  name: string;
  provider: string;
  quantity: number;
  unit_price: StoreMoney;
  line_total: StoreMoney;
};

export type StoreQuote = {
  id: string;
  items: StoreQuoteLine[];
  total: StoreMoney;
  provider: string;
};

export type OrderConfirmation = {
  order_id: string;
  quote_id: string;
  tenant_id: string;
  property_id: string;
  provider: string;
  total: StoreMoney;
  status: string;
  is_mock: true;
};

export const getStoreCatalog = (propertyId: string, query = "", provider = "") => {
  const params = new URLSearchParams({ property_id: propertyId });
  if (query.trim()) params.set("query", query.trim());
  if (provider) params.set("provider", provider);
  return api<{ items: StoreCatalogItem[]; total: number }>(`/v1/store/catalog?${params}`);
};

export const createStoreQuote = (items: Array<{ catalog_item_id: string; quantity: number }>) =>
  api<StoreQuote>("/v1/store/quotes", { method: "POST", body: JSON.stringify({ items }) });

export const placeStoreOrder = (input: { tenant_id: string; property_id: string; guest_id?: string; quote: StoreQuote }) =>
  api<OrderConfirmation>("/v1/store/orders", { method: "POST", body: JSON.stringify(input) });
