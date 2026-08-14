import { beforeEach, describe, expect, it } from "vitest";
import {
  clearPendingPurchase,
  getPendingPurchase,
  savePendingPurchase,
} from "../lib/pending-purchase";

describe("pending cart lifecycle", () => {
  const propertyId = "prop-pending-demo";

  beforeEach(() => clearPendingPurchase(propertyId));

  it("persists enough draft state for review and dashboard approval", () => {
    savePendingPurchase(propertyId, [{
      catalogItemId: "catalog-coffee",
      name: "Filter Coffee 100g",
      sku: "COFFEE-01",
      quantity: 3,
      monthlyUse: 3,
      unitPriceMinorUnits: 30000,
      currency: "INR",
      draftPackageId: "pkg-draft-01",
    }]);

    expect(getPendingPurchase(propertyId)).toEqual([expect.objectContaining({
      sku: "COFFEE-01",
      quantity: 3,
      draftPackageId: "pkg-draft-01",
    })]);
  });

  it("removes the dashboard task after activation or discard", () => {
    savePendingPurchase(propertyId, [{
      name: "Welcome Kit — Premium",
      sku: "KIT-01",
      quantity: 1,
      unitPriceMinorUnits: 140000,
      currency: "INR",
    }]);
    clearPendingPurchase(propertyId);
    expect(getPendingPurchase(propertyId)).toEqual([]);
  });
});
