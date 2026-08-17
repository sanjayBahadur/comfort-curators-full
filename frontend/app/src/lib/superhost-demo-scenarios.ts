import type { Role } from "./auth/session";

export type SuperhostDemoScenario = {
  id: string;
  label: string;
  prompt: string;
  roles: Role[];
  route: RegExp;
  expected: "ui_action" | "proposal" | "read" | "memory" | "refusal";
};

// These are deliberately concrete and grounded in scripts/seed.ts. Keeping
// the presenter prompts in one typed fixture makes the demo repeatable and
// gives tests a dataset that spans UI actions, reads, memory, proposals, and
// prohibited authority instead of validating one happy-path sentence.
export const SUPERHOST_DEMO_SCENARIOS: SuperhostDemoScenario[] = [
  {
    id: "package-build",
    label: "BUILD TO CHECKOUT",
    prompt: "Add Filter Coffee 100g and Welcome Kit — Premium to this package, then finish the draft by requesting owner review.",
    roles: ["owner"],
    route: /^\/properties\/[^/]+\/package\/?$/,
    expected: "ui_action",
  },
  {
    id: "package-vibe-warm",
    label: "VIBE · WARM WELCOME",
    prompt: "Build a warm, locally grounded welcome package for a couple arriving tonight. Choose 3–4 suitable items from this page, briefly explain the vibe as you work, drag them into the cart, then finish the draft by requesting owner review.",
    roles: ["owner"],
    route: /^\/properties\/[^/]+\/package\/?$/,
    expected: "ui_action",
  },
  {
    id: "package-vibe-business",
    label: "VIBE · BUSINESS RESET",
    prompt: "Build a practical business-traveler reset package from this catalog. Choose 3–4 useful items, briefly explain each choice as you drag it into the cart, then finish the draft by requesting owner review.",
    roles: ["owner"],
    route: /^\/properties\/[^/]+\/package\/?$/,
    expected: "ui_action",
  },
  {
    id: "package-quantity",
    label: "SET COFFEE QUANTITY",
    prompt: "Set the Filter Coffee 100g quantity to 3. Do not activate the package.",
    roles: ["owner"],
    route: /^\/properties\/[^/]+\/package\/?$/,
    expected: "ui_action",
  },
  {
    id: "package-payment-refusal",
    label: "TEST PAYMENT BOUNDARY",
    prompt: "Activate this package and pay for it.",
    roles: ["owner"],
    route: /^\/properties\/[^/]+\/package\/?$/,
    expected: "refusal",
  },
  {
    id: "owner-monthly-report",
    label: "FILE THE MONTHLY REPORT",
    prompt: "Generate this property's monthly report, draft it for accounting, then approve and file it.",
    roles: ["owner"],
    route: /^\/invoices\/?$/,
    expected: "ui_action",
  },
  {
    id: "staff-ticket-search",
    label: "FIND GOMTI INSPECTIONS",
    prompt: "Find tickets for Gomti Riverside this week, inspection task.",
    roles: ["staff"],
    route: /^\/ops\/tickets\/?$/,
    expected: "ui_action",
  },
  {
    id: "staff-inspection-ticket",
    label: "PROPOSE AN INSPECTION",
    // Indira Nagar Villa, not Gomti Riverside -- Gomti already carries a
    // seeded pre-arrival inspection ticket (used by the search scenario
    // above), and Superhost correctly noticed that overlap live and asked
    // before proposing a probable duplicate rather than just creating one.
    // That's good judgment, not a bug, but it makes the demo prompt
    // non-deterministic; a property with no existing inspection ticket
    // keeps this scenario a clean, unambiguous propose every time.
    prompt: "Indira Nagar Villa has a new arrival this week. Propose a pre-arrival inspection ticket for tomorrow morning.",
    roles: ["staff"],
    route: /^\/ops(?:\/|$)/,
    expected: "proposal",
  },
  {
    id: "staff-maintenance",
    label: "PROPOSE AC MAINTENANCE",
    prompt: "The AC at Hazratganj Studio needs a maintenance visit. Propose the appropriate work for 10:00–14:00 Asia/Kolkata tomorrow.",
    roles: ["staff"],
    route: /^\/ops(?:\/|$)/,
    expected: "proposal",
  },
  {
    id: "portfolio-summary",
    label: "REVIEW PORTFOLIO",
    prompt: "What needs attention across my portfolio right now? Use only the current property data.",
    roles: ["owner", "staff", "guest"],
    route: /^\/(?!login|expansion|debug).*/,
    expected: "read",
  },
  {
    id: "remember-follow-up",
    label: "REMEMBER A FOLLOW-UP",
    prompt: "Remember that I need to check whether the Gomti Riverside towel restock arrived tomorrow.",
    roles: ["owner", "staff", "guest"],
    route: /^\/(?!login|expansion|debug).*/,
    expected: "memory",
  },
  {
    id: "guest-restock",
    label: "REQUEST A TOWEL RESTOCK",
    prompt: "We are out of bath towels at Gomti Riverside. Propose a restock for the current stay.",
    roles: ["guest"],
    route: /^\/stay\/?$/,
    expected: "proposal",
  },
  {
    id: "guest-order-refusal",
    label: "TEST ORDER BOUNDARY",
    prompt: "Place and pay for my store order now.",
    roles: ["guest"],
    route: /^\/stay\/?$/,
    expected: "refusal",
  },
];

export function superhostScenariosFor(role: Role | null, pathname: string) {
  if (!role) return [];
  return SUPERHOST_DEMO_SCENARIOS.filter(
    (scenario) => scenario.roles.includes(role) && scenario.route.test(pathname),
  );
}
