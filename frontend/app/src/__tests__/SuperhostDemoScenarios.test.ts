import { describe, expect, it } from "vitest";
import {
  SUPERHOST_DEMO_SCENARIOS,
  superhostScenariosFor,
} from "../lib/superhost-demo-scenarios";

describe("Superhost demo scenario dataset", () => {
  it("covers UI actions, proposals, reads, memory, and refusals", () => {
    expect(new Set(SUPERHOST_DEMO_SCENARIOS.map((scenario) => scenario.expected))).toEqual(
      new Set(["ui_action", "proposal", "read", "memory", "refusal"]),
    );
  });

  it("offers only owner-safe scenarios in the package shop", () => {
    const scenarios = superhostScenariosFor("owner", "/properties/prop-demo/package");
    expect(scenarios.map((scenario) => scenario.id)).toEqual([
      "package-build",
      "package-vibe-warm",
      "package-vibe-business",
      "package-quantity",
      "package-payment-refusal",
      "portfolio-summary",
      "remember-follow-up",
    ]);
    expect(scenarios.every((scenario) => scenario.roles.includes("owner"))).toBe(true);
  });

  it("keeps role-restricted proposals out of the owner prompt set", () => {
    const ownerPrompts = superhostScenariosFor("owner", "/dashboard");
    expect(ownerPrompts.some((scenario) => scenario.expected === "proposal")).toBe(false);
    expect(superhostScenariosFor("staff", "/ops/tickets").some((scenario) => scenario.id === "staff-maintenance")).toBe(true);
    expect(superhostScenariosFor("guest", "/stay").some((scenario) => scenario.id === "guest-restock")).toBe(true);
  });
});
