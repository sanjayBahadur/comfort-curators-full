const API_BASE_URL = process.env.CC_API_BASE_URL ?? "http://127.0.0.1:8080";
const APP_BASE_URL = process.env.CC_APP_BASE_URL ?? "http://127.0.0.1:3000";
const TENANT_ID =
  process.env.CC_DEMO_TENANT_ID ?? "11111111-1111-4111-8111-111111111111";
const OWNER_AUTHORITY_ID =
  process.env.CC_DEMO_OWNER_AUTHORITY_ID ??
  "22222222-2222-4222-8222-222222222222";
const CALENDAR_FIXTURE_PATHS = ["/demo.ics", "/demo-hazratganj.ics"] as const;

type JsonRecord = Record<string, unknown>;

type Resource<T> = {
  id: string;
  version: number;
  data: T;
};

type Collection<T> = {
  items: Array<Resource<T>>;
  next_cursor?: string | null;
  total?: number;
};

type SessionResponse = {
  roles: string[];
  session_token: string;
  user_id: string;
};

type CatalogItem = {
  sku: string;
  name: string;
  category: string;
  brand: string;
  pack_size: string;
  unit_cost_minor_units: number;
  unit_cost_currency: "INR";
  owner_price_minor_units: number;
  owner_price_currency: "INR";
  tax_class: string;
  supplier: string;
  country_of_origin: "IN";
  status: "active";
  shelf_life_rule: string;
  substitution_group: string;
  operational_suitability: string;
  label: "curators_standard" | "owner_preferred" | "alternative";
};

type Address = {
  line1: string;
  line2?: string;
  city: string;
  state: string;
  postal_code: string;
  country: string;
};

type PropertyData = {
  service_address: Address;
  state: PropertyState;
  readiness: {
    owner_contract_accepted: boolean;
    compliance_complete: boolean;
    mandatory_fields_set: boolean;
  };
};

type DocumentData = {
  property_id: string;
  title: string;
  document_type: string;
};

type PropertyState =
  | "lead"
  | "qualifying"
  | "onboarding"
  | "remediation"
  | "ready_inactive"
  | "active"
  | "paused"
  | "suspended"
  | "offboarding"
  | "archived";

const PROPERTY_TRANSITION_REASONS: Record<PropertyState, string> = {
  lead: "Owner enquiry recorded for an initial service review.",
  qualifying: "Service area, property type, and operating fit confirmed.",
  onboarding: "Owner onboarding opened and required property records requested.",
  remediation: "Walkthrough findings assigned for resolution before launch.",
  ready_inactive: "Onboarding records, access details, and compliance checks completed.",
  active: "Owner approvals confirmed and the property released into active operations.",
  paused: "Operations paused while the property scope is reviewed.",
  suspended: "Operations suspended after a compliance or safety exception.",
  offboarding: "Service closeout opened and final property records requested.",
  archived: "Service closeout completed and the property record archived.",
};

type WorkerData = {
  legal_name: string;
  status: string;
  service_zone: string;
  skills: string[];
};

type AvailabilityWindow = {
  id: string;
  worker_id: string;
  day_of_week: number;
  start_minute: number;
  end_minute: number;
};

type TicketStatus =
  | "draft"
  | "proposed"
  | "approved"
  | "scheduled"
  | "assigned"
  | "in_progress"
  | "evidence_submitted"
  | "verified"
  | "closed"
  | "blocked"
  | "cancelled"
  | "rejected";

type TicketData = {
  property_id: string;
  type: string;
  status: TicketStatus;
  reason: string;
};

type PackageData = {
  status: "draft" | "active" | "superseded" | "rejected";
  effective_date: string;
  setup_cost_minor_units: number;
  monthly_cost_minor_units: number;
  currency: string;
  items: Array<{ sku: string }>;
};

type CalendarFeedData = {
  source: string;
  url: string;
  status: "active" | "paused" | "disabled";
};

type DispatchCandidates = {
  data: {
    ticket_id: string;
    candidates: Array<{
      worker_id: string;
      eligible: boolean;
      score: number;
    }> | null;
  };
};

type PollResponse = {
  status: string;
  result: {
    unchanged: boolean;
    events_created: number;
    reservations_created: number;
    proposals_proposed: number;
  };
};

type ProposalGeneration = {
  result: {
    proposed: number;
    updated: number;
    cancelled: number;
    skipped: boolean;
    reason?: string;
  };
};

type SeedStats = {
  created: Record<string, number>;
  reused: Record<string, number>;
};

type RequestOptions = {
  method?: "GET" | "POST" | "PUT";
  body?: unknown;
  headers?: Record<string, string>;
  authenticated?: boolean;
};

class SeedApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly requestId?: string;

  constructor(
    message: string,
    status: number,
    code?: string,
    requestId?: string,
  ) {
    super(message);
    this.name = "SeedApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

let sessionToken = "";

const stats: SeedStats = {
  created: {},
  reused: {},
};

function count(kind: "created" | "reused", resource: string) {
  stats[kind][resource] = (stats[kind][resource] ?? 0) + 1;
}

function step(message: string) {
  console.log(`\n→ ${message}`);
}

function detail(message: string) {
  console.log(`  ${message}`);
}

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.authenticated !== false) {
    if (!sessionToken) {
      throw new Error(`No authenticated session token is available for ${path}`);
    }
    headers.set("Authorization", `Bearer ${sessionToken}`);
  }
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method ?? "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  const raw = await response.text();
  let payload: unknown = undefined;
  if (raw) {
    try {
      payload = JSON.parse(raw);
    } catch {
      payload = raw;
    }
  }

  if (!response.ok) {
    const errorPayload = isRecord(payload) ? payload : {};
    const message =
      typeof errorPayload.message === "string"
        ? errorPayload.message
        : `Request failed with HTTP ${response.status}`;
    const code =
      typeof errorPayload.code === "string" ? errorPayload.code : undefined;
    const requestId =
      typeof errorPayload.request_id === "string"
        ? errorPayload.request_id
        : (response.headers.get("x-request-id") ?? undefined);
    throw new SeedApiError(message, response.status, code, requestId);
  }

  return payload as T;
}

async function preflight() {
  step("Checking the backend and the demo iCalendar feeds");

  const health = await request<{ status: string }>("/health/ready", {
    authenticated: false,
  });
  if (health.status !== "ok") {
    throw new Error(`Backend is not ready: ${JSON.stringify(health)}`);
  }
  detail(`backend ready at ${API_BASE_URL}`);

  for (const fixturePath of CALENDAR_FIXTURE_PATHS) {
    const feedResponse = await fetch(`${APP_BASE_URL}${fixturePath}`);
    const feedBody = await feedResponse.text();
    if (!feedResponse.ok || !feedBody.startsWith("BEGIN:VCALENDAR")) {
      throw new Error(
        `Vite must be running at ${APP_BASE_URL} and serving ${fixturePath} before seeding`,
      );
    }
    detail(`${fixturePath.slice(1)} available at ${APP_BASE_URL}${fixturePath}`);
  }
}

async function createStaffSession() {
  step("Creating a staff session");
  const session = await request<SessionResponse>("/auth/session/create", {
    method: "POST",
    authenticated: false,
    body: {
      tenant_id: TENANT_ID,
      contact: "phase2.seed@comfortcurators.in",
      roles: ["staff"],
    },
  });
  if (!session.session_token || !session.roles.includes("staff")) {
    throw new Error("The backend did not mint a valid staff session");
  }
  sessionToken = session.session_token;
  detail(`staff actor ${session.user_id}`);
}

async function createOwnerSession() {
  step("Creating the demo owner session");
  const session = await request<SessionResponse>("/auth/session/create", {
    method: "POST",
    authenticated: false,
    body: {
      tenant_id: TENANT_ID,
      contact: "owner@demo.test",
      roles: ["owner"],
    },
  });
  if (!session.session_token || !session.roles.includes("owner")) {
    throw new Error("The backend did not mint a valid owner session");
  }
  sessionToken = session.session_token;
  detail(`owner actor ${session.user_id}`);
}

const catalogSeed: CatalogItem[] = [
  catalogItem("TOWEL-01", "Bath Towel 500gsm", "linen", "Trident", "1 towel", 320, 450, "towels", "Trident Ltd"),
  catalogItem("TOWEL-02", "Hand Towel", "linen", "Trident", "1 towel", 140, 200, "towels", "Trident Ltd"),
  catalogItem("SHEET-01", "Cotton Bedsheet Set (Queen)", "linen", "Spaces", "1 set", 890, 1_250, "queen_bedding", "Welspun India"),
  catalogItem("PILLOW-01", "Microfibre Pillow", "linen", "Sleepwell", "1 pillow", 340, 480, "pillows", "Sheela Foam"),
  catalogItem("SOAP-01", "Handmade Soap Bar", "toiletries", "Kama Ayurveda", "100 g", 60, 95, "guest_soap", "Lucknow Essentials"),
  catalogItem("SHAMP-01", "Shampoo 50ml", "toiletries", "Biotique", "50 ml", 45, 75, "guest_shampoo", "Lucknow Essentials"),
  catalogItem("TEA-01", "Assam Tea Sachets (10)", "perishable", "Tata Tea", "10 sachets", 120, 180, "tea", "Tata Consumer Products", "owner_preferred", "best_before_12_months"),
  catalogItem("COFFEE-01", "Filter Coffee 100g", "perishable", "Cothas", "100 g", 210, 300, "coffee", "Cothas Coffee", "curators_standard", "best_before_9_months"),
  catalogItem("WATER-01", "Mineral Water 1L (×6)", "perishable", "Bisleri", "6 × 1 L", 150, 220, "drinking_water", "Bisleri International", "alternative", "best_before_12_months"),
  catalogItem("CLEAN-01", "Floor Cleaner 500ml", "cleaning", "Lizol", "500 ml", 130, 190, "floor_cleaner", "Reckitt India"),
  catalogItem("CLEAN-02", "Glass Cleaner 500ml", "cleaning", "Colin", "500 ml", 110, 165, "glass_cleaner", "Reckitt India"),
  catalogItem("KIT-01", "Welcome Kit — Premium", "bundle", "Comfort Curators", "1 kit", 900, 1_400, "welcome_kit", "Lucknow Essentials", "owner_preferred"),
  catalogItem("LAMP-01", "Bedside Lamp", "decor", "Fabindia", "1 lamp", 1_200, 1_750, "bedside_lighting", "Fabindia Ltd"),
  catalogItem("CUSH-01", "Cushion Cover (pair)", "decor", "Fabindia", "2 covers", 380, 550, "cushion_covers", "Fabindia Ltd", "alternative"),
  catalogItem("FIRSTAID-01", "First Aid Kit", "safety", "Apollo", "1 kit", 450, 620, "first_aid", "Apollo Pharmacy"),

  catalogItem("TOWEL-03", "Green Bath Towel Set", "linen", "Comfort Curators", "1 set", 390, 550, "towel_set", "Local Weavers"),
  catalogItem("TOWEL-04", "Multicolor Bath Towel Set", "linen", "Comfort Curators", "1 set", 390, 550, "towel_set", "Local Weavers"),
  catalogItem("TOWEL-05", "Dark Gray Towel Set", "linen", "Comfort Curators", "1 set", 460, 650, "towel_set", "Local Weavers"),
  catalogItem("SHEET-02", "Bamboo King Sheet Set (Ivory)", "linen", "Pure Bamboo", "1 set", 1_280, 1_800, "king_bedding", "Pure Bamboo"),
  catalogItem("BEDSET-01", "Queen Comforter Bedding Set (Dark Grey)", "linen", "Comfort Curators", "1 set", 1_560, 2_200, "queen_bedding", "Lucknow Textiles", "alternative"),
  catalogItem("BEDSP-01", "Quilted Bedspread (Dark Purple)", "linen", "Comfort Curators", "1 bedspread", 1_070, 1_500, "queen_bedding", "Lucknow Textiles", "alternative"),
  catalogItem("TOOTHBR-01", "Disposable Toothbrushes (36-Pack)", "toiletries", "Aussumy", "36 pcs", 85, 120, "guest_toothbrush", "Aussumy"),
  catalogItem("TOOTHBR-02", "Disposable Toothbrushes (160-Pack)", "toiletries", "Avistar", "160 pcs", 200, 280, "guest_toothbrush", "Avistar"),
  catalogItem("TOOTHBR-03", "Mini Disposable Toothbrushes (20-Pack)", "toiletries", "Malisseladi", "20 pcs", 55, 80, "guest_toothbrush", "Malisseladi"),
  catalogItem("TOOTHPA-01", "Sensodyne Pronamel Multipack", "toiletries", "Sensodyne", "1 pack", 320, 450, "guest_toothpaste", "GSK India"),
  catalogItem("TOILETRY-01", "Hotel Toiletries Set (Tropical Oasis)", "toiletries", "Black Tropical Oasis", "1 set", 250, 350, "guest_amenity", "Hotel Essentials Ltd"),
  catalogItem("TOILETRY-02", "Hotel Toiletries Set (Tropical Oasis, 200-Piece)", "toiletries", "Black Tropical Oasis", "200 pcs", 600, 850, "guest_amenity", "Hotel Essentials Ltd"),
  catalogItem("TOILETRY-03", "Hotel Toiletries Set (White Tea, 75-Piece)", "toiletries", "Terra Pure", "75 pcs", 460, 650, "guest_amenity", "Terra Pure Essentials"),
  catalogItem("TOILETRY-04", "Hotel Toiletries Set (Nature Essence)", "toiletries", "Nature Essence", "1 set", 250, 350, "guest_amenity", "Hotel Essentials Ltd"),
  catalogItem("WIPES-01", "Flushable Wipes (60-Count, 6-Pack)", "toiletries", "Mollis", "360 wipes", 200, 280, "guest_wipes", "Mollis Hygiene"),
  catalogItem("WIPES-02", "Bamboo Clean Towels (XL, 50-Count)", "toiletries", "Clean Skin Club", "50 pcs", 140, 200, "guest_wipes", "Clean Skin Club"),
  catalogItem("KCUP-01", "Hazelnut Coffee K-Cup Pods (100-Count)", "perishable", "Happy Belly", "100 pods", 850, 1_200, "coffee", "Happy Belly", "alternative", "best_before_9_months"),
  catalogItem("KCUP-02", "Starbucks Variety K-Cup Pods (40-Count)", "perishable", "Starbucks", "40 pods", 530, 750, "coffee", "Starbucks India", "alternative", "best_before_9_months"),
  catalogItem("CLEAN-03", "Sal Suds Pine Cleaner (1 Gallon)", "cleaning", "Dr. Bronner's", "1 gal", 460, 650, "all_purpose_cleaner", "Dr. Bronner's"),
  catalogItem("CANDLE-01", "Teakwood Mahogany Candle", "decor", "Homelux", "1 candle", 270, 380, "candles", "Homelux"),
  catalogItem("CANDLE-02", "Sandalwood Candle Gift Set", "decor", "Homelux", "1 set", 530, 750, "candles", "Homelux", "owner_preferred"),
  catalogItem("CANDLE-03", "Balsam Cedar Candle", "decor", "Yankee Candle", "1 candle", 320, 450, "candles", "Yankee Candle"),
  catalogItem("ARTIFPLANT-01", "Artificial Trailing Ivy Wall Greenery", "decor", "Comfort Curators", "1 piece", 1_280, 1_800, "artificial_plants", "Lucknow Decor"),
  catalogItem("ARTIFPLANT-02", "Artificial Willow Branch Vine Garland", "decor", "Comfort Curators", "1 garland", 670, 950, "artificial_plants", "Lucknow Decor"),
  catalogItem("ARTIFPLANT-03", "Artificial Potted Plants (8-Pack)", "decor", "Comfort Curators", "8 pcs", 850, 1_200, "artificial_plants", "Lucknow Decor"),
  catalogItem("ARTIFPLANT-04", "Artificial Succulent Cactus Pots (4-Piece)", "decor", "Comfort Curators", "4 pcs", 460, 650, "artificial_plants", "Lucknow Decor"),
  catalogItem("ARTIFPLANT-05", "Artificial Eucalyptus Potted Plants (6-Pack)", "decor", "Comfort Curators", "6 pcs", 780, 1_100, "artificial_plants", "Lucknow Decor"),
  catalogItem("RUG-01", "Fish Print Fringed Rug", "decor", "Comfort Curators", "1 rug", 1_280, 1_800, "rugs", "Lucknow Textiles"),
  catalogItem("TAPESTRY-01", "Medieval Cat Musical Tapestry", "decor", "Comfort Curators", "1 tapestry", 850, 1_200, "wall_decor", "Lucknow Decor"),
  catalogItem("TAPESTRY-02", "Mountain Lake Forest Wall Tapestry", "decor", "Comfort Curators", "1 tapestry", 850, 1_200, "wall_decor", "Lucknow Decor"),
  catalogItem("WALLART-01", "Floral Wall Banners Set", "decor", "Noble Unicorn", "1 set", 530, 750, "wall_decor", "Noble Unicorn"),
  catalogItem("WINDCH-01", "Crystal Wind Chime (Tree of Life)", "decor", "Tree of Life", "1 chime", 390, 550, "wall_decor", "Tree of Life"),
  catalogItem("PLANTER-01", "Wall-Mounted Glass Pothos Planter", "decor", "Comfort Curators", "1 planter", 320, 450, "planters", "Lucknow Decor"),
  catalogItem("WALLO-01", "Botanical Fabric Wall Organizer", "decor", "Comfort Curators", "1 organizer", 460, 650, "wall_decor", "Lucknow Decor"),
  catalogItem("MACRAME-01", "Macrame Hanging Plant Holders Set", "decor", "Comfort Curators", "1 set", 270, 380, "planters", "Lucknow Decor"),
  catalogItem("SIDETBL-01", "Mango Wood Round Side Table (Black)", "furniture", "Comfort Curators", "1 table", 1_990, 2_800, "side_tables", "Lucknow Woodworks"),
  catalogItem("SIDETBL-02", "Dark Brown Round Side Table", "furniture", "Kate and Laurel", "1 table", 1_780, 2_500, "side_tables", "Kate and Laurel"),
  catalogItem("PLANTSH-01", "Corner Wooden Plant Shelves", "furniture", "Comfort Curators", "1 shelf", 2_280, 3_200, "plant_shelves", "Lucknow Woodworks", "owner_preferred"),
  catalogItem("PLANTSH-02", "Freestanding Wooden Plant Display Shelf", "furniture", "Comfort Curators", "1 shelf", 1_990, 2_800, "plant_shelves", "Lucknow Woodworks", "owner_preferred"),
  catalogItem("PLANTSH-03", "Tall Multi-Tier Wooden Plant Stand", "furniture", "Comfort Curators", "1 stand", 1_280, 1_800, "plant_shelves", "Lucknow Woodworks"),
  catalogItem("HOOK-01", "Brushed Gold Four Wall Hooks", "furniture", "Comfort Curators", "4 hooks", 200, 280, "wall_hooks", "Lucknow Decor"),
  catalogItem("LAMP-02", "Brown Pleated Floor Lamp", "furniture", "Comfort Curators", "1 lamp", 1_560, 2_200, "floor_lighting", "Lucknow Decor"),
  catalogItem("LAMP-03", "Silver Arc Floor Lamp", "furniture", "Comfort Curators", "1 lamp", 2_700, 3_800, "floor_lighting", "Lucknow Decor", "owner_preferred"),
  catalogItem("CURTAIN-01", "Natural Linen Curtains (2-Panel)", "window", "Mysky Home", "2 panels", 1_070, 1_500, "curtains", "Mysky Home"),
  catalogItem("CURTAIN-02", "Gray Blackout Curtains (2-Panel)", "window", "NICETOWN", "2 panels", 850, 1_200, "curtains", "NICETOWN"),
  catalogItem("CURTAIN-03", "Sage Green Blackout Curtains (2-Panel)", "window", "Simplebrand", "2 panels", 850, 1_200, "curtains", "Simplebrand"),
  catalogItem("CURTAIN-04", "Natural Linen Semi-Sheer Curtains (2-Panel)", "window", "Twodrapes", "2 panels", 960, 1_350, "curtains", "Twodrapes"),
];

function catalogItem(
  sku: string,
  name: string,
  category: string,
  brand: string,
  packSize: string,
  unitCostRupees: number,
  ownerPriceRupees: number,
  substitutionGroup: string,
  supplier: string,
  label: CatalogItem["label"] = "curators_standard",
  shelfLifeRule = "none",
): CatalogItem {
  return {
    sku,
    name,
    category,
    brand,
    pack_size: packSize,
    unit_cost_minor_units: unitCostRupees * 100,
    unit_cost_currency: "INR",
    owner_price_minor_units: ownerPriceRupees * 100,
    owner_price_currency: "INR",
    tax_class: "gst_5",
    supplier,
    country_of_origin: "IN",
    status: "active",
    shelf_life_rule: shelfLifeRule,
    substitution_group: substitutionGroup,
    operational_suitability: "high",
    label,
  };
}

async function seedCatalog() {
  step("Seeding the catalog");
  const existing = await request<Collection<CatalogItem>>("/v1/catalog/items");
  const bySku = new Map(existing.items.map((item) => [item.data.sku, item]));

  for (const item of catalogSeed) {
    const found = bySku.get(item.sku);
    if (found) {
      count("reused", "catalog_items");
      detail(`reused ${item.sku}`);
      continue;
    }
    const created = await request<Resource<CatalogItem>>("/v1/catalog/items", {
      method: "POST",
      body: item,
    });
    bySku.set(item.sku, created);
    count("created", "catalog_items");
    detail(`created ${item.sku}`);
  }

  return bySku;
}

const propertySeed = [
  {
    label: "Gomti Riverside 2BHK",
    idempotencyKey: "comfort-curators-demo-gomti-riverside-v1",
    calendarFeedUrl:
      process.env.CC_DEMO_ICAL_URL ??
      "http://host.docker.internal:3000/demo.ics",
    address: {
      line1: "12 Gomti Nagar Extension",
      line2: "Gomti Riverside 2BHK",
      city: "Lucknow",
      state: "UP",
      postal_code: "226010",
      country: "IN",
    },
    maximumOccupancy: 4,
  },
  {
    label: "Hazratganj Studio",
    idempotencyKey: "comfort-curators-demo-hazratganj-studio-v1",
    calendarFeedUrl:
      process.env.CC_DEMO_HAZRATGANJ_ICAL_URL ??
      "http://host.docker.internal:3000/demo-hazratganj.ics",
    address: {
      line1: "4/22 Hazratganj",
      line2: "Hazratganj Studio",
      city: "Lucknow",
      state: "UP",
      postal_code: "226001",
      country: "IN",
    },
    maximumOccupancy: 2,
  },
] as const;

async function seedProperties() {
  step("Seeding and activating two Lucknow properties");
  const existing = await request<Collection<PropertyData>>("/v1/properties");
  const results: Array<Resource<PropertyData>> = [];

  for (const seed of propertySeed) {
    let property = existing.items.find(
      (item) =>
        item.data.service_address.line1.toLocaleLowerCase("en-IN") ===
          seed.address.line1.toLocaleLowerCase("en-IN") &&
        item.data.service_address.postal_code === seed.address.postal_code,
    );

    if (!property) {
      property = await request<Resource<PropertyData>>("/v1/properties", {
        method: "POST",
        body: {
          idempotency_key: seed.idempotencyKey,
          tenant_id: TENANT_ID,
          owner_authority_id: OWNER_AUTHORITY_ID,
          service_address: seed.address,
          timezone: "Asia/Kolkata",
          maximum_occupancy: seed.maximumOccupancy,
          emergency_contacts: [
            {
              name: "Comfort Curators Lucknow Desk",
              phone: "+91 522 400 2026",
              role: "operations",
            },
          ],
        },
      });
      count("created", "properties");
      detail(`created ${seed.label}`);
    } else {
      count("reused", "properties");
      detail(`reused ${seed.label}`);
    }

    property = await activateProperty(property, seed.label);
    results.push(property);
  }

  return results;
}

// document_type must be one of the backend's real allowlist (see
// internal/documents/models.go's validDocTypes): agreement,
// compliance_cert, insurance_policy, inspection_report, government_id,
// property_deed, tax_document, evidence_photo, other. Confirmed live --
// P9.14's original values (fire_safety_report, appliance_warranty,
// guest_registration, society_noc, electrical_safety_report,
// inventory_register) were plausible-sounding but not real backend
// values and 422'd on the very first live seed run.
const documentSeed = [
  [
    { documentType: "agreement", title: "Owner Service Agreement" },
    { documentType: "compliance_cert", title: "Annual Compliance Certificate" },
    { documentType: "tax_document", title: "FY 2026 Property Tax Record" },
    { documentType: "inspection_report", title: "Fire Safety Equipment Inspection" },
    { documentType: "other", title: "Appliance Warranty Register" },
    { documentType: "compliance_cert", title: "Guest Registration Compliance Record" },
  ],
  [
    { documentType: "insurance_policy", title: "Home Insurance Policy" },
    { documentType: "inspection_report", title: "Move-in Inspection Report" },
    { documentType: "property_deed", title: "Registered Property Deed" },
    { documentType: "other", title: "Resident Association Hosting NOC" },
    { documentType: "inspection_report", title: "Electrical Safety Inspection" },
    { documentType: "other", title: "Furnishing and Appliance Inventory" },
  ],
] as const;

function propertyDocumentName(property: Resource<PropertyData>) {
  return property.data.service_address.line2 ?? property.data.service_address.line1;
}

async function seedDocuments(properties: Array<Resource<PropertyData>>) {
  step("Seeding property document records");

  for (const [index, property] of properties.entries()) {
    const existing = await request<Collection<DocumentData>>(
      `/v1/properties/${property.id}/documents`,
    );
    // Unlike every other list endpoint this file reads, documents' GET
    // returns a JSON `null` (not `[]`) for zero results -- a nil Go slice
    // serialized as-is. Guard defensively rather than assume the same
    // "always an array" contract the rest of this file relies on.
    const existingTitles = new Set((existing.items ?? []).map((document) => document.data.title));
    const propertyName = propertyDocumentName(property);

    for (const seed of documentSeed[index] ?? []) {
      const title = `${seed.title} — ${propertyName}`;
      if (existingTitles.has(title)) {
        count("reused", "documents");
        detail(`reused ${title}`);
        continue;
      }

      await request<Resource<DocumentData>>("/v1/documents", {
        method: "POST",
        body: {
          title,
          document_type: seed.documentType,
          property_id: property.id,
        },
      });
      existingTitles.add(title);
      count("created", "documents");
      detail(`created ${title}`);
    }
  }
}

function isManagedProperty(property: Resource<PropertyData>) {
  return propertySeed.some(
    (seed) =>
      property.data.service_address.line1.toLocaleLowerCase("en-IN") ===
        seed.address.line1.toLocaleLowerCase("en-IN") &&
      property.data.service_address.postal_code === seed.address.postal_code,
  );
}

async function assertOwnerCanSeeExistingSeedProperties(
  staffVisible: Array<Resource<PropertyData>>,
) {
  const staffSeedProperties = staffVisible.filter(isManagedProperty);
  if (staffSeedProperties.length === 0) return;

  const ownerVisible = await request<Collection<PropertyData>>("/v1/properties");
  const ownerIds = new Set(ownerVisible.items.map((property) => property.id));
  const inaccessible = staffSeedProperties.filter((property) => !ownerIds.has(property.id));
  if (inaccessible.length > 0) {
    throw new Error(
      "Existing Phase 2 properties were created by the staff actor and are invisible to owner@demo.test. Reset the demo database once, then rerun this corrected seed; duplicate properties were not created.",
    );
  }
}

async function activateProperty(
  property: Resource<PropertyData>,
  label: string,
): Promise<Resource<PropertyData>> {
  const readiness = property.data.readiness;
  if (
    !readiness.owner_contract_accepted ||
    !readiness.compliance_complete ||
    !readiness.mandatory_fields_set
  ) {
    property = await request<Resource<PropertyData>>(
      `/v1/properties/${property.id}/readiness`,
      {
        method: "PUT",
        body: {
          owner_contract_accepted: true,
          compliance_complete: true,
          mandatory_fields_set: true,
        },
      },
    );
    count("created", "readiness_updates");
  }

  const route: Partial<Record<PropertyState, PropertyState[]>> = {
    lead: ["qualifying", "onboarding", "remediation", "ready_inactive", "active"],
    qualifying: ["onboarding", "remediation", "ready_inactive", "active"],
    onboarding: ["remediation", "ready_inactive", "active"],
    remediation: ["ready_inactive", "active"],
    ready_inactive: ["active"],
    paused: ["active"],
    suspended: ["paused", "active"],
    active: [],
  };
  const transitions = route[property.data.state];
  if (!transitions) {
    throw new Error(
      `${label} is ${property.data.state}; the halted backend has no safe route back to active`,
    );
  }

  for (const toState of transitions) {
    property = await request<Resource<PropertyData>>(
      `/v1/properties/${property.id}/transitions`,
      {
        method: "POST",
        headers: { "If-Match": `"${property.version}"` },
        body: {
          idempotency_key: `seed-${property.id}-${toState}`,
          to_state: toState,
          reason: PROPERTY_TRANSITION_REASONS[toState],
          evidence_ids: [],
        },
      },
    );
    count("created", "property_transitions");
    detail(`${label}: ${toState}`);
  }

  if (transitions.length === 0) {
    count("reused", "active_properties");
  }
  return property;
}

type PackageLine = {
  sku: string;
  quantity: number;
  expectedMonthlyConsumption: number;
};

const packageSeed: PackageLine[][] = [
  [
    { sku: "TOWEL-01", quantity: 6, expectedMonthlyConsumption: 12 },
    { sku: "TOWEL-02", quantity: 6, expectedMonthlyConsumption: 12 },
    { sku: "SHEET-01", quantity: 3, expectedMonthlyConsumption: 4 },
    { sku: "PILLOW-01", quantity: 6, expectedMonthlyConsumption: 2 },
    { sku: "SOAP-01", quantity: 18, expectedMonthlyConsumption: 36 },
    { sku: "SHAMP-01", quantity: 18, expectedMonthlyConsumption: 36 },
    { sku: "TEA-01", quantity: 4, expectedMonthlyConsumption: 8 },
    { sku: "COFFEE-01", quantity: 2, expectedMonthlyConsumption: 4 },
    { sku: "WATER-01", quantity: 8, expectedMonthlyConsumption: 16 },
    { sku: "CLEAN-01", quantity: 2, expectedMonthlyConsumption: 2 },
    { sku: "CLEAN-02", quantity: 2, expectedMonthlyConsumption: 2 },
    { sku: "KIT-01", quantity: 2, expectedMonthlyConsumption: 4 },
    { sku: "LAMP-01", quantity: 2, expectedMonthlyConsumption: 0 },
    { sku: "CUSH-01", quantity: 3, expectedMonthlyConsumption: 0 },
    { sku: "FIRSTAID-01", quantity: 1, expectedMonthlyConsumption: 0 },
  ],
  [
    { sku: "TOWEL-01", quantity: 3, expectedMonthlyConsumption: 6 },
    { sku: "TOWEL-02", quantity: 3, expectedMonthlyConsumption: 6 },
    { sku: "SHEET-01", quantity: 2, expectedMonthlyConsumption: 3 },
    { sku: "PILLOW-01", quantity: 3, expectedMonthlyConsumption: 1 },
    { sku: "SOAP-01", quantity: 10, expectedMonthlyConsumption: 20 },
    { sku: "SHAMP-01", quantity: 10, expectedMonthlyConsumption: 20 },
    { sku: "TEA-01", quantity: 2, expectedMonthlyConsumption: 4 },
    { sku: "COFFEE-01", quantity: 1, expectedMonthlyConsumption: 2 },
    { sku: "WATER-01", quantity: 4, expectedMonthlyConsumption: 8 },
    { sku: "CLEAN-01", quantity: 1, expectedMonthlyConsumption: 1 },
    { sku: "CLEAN-02", quantity: 1, expectedMonthlyConsumption: 1 },
    { sku: "KIT-01", quantity: 1, expectedMonthlyConsumption: 2 },
    { sku: "LAMP-01", quantity: 1, expectedMonthlyConsumption: 0 },
    { sku: "CUSH-01", quantity: 2, expectedMonthlyConsumption: 0 },
    { sku: "FIRSTAID-01", quantity: 1, expectedMonthlyConsumption: 0 },
  ],
];

async function seedPackages(
  properties: Array<Resource<PropertyData>>,
  catalog: Map<string, Resource<CatalogItem>>,
) {
  step("Creating and activating property package versions");

  for (const [index, property] of properties.entries()) {
    const versions = await request<Collection<PackageData>>(
      `/v1/properties/${property.id}/packages`,
    );
    const active = versions.items.find((version) => version.data.status === "active");
    if (active) {
      count("reused", "active_packages");
      detail(`reused active package ${active.id} for ${propertySeed[index].label}`);
      continue;
    }

    const desiredSkus = packageSeed[index].map((line) => line.sku).sort();
    let version = versions.items.find(
      (candidate) =>
        candidate.data.status === "draft" &&
        candidate.data.items
          .map((item) => item.sku)
          .sort()
          .join(",") === desiredSkus.join(","),
    );

    if (!version) {
      const items = packageSeed[index].map((line, orderIndex) => {
        const catalogItemResource = catalog.get(line.sku);
        if (!catalogItemResource) {
          throw new Error(`Catalog item ${line.sku} is missing after the catalog seed`);
        }
        return {
          catalog_item_id: catalogItemResource.id,
          quantity: line.quantity,
          expected_monthly_consumption: line.expectedMonthlyConsumption,
          order_index: orderIndex,
        };
      });
      version = await request<Resource<PackageData>>(
        `/v1/properties/${property.id}/packages`,
        {
          method: "POST",
          body: {
            effective_date: "2026-08-09T00:00:00Z",
            substitution_policy: "owner_approval",
            require_approval_for_price_increase: true,
            require_approval_for_new_sku: true,
            items,
            bundles: [],
          },
        },
      );
      count("created", "package_versions");
    } else {
      count("reused", "draft_packages");
    }

    const activated = await request<Resource<PackageData>>(
      `/v1/properties/${property.id}/packages/${version.id}/activate`,
      { method: "POST", body: {} },
    );
    count("created", "package_activations");
    detail(
      `activated ${activated.id} for ${propertySeed[index].label} (${activated.data.currency} ${activated.data.monthly_cost_minor_units} minor units/month)`,
    );
  }
}

const workerSeed = [
  {
    legal_name: "Asha Verma",
    date_of_birth: "1996-04-12T00:00:00Z",
    contact_method: "+91 94150 12001",
    service_zone: "lucknow-central",
    skills: ["cleaning", "linen", "turnover"],
  },
  {
    legal_name: "Ravi Prakash",
    date_of_birth: "1991-09-23T00:00:00Z",
    contact_method: "+91 94150 12002",
    service_zone: "lucknow-central",
    skills: ["maintenance", "plumbing", "general"],
  },
  {
    legal_name: "Meena Kumari",
    date_of_birth: "1994-02-18T00:00:00Z",
    contact_method: "+91 94150 12003",
    service_zone: "lucknow-north",
    skills: ["cleaning", "restocking", "restock", "inventory"],
  },
] as const;

async function seedWorkers() {
  step("Seeding workers and dispatch availability");
  const existing = await request<Collection<WorkerData>>("/v1/workers");
  const workers: Array<Resource<WorkerData>> = [];

  for (const seed of workerSeed) {
    let worker = existing.items.find(
      (item) => item.data.legal_name === seed.legal_name,
    );
    if (!worker) {
      worker = await request<Resource<WorkerData>>("/v1/workers", {
        method: "POST",
        body: {
          ...seed,
          classification: "employee",
          specialist: false,
          verified_identity: true,
        },
      });
      count("created", "workers");
      detail(`created ${seed.legal_name}`);
    } else {
      count("reused", "workers");
      detail(`reused ${seed.legal_name}`);
    }

    const windows = await request<{ items: AvailabilityWindow[]; total: number }>(
      `/v1/workers/${worker.id}/availability-windows`,
    );
    if (windows.items.length === 0) {
      await request<Resource<AvailabilityWindow>>(
        `/v1/workers/${worker.id}/availability-windows`,
        {
          method: "POST",
          body: {
            day_of_week: 0,
            start_minute: 0,
            end_minute: 1439,
            effective_at: "2026-01-01T00:00:00Z",
          },
        },
      );
      count("created", "availability_windows");
    } else {
      count("reused", "availability_windows");
    }
    workers.push(worker);
  }

  return workers;
}

const ticketSeed = [
  {
    propertyIndex: 0,
    type: "turnover",
    targetStatus: "assigned",
    reason: "Complete the guest checkout turnover at Gomti Riverside",
    requested_window: {
      start: "2026-08-13T05:30:00Z",
      end: "2026-08-13T08:30:00Z",
    },
  },
  {
    propertyIndex: 0,
    type: "restock",
    targetStatus: "in_progress",
    reason: "Replenish guest supplies at Gomti Riverside before the next arrival",
    requested_window: {
      start: "2026-08-13T08:30:00Z",
      end: "2026-08-13T10:30:00Z",
    },
  },
  {
    propertyIndex: 1,
    type: "routine_maintenance",
    targetStatus: "assigned",
    reason: "Complete the preventive plumbing check at Hazratganj Studio",
    requested_window: {
      start: "2026-08-14T04:00:00Z",
      end: "2026-08-14T06:00:00Z",
    },
  },
  {
    propertyIndex: 1,
    type: "pre_arrival_inspection",
    targetStatus: "approved",
    reason: "Complete the pre-arrival quality check for the weekend guest",
    requested_window: {
      start: "2026-08-11T10:30:00Z",
      end: "2026-08-11T11:30:00Z",
    },
  },
  {
    propertyIndex: 0,
    type: "inventory_count",
    targetStatus: "scheduled",
    reason: "Count this month’s linen and guest-amenity inventory",
    requested_window: {
      start: "2026-08-12T04:30:00Z",
      end: "2026-08-12T06:00:00Z",
    },
  },
  {
    propertyIndex: 1,
    type: "document_review",
    targetStatus: "proposed",
    reason: "Review the annual insurance renewal packet before submission",
    requested_window: {
      start: "2026-08-18T06:30:00Z",
      end: "2026-08-18T07:30:00Z",
    },
  },
  {
    propertyIndex: 0,
    type: "incident",
    targetStatus: "draft",
    reason: "Assess the reported balcony door latch issue and confirm safe closure",
    requested_window: {
      start: "2026-08-03T09:00:00Z",
      end: "2026-08-03T10:00:00Z",
    },
  },
  {
    propertyIndex: 1,
    type: "specialist_vendor_request",
    targetStatus: "scheduled",
    reason: "Deep-service the air conditioner before peak occupancy",
    requested_window: {
      start: "2026-08-24T04:00:00Z",
      end: "2026-08-24T07:00:00Z",
    },
  },
  {
    propertyIndex: 0,
    type: "property_onboarding",
    targetStatus: "proposed",
    reason: "Archive the completed photography and listing handoff",
    requested_window: {
      start: "2026-07-28T05:00:00Z",
      end: "2026-07-28T08:00:00Z",
    },
  },
] as const;

async function seedTickets(
  properties: Array<Resource<PropertyData>>,
  workers: Array<Resource<WorkerData>>,
) {
  step("Seeding, scheduling, and dispatching tickets");
  const existing = await listTicketsForProperties(properties);

  for (const seed of ticketSeed) {
    let ticket = existing.items.find((item) => item.data.reason === seed.reason);
    if (!ticket) {
      ticket = await request<Resource<TicketData>>("/v1/tickets", {
        method: "POST",
        body: {
          tenant_id: TENANT_ID,
          property_id: properties[seed.propertyIndex].id,
          type: seed.type,
          requested_window: seed.requested_window,
          reason: seed.reason,
          checklist_version_id: "",
        },
      });
      count("created", "tickets");
      detail(`created ${seed.type} ticket`);
    } else {
      count("reused", "tickets");
      detail(`reused ${seed.type} ticket`);
    }

    const needsAssignment = ["assigned", "in_progress"].includes(seed.targetStatus);
    ticket = await advanceTicketTo(
      ticket,
      needsAssignment ? "scheduled" : seed.targetStatus,
    );
    if (needsAssignment) {
      await ensureTicketAssignment(ticket, workers);
      ticket = await advanceTicketTo(ticket, seed.targetStatus);
    }

    await ensureTicketChecklist(ticket);
  }
}

async function ensureTicketAssignment(
  ticket: Resource<TicketData>,
  workers: Array<Resource<WorkerData>>,
) {
  const assignments = await request<{
    items: Array<Resource<JsonRecord>> | null;
    next_cursor?: string | null;
  }>(`/v1/tickets/${ticket.id}/dispatch/assignments`);
  if ((assignments.items ?? []).length > 0) {
    count("reused", "dispatch_assignments");
    return;
  }

  const candidates = await request<DispatchCandidates>(
    `/v1/tickets/${ticket.id}/dispatch/candidates`,
    { method: "POST", body: {} },
  );
  const eligible = (candidates.data.candidates ?? [])
    .filter((candidate) => candidate.eligible)
    .sort((left, right) => right.score - left.score);
  const seededWorkerIds = new Set(workers.map((worker) => worker.id));
  const candidate =
    eligible.find((item) => seededWorkerIds.has(item.worker_id)) ?? eligible[0];
  if (!candidate) {
    throw new Error(
      `No eligible worker for ${ticket.data.type}; verify skills and availability windows`,
    );
  }
  await request<Resource<JsonRecord>>(
    `/v1/tickets/${ticket.id}/dispatch/assign`,
    { method: "POST", body: { worker_id: candidate.worker_id } },
  );
  count("created", "dispatch_assignments");
  detail(`dispatched ${ticket.data.type} to ${candidate.worker_id}`);
}

async function ensureTicketChecklist(ticket: Resource<TicketData>) {
  const existing = await request<Collection<JsonRecord>>(
    `/v1/tickets/${ticket.id}/checklist-items`,
  );
  if (existing.items.length > 0) {
    count("reused", "checklist_items");
    return;
  }

  await request<Collection<JsonRecord>>(
    `/v1/tickets/${ticket.id}/checklist-syncs`,
    {
      method: "POST",
      body: {
        items: [
          {
            template_item_index: 0,
            label: "Complete the service brief",
            status: "pending",
            evidence_required: false,
          },
          {
            template_item_index: 1,
            label: "Capture completion evidence metadata",
            status: "pending",
            evidence_required: true,
          },
        ],
      },
    },
  );
  count("created", "checklist_items");
  detail(`attached checklist to ${ticket.data.type} ticket`);
}

async function listTicketsForProperties(
  properties: Array<Resource<PropertyData>>,
): Promise<Collection<TicketData>> {
  const collections = await Promise.all(
    properties.map((property) =>
      request<Collection<TicketData>>(
        `/v1/tickets?property_id=${encodeURIComponent(property.id)}&limit=200`,
      ),
    ),
  );
  return {
    items: collections.flatMap((collection) => collection.items),
    total: collections.reduce(
      (total, collection) => total + collection.items.length,
      0,
    ),
  };
}

async function advanceTicketTo(
  ticket: Resource<TicketData>,
  targetStatus: TicketStatus,
): Promise<Resource<TicketData>> {
  const workflow: TicketStatus[] = [
    "draft",
    "proposed",
    "approved",
    "scheduled",
    "assigned",
    "in_progress",
  ];
  const currentIndex = workflow.indexOf(ticket.data.status);
  const targetIndex = workflow.indexOf(targetStatus);
  if (targetIndex < 0) {
    throw new Error(`Seed target status ${targetStatus} is not supported`);
  }
  if (["evidence_submitted", "verified", "closed"].includes(ticket.data.status)) {
    detail(`kept ${ticket.data.type} at later status ${ticket.data.status}`);
    return ticket;
  }
  if (currentIndex < 0) {
    throw new Error(
      `Ticket ${ticket.id} is ${ticket.data.status}; it cannot be safely advanced by the seed`,
    );
  }
  if (currentIndex > targetIndex) {
    detail(`kept ${ticket.data.type} at later status ${ticket.data.status}`);
    return ticket;
  }
  for (const status of workflow.slice(currentIndex + 1, targetIndex + 1)) {
    ticket = await transitionTicket(ticket.id, status);
    count("created", "ticket_transitions");
  }
  return ticket;
}

async function transitionTicket(ticketId: string, toState: TicketStatus) {
  return request<Resource<TicketData>>(`/v1/tickets/${ticketId}/transitions`, {
    method: "POST",
    body: {
      to_state: toState,
      reason: "Requested service window confirmed for operations scheduling",
      evidence_ids: [],
    },
  });
}

async function seedReservationChain(
  property: Resource<PropertyData>,
  feedUrl: string,
) {
  step(
    `Registering and polling the demo iCalendar feed for ${propertyDocumentName(property)}`,
  );
  const feeds = await request<Collection<CalendarFeedData>>(
    `/v1/properties/${property.id}/calendar-feeds`,
  );
  let feed = feeds.items.find((item) => item.data.url === feedUrl);
  if (!feed) {
    feed = await request<Resource<CalendarFeedData>>(
      `/v1/properties/${property.id}/calendar-feeds`,
      {
        method: "POST",
        body: {
          source: "airbnb",
          url: feedUrl,
          property_timezone: "Asia/Kolkata",
          stale_after_minutes: 120,
          minimum_turnaround_minutes: 180,
        },
      },
    );
    count("created", "calendar_feeds");
  } else {
    count("reused", "calendar_feeds");
  }

  if (feed.data.status !== "active") {
    feed = await request<Resource<CalendarFeedData>>(
      `/v1/calendar-feeds/${feed.id}/status`,
      { method: "PUT", body: { status: "active" } },
    );
  }

  const poll = await request<PollResponse>(
    `/v1/calendar-feeds/${feed.id}/polls`,
    { method: "POST", body: {} },
  );
  detail(
    poll.result.unchanged
      ? "calendar content unchanged; ingestion reused existing records"
      : `calendar created ${poll.result.reservations_created} reservation(s) and ${poll.result.proposals_proposed} proposal(s)`,
  );

  const reservations = await request<Collection<JsonRecord>>(
    `/v1/properties/${property.id}/reservations`,
  );
  if (reservations.items.length < 1) {
    throw new Error("Calendar poll completed but produced no reservations");
  }

  const generation = await request<ProposalGeneration>(
    `/v1/properties/${property.id}/turnover-proposals/generate`,
    { method: "POST", body: {} },
  );
  if (generation.result.skipped) {
    throw new Error(
      `Turnover generation skipped: ${generation.result.reason ?? "unknown reason"}`,
    );
  }
  const proposals = await request<Collection<JsonRecord>>(
    `/v1/properties/${property.id}/turnover-proposals`,
  );
  if (proposals.items.length < 1) {
    throw new Error("No turnover or inspection proposals exist after calendar ingestion");
  }
  detail(
    `${reservations.items.length} reservation(s), ${proposals.items.length} proposal(s); explicit generation proposed ${generation.result.proposed} and updated ${generation.result.updated}`,
  );

  return {
    reservations: reservations.items.length,
    proposals: proposals.items.length,
    pollProposed: poll.result.proposals_proposed,
    generateProposed: generation.result.proposed,
  };
}

async function verify(properties: Array<Resource<PropertyData>>) {
  step("Verifying the final demo scenario");
  const [catalog, allProperties, workers, tickets, reservationsByProperty] =
    await Promise.all([
      request<Collection<CatalogItem>>("/v1/catalog/items"),
      request<Collection<PropertyData>>("/v1/properties"),
      request<Collection<WorkerData>>("/v1/workers"),
      listTicketsForProperties(properties),
      Promise.all(
        properties.map((property) =>
          request<Collection<JsonRecord>>(
            `/v1/properties/${property.id}/reservations`,
          ),
        ),
      ),
    ]);

  const seededSkus = new Set<string>(catalogSeed.map((item) => item.sku));
  const seededReasons = new Set<string>(
    ticketSeed.map((ticket) => ticket.reason),
  );
  const seededNames = new Set<string>(
    workerSeed.map((worker) => worker.legal_name),
  );
  const seededTicketReasons = new Set(
    tickets.items
      .filter((ticket) => seededReasons.has(ticket.data.reason))
      .map((ticket) => ticket.data.reason),
  );
  const summary = {
    catalog: {
      seeded: catalog.items.filter((item) => seededSkus.has(item.data.sku)).length,
      total: catalog.items.length,
    },
    properties: {
      seeded: properties.length,
      total: allProperties.items.length,
      active: properties.filter((property) => property.data.state === "active").length,
    },
    workers: {
      seeded: workers.items.filter((worker) => seededNames.has(worker.data.legal_name))
        .length,
      total: workers.items.length,
    },
    tickets: {
      seeded: seededTicketReasons.size,
      total: tickets.items.length,
    },
    reservations: reservationsByProperty.reduce(
      (total, reservations) => total + reservations.items.length,
      0,
    ),
    reservations_by_property: reservationsByProperty.map(
      (reservations, index) => ({
        property: propertySeed[index].label,
        count: reservations.items.length,
      }),
    ),
  };

  if (
    summary.catalog.seeded !== catalogSeed.length ||
    summary.properties.seeded !== propertySeed.length ||
    summary.properties.active !== propertySeed.length ||
    summary.workers.seeded !== workerSeed.length ||
    summary.tickets.seeded !== ticketSeed.length ||
    summary.reservations_by_property.some((property) => property.count < 1)
  ) {
    throw new Error(`Seed verification failed: ${JSON.stringify(summary)}`);
  }
  return summary;
}

async function main() {
  console.log("Comfort Curators · Phase 2 demo seed");
  console.log(`Tenant: ${TENANT_ID}`);
  await preflight();
  await createStaffSession();
  const catalog = await seedCatalog();
  const staffVisibleProperties = await request<Collection<PropertyData>>("/v1/properties");
  await createOwnerSession();
  await assertOwnerCanSeeExistingSeedProperties(staffVisibleProperties.items);
  const properties = await seedProperties();
  await seedDocuments(properties);
  await createStaffSession();
  await seedPackages(properties, catalog);
  const workers = await seedWorkers();
  await seedTickets(properties, workers);
  const reservationChains = [];
  for (const [index, property] of properties.entries()) {
    reservationChains.push(
      await seedReservationChain(property, propertySeed[index].calendarFeedUrl),
    );
  }
  const summary = await verify(properties);

  console.log("\n✓ Phase 2 seed complete");
  console.log(
    JSON.stringify(
      {
        created: stats.created,
        reused: stats.reused,
        summary,
        reservation_chains: reservationChains,
      },
      null,
      2,
    ),
  );
}

main().catch((error: unknown) => {
  if (error instanceof SeedApiError) {
    console.error(
      `\n✗ Seed API error (${error.status}${error.code ? ` ${error.code}` : ""}): ${error.message}`,
    );
    if (error.requestId) {
      console.error(`  request_id: ${error.requestId}`);
    }
  } else if (error instanceof Error) {
    console.error(`\n✗ ${error.message}`);
  } else {
    console.error("\n✗ Unknown seed failure", error);
  }
  process.exitCode = 1;
});
