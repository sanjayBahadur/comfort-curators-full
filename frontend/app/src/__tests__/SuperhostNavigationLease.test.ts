import { describe, expect, it } from "vitest";
import {
  DEFAULT_TTL,
  grantSession,
  isExpired,
  recordAction,
  remainingTime,
} from "../components/superhost/control-session";
import {
  clearCurrentProperty,
  getCurrentProperty,
  setCurrentProperty,
} from "../lib/current-property";

describe("Superhost navigation lease", () => {
  it("renews its bounded lease after each successful authorized action", () => {
    const granted = grantSession(1_000);
    const actionTime = 61_000;
    const renewed = recordAction(granted, actionTime);

    expect(renewed.startedAt).toBe(actionTime);
    expect(remainingTime(renewed, actionTime)).toBe(DEFAULT_TTL);
    expect(isExpired(renewed, actionTime + DEFAULT_TTL - 1)).toBe(false);
    expect(isExpired(renewed, actionTime + DEFAULT_TTL)).toBe(true);
  });

  it("does not clear property scope between adjacent property routes", async () => {
    setCurrentProperty({ id: "property-a", label: "Dashboard property" });
    clearCurrentProperty();
    setCurrentProperty({ id: "property-a", label: "Package property" });

    await Promise.resolve();
    expect(getCurrentProperty()).toEqual({ id: "property-a", label: "Package property" });

    clearCurrentProperty();
    await Promise.resolve();
    expect(getCurrentProperty()).toBeNull();
  });
});
