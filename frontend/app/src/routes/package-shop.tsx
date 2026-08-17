import {
  DndContext,
  DragOverlay,
  PointerSensor,
  TouchSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { useQuery } from "@tanstack/react-query";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
} from "react";
import { Link, Navigate, useParams, useSearchParams } from "react-router-dom";
import { PaymentBoundary, PaymentBoundaryButton, PaymentBoundaryTriggerButton } from "../components/superhost/PaymentBoundary";
import { useControlSession } from "../components/superhost/ControlSession";
import { useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import { toast } from "sonner";
import Money from "../components/money";
import Modal from "../components/ui/Modal";
import { getCatalogDescription } from "../lib/catalog-descriptions";
import { getPackageCatalogImage } from "../lib/catalog-images";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { clearPendingPurchase, getPendingPurchase, savePendingPurchase } from "../lib/pending-purchase";
import {
  activatePackage,
  createPackageDraft,
  getCatalog,
  getProperties,
  type CatalogLabel,
  type CatalogResource,
  type PackagePolicy,
  type PackageResource,
} from "../lib/api/shop";
import { getToken } from "../lib/auth/session";
import { formatMoney } from "../lib/money";
import "./package-shop.css";

type CartLine = {
  item: CatalogResource;
  quantity: number;
  monthlyUse: number;
};

type PresetEntry = { sku: string; quantity: number; monthlyUse: number };
type PresetMap = Record<string, PresetEntry[]>;

const PRESETS_KEY = (propertyId: string) => `cc_shop_presets_${propertyId}`;

function loadPresets(propertyId: string): PresetMap {
  try {
    const raw = window.localStorage.getItem(PRESETS_KEY(propertyId));
    return raw ? (JSON.parse(raw) as PresetMap) : {};
  } catch {
    return {};
  }
}

function savePresets(propertyId: string, presets: PresetMap) {
  window.localStorage.setItem(PRESETS_KEY(propertyId), JSON.stringify(presets));
}

type ContextMenuState = {
  item: CatalogResource;
  x: number;
  y: number;
  returnFocus: HTMLElement;
};

const LABELS: Array<{ value: CatalogLabel; label: string }> = [
  { value: "curators_standard", label: "curators' standard" },
  { value: "owner_preferred", label: "owner preferred" },
  { value: "alternative", label: "alternative" },
];

const PRODUCT_MARKS: Record<string, string> = {
  linen: "LN",
  bath: "BT",
  kitchen: "KT",
  pantry: "PN",
  cleaning: "CL",
  decor: "DC",
  safety: "SF",
};

const nextEffectiveDate = () => {
  const value = new Date(Date.now() + 86_400_000);
  value.setUTCHours(0, 0, 0, 0);
  return value.toISOString();
};

const parseList = (value: string | null) =>
  new Set((value ?? "").split(",").map((entry) => entry.trim()).filter(Boolean));

const clampInteger = (value: number, minimum: number, maximum = 999) =>
  Math.min(maximum, Math.max(minimum, Number.isFinite(value) ? Math.round(value) : minimum));

// UI-only default for now: decor and furniture are one-time buys, not a
// recurring monthly cost -- nobody re-buys a side table or a wall hanging
// every month. Setting monthlyUse to 0 for these categories at add-time
// only affects MonthlyCostMinorUnits (= consumption x price, computed
// server-side in catalog/service.go); SetupCostMinorUnits is quantity x
// price, entirely separate, so this cannot change the number Superhost
// treats as the budget ceiling. Human-editable afterward, same as any
// other line -- this only sets the starting default.
const NO_DEFAULT_MONTHLY_RECURRENCE = new Set(["decor", "furniture"]);
const defaultMonthlyUse = (item: CatalogResource) =>
  NO_DEFAULT_MONTHLY_RECURRENCE.has(item.data.category.toLowerCase()) ? 0 : 1;

function DropArea({
  id,
  className,
  label,
  children,
}: {
  id: string;
  className: string;
  label: string;
  children: ReactNode;
}) {
  const { setNodeRef, isOver } = useDroppable({ id });
  return (
    // data-lenis-prevent: both callers (.shop-results, .shop-cart-column)
    // are scrollable regions -- without this opt-out from the site's
    // global Lenis smooth-scroll, wheel input over either scrolled the
    // whole page instead of the region under the pointer.
    <div ref={setNodeRef} className={className} data-agent-drop-zone={id} data-over={isOver} data-lenis-prevent role="region" aria-label={label}>
      {children}
    </div>
  );
}

function MobileCartBar({
  open,
  itemCount,
  monthlyCost,
  onOpen,
}: {
  open: boolean;
  itemCount: number;
  monthlyCost: string;
  onOpen: () => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: "cart-mobile-zone" });
  return (
    <button ref={setNodeRef} className="shop-mobile-cart" data-over={isOver} type="button" aria-expanded={open} onClick={onOpen}>
      <span>{itemCount} {itemCount === 1 ? "ITEM" : "ITEMS"}</span>
      <strong>{monthlyCost} / MONTH</strong>
    </button>
  );
}

function ProductVisual({ item }: { item: CatalogResource }) {
  const photo = getPackageCatalogImage(item.data.sku);
  if (photo) {
    return (
      <div className="shop-product-visual" aria-hidden="true">
        <img className="shop-product-photo" src={photo.src} alt="" loading="lazy" />
        <em className="shop-product-classification">{photo.label}</em>
        <small>{item.data.sku}</small>
      </div>
    );
  }
  const mark = PRODUCT_MARKS[item.data.category.toLowerCase()] ?? item.data.category.slice(0, 2);
  return (
    <div className="shop-product-visual" aria-hidden="true">
      <span>{mark.toUpperCase()}</span>
      <small>{item.data.sku}</small>
    </div>
  );
}

function ProductCard({
  item,
  quantity,
  justAdded,
  onAdd,
  onOpen,
  onContextMenu,
}: {
  item: CatalogResource;
  quantity: number;
  justAdded: boolean;
  onAdd: (item: CatalogResource) => void;
  onOpen: (item: CatalogResource, trigger: HTMLElement) => void;
  onContextMenu: (event: ReactMouseEvent<HTMLElement>, item: CatalogResource) => void;
}) {
  // Price + category are inlined into the label itself, not just shown
  // visually -- this is literally the only per-item data Superhost ever
  // receives (see sendSuperhostMessage's uiSurfaces: {id, label, actions}
  // only, no separate price field). Without this, a stated budget like
  // "under 3000 INR" was unenforceable in practice: the model had no way
  // to know what anything cost while deciding what to add.
  const addSurface = useAgentSurface(
    `shop-catalog-add-${item.id}`,
    ["click"] as AgentAction[],
    `Drag ${item.data.name} (${formatMoney(item.data.owner_price_minor_units, item.data.owner_price_currency)} each, category: ${item.data.category}) into the package cart`,
  );
  const { listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `catalog:${item.id}`,
    data: { kind: "catalog", item },
  });
  const { onPointerDown: pointerActivator, ...touchSafeListeners } = listeners ?? {};
  const style = transform
    ? ({ transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` } as CSSProperties)
    : undefined;

  function handleKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onOpen(item, event.currentTarget);
    }
    if (event.key === "ContextMenu" || (event.shiftKey && event.key === "F10")) {
      event.preventDefault();
      const rect = event.currentTarget.getBoundingClientRect();
      onContextMenu(
        {
          preventDefault: () => undefined,
          clientX: rect.left + 18,
          clientY: rect.top + 18,
          currentTarget: event.currentTarget,
        } as ReactMouseEvent<HTMLElement>,
        item,
      );
    }
  }

  return (
    <article
      ref={setNodeRef}
      className="shop-product-card"
      data-in-cart={quantity > 0}
      data-just-added={justAdded}
      data-dragging={isDragging}
      style={style}
      {...touchSafeListeners}
      onPointerDown={(event) => {
        if (event.pointerType !== "touch") pointerActivator?.(event);
      }}
      onContextMenu={(event) => onContextMenu(event, item)}
    >
      {quantity > 0 && <span className="shop-quantity-badge">{quantity}</span>}
      <div className="shop-product-open" role="button" tabIndex={0} onClick={(event) => onOpen(item, event.currentTarget)} onKeyDown={handleKeyDown} aria-label={`View ${item.data.name}`}>
        <ProductVisual item={item} />
        <span className="shop-product-copy">
          <strong>{item.data.name}</strong>
          <span>{item.data.pack_size} · {item.data.category}</span>
          <Money value={item.data.owner_price_minor_units} currency={item.data.owner_price_currency} />
        </span>
      </div>
      <button
        ref={addSurface.ref}
        className="shop-add-control"
        data-agent-drag-target="cart-zone"
        type="button"
        aria-label={`Add ${item.data.name} to package${quantity ? `; ${quantity} currently added` : ""}`}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={(event) => { event.stopPropagation(); onAdd(item); }}
      >
        {justAdded ? "JUST ADDED" : quantity > 0 ? `ADD + · ${quantity}` : "ADD +"}
      </button>
    </article>
  );
}

function CartRow({
  line,
  active,
  exiting,
  onChange,
  onRemove,
}: {
  line: CartLine;
  active: boolean;
  exiting: boolean;
  onChange: (patch: Partial<Pick<CartLine, "quantity" | "monthlyUse">>) => void;
  onRemove: () => void;
}) {
  const unitPriceLabel = `${formatMoney(line.item.data.owner_price_minor_units, line.item.data.owner_price_currency)} each, category: ${line.item.data.category}`;
  const removeSurface = useAgentSurface(`shop-cart-remove-${line.item.id}`, ["click"] as AgentAction[], `Remove ${line.item.data.name} (${unitPriceLabel}, currently ${line.quantity} in cart) from the package`);
  const quantitySurface = useAgentSurface(`shop-cart-qty-${line.item.id}`, ["focus", "set"] as AgentAction[], `Set quantity for ${line.item.data.name} (${unitPriceLabel}, currently ${line.quantity})`);
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `cart:${line.item.id}`,
    data: { kind: "cart", item: line.item },
  });
  const { onPointerDown: pointerActivator, ...touchSafeListeners } = listeners ?? {};
  const style = transform
    ? ({ transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` } as CSSProperties)
    : undefined;

  return (
    <article
      ref={setNodeRef}
      className="shop-cart-row"
      data-active={active}
      data-dragging={isDragging}
      data-exiting={exiting}
      style={style}
      {...touchSafeListeners}
      {...attributes}
      role="group"
      aria-label={`${line.item.data.name} package line`}
      onPointerDown={(event) => {
        if (event.pointerType !== "touch") pointerActivator?.(event);
      }}
    >
      <div className="shop-cart-row-heading">
        {(() => {
          const photo = getPackageCatalogImage(line.item.data.sku);
          return photo
            ? <img className="shop-cart-row-thumb" src={photo.src} alt="" loading="lazy" />
            : <span className="shop-cart-row-thumb shop-cart-row-thumb--empty" aria-hidden="true" />;
        })()}
        <strong>{line.item.data.name}</strong>
        <button ref={removeSurface.ref} type="button" onPointerDown={(event) => event.stopPropagation()} onClick={onRemove} aria-label={`Remove ${line.item.data.name}`}>×</button>
      </div>
      <span className="shop-cart-unit"><Money value={line.item.data.owner_price_minor_units} currency={line.item.data.owner_price_currency} /> EA</span>
      <div className="shop-cart-control">
        <span>QTY</span>
        <button type="button" onPointerDown={(event) => event.stopPropagation()} onClick={() => onChange({ quantity: clampInteger(line.quantity - 1, 1) })} aria-label={`Decrease ${line.item.data.name} quantity`}>−</button>
        <input
          ref={quantitySurface.ref}
          aria-label={`Quantity for ${line.item.data.name}`}
          inputMode="numeric"
          min="1"
          type="number"
          value={line.quantity}
          onPointerDown={(event) => event.stopPropagation()}
          onChange={(event) => onChange({ quantity: clampInteger(event.currentTarget.valueAsNumber, 1) })}
        />
        <button type="button" onPointerDown={(event) => event.stopPropagation()} onClick={() => onChange({ quantity: clampInteger(line.quantity + 1, 1) })} aria-label={`Increase ${line.item.data.name} quantity`}>+</button>
      </div>
      <label className="shop-cart-control shop-monthly-control">
        <span>MONTHLY USE</span>
        <input
          aria-label={`Monthly use for ${line.item.data.name}`}
          inputMode="numeric"
          min="0"
          type="number"
          value={line.monthlyUse}
          onPointerDown={(event) => event.stopPropagation()}
          onChange={(event) => onChange({ monthlyUse: clampInteger(event.currentTarget.valueAsNumber, 0) })}
        />
      </label>
    </article>
  );
}

// The big-picture cart view (see CartModal below) reuses this row's exact
// markup and CSS classes -- same theme, same controls -- but is its own
// component rather than a second render of CartRow. CartRow calls
// useAgentSurface with an id keyed only by the item (shop-cart-remove-X,
// shop-cart-qty-X); mounting two live instances of that same id at once
// (one in the sidebar, one in an open modal) would make the second one's
// unmount silently unregister the surface the first is still using. This
// is purely presentational for a person reviewing the cart, so it never
// needs an agent surface of its own -- Superhost already has the real
// ones in the sidebar.
function CartModalRow({
  line,
  onChange,
  onRemove,
}: {
  line: CartLine;
  onChange: (patch: Partial<Pick<CartLine, "quantity" | "monthlyUse">>) => void;
  onRemove: () => void;
}) {
  return (
    <article className="shop-cart-row" role="group" aria-label={`${line.item.data.name} package line`}>
      <div className="shop-cart-row-heading">
        {(() => {
          const photo = getPackageCatalogImage(line.item.data.sku);
          return photo
            ? <img className="shop-cart-row-thumb" src={photo.src} alt="" loading="lazy" />
            : <span className="shop-cart-row-thumb shop-cart-row-thumb--empty" aria-hidden="true" />;
        })()}
        <strong>{line.item.data.name}</strong>
        <button type="button" onClick={onRemove} aria-label={`Remove ${line.item.data.name}`}>×</button>
      </div>
      <span className="shop-cart-unit"><Money value={line.item.data.owner_price_minor_units} currency={line.item.data.owner_price_currency} /> EA</span>
      <div className="shop-cart-control">
        <span>QTY</span>
        <button type="button" onClick={() => onChange({ quantity: clampInteger(line.quantity - 1, 1) })} aria-label={`Decrease ${line.item.data.name} quantity`}>−</button>
        <input
          aria-label={`Quantity for ${line.item.data.name}`}
          inputMode="numeric"
          min="1"
          type="number"
          value={line.quantity}
          onChange={(event) => onChange({ quantity: clampInteger(event.currentTarget.valueAsNumber, 1) })}
        />
        <button type="button" onClick={() => onChange({ quantity: clampInteger(line.quantity + 1, 1) })} aria-label={`Increase ${line.item.data.name} quantity`}>+</button>
      </div>
      <label className="shop-cart-control shop-monthly-control">
        <span>MONTHLY USE</span>
        <input
          aria-label={`Monthly use for ${line.item.data.name}`}
          inputMode="numeric"
          min="0"
          type="number"
          value={line.monthlyUse}
          onChange={(event) => onChange({ monthlyUse: clampInteger(event.currentTarget.valueAsNumber, 0) })}
        />
      </label>
    </article>
  );
}

function CartModal({
  open,
  onClose,
  cart,
  currency,
  monthlyTotalMinorUnits,
  onChangeLine,
  onRemoveLine,
  onClearAll,
}: {
  open: boolean;
  onClose: () => void;
  cart: CartLine[];
  currency: string;
  monthlyTotalMinorUnits: number | null;
  onChangeLine: (itemId: string, patch: Partial<Pick<CartLine, "quantity" | "monthlyUse">>) => void;
  onRemoveLine: (itemId: string) => void;
  onClearAll: () => void;
}) {
  const totalQuantity = cart.reduce((total, line) => total + line.quantity, 0);
  // Resets naturally on its own -- a fresh boolean each time the modal
  // mounts, no need to wire this into the parent's own state.
  const [confirmClear, setConfirmClear] = useState(false);

  return (
    <Modal
      open={open}
      onClose={() => { setConfirmClear(false); onClose(); }}
      title="Your package"
      label={`CART / ${totalQuantity} ITEM${totalQuantity === 1 ? "" : "S"}`}
      className="shop-cart-modal"
    >
      {cart.length === 0 ? (
        <div className="shop-empty-cart"><strong>empty.</strong><em>for now.</em></div>
      ) : (
        <>
          <div className="shop-cart-modal-actions">
            {confirmClear ? (
              <div className="shop-cart-modal-clear-confirm">
                CLEAR EVERYTHING?
                <button type="button" onClick={() => { onClearAll(); setConfirmClear(false); }}>OK</button>
                <button type="button" onClick={() => setConfirmClear(false)}>—</button>
              </div>
            ) : (
              <button type="button" onClick={() => setConfirmClear(true)}>CLEAR ALL ×</button>
            )}
          </div>
          <div className="shop-cart-modal-rows">
            {cart.map((line) => (
              <CartModalRow
                key={line.item.id}
                line={line}
                onChange={(patch) => onChangeLine(line.item.id, patch)}
                onRemove={() => onRemoveLine(line.item.id)}
              />
            ))}
          </div>
          <div className="shop-cart-modal-total">
            <span>MONTHLY TOTAL</span>
            <strong>{monthlyTotalMinorUnits !== null ? <Money value={monthlyTotalMinorUnits} currency={currency} /> : "—"}</strong>
          </div>
        </>
      )}
    </Modal>
  );
}

function QuickView({
  item,
  onClose,
}: {
  item: CatalogResource | null;
  onClose: () => void;
}) {
  if (!item) return null;
  const description = getCatalogDescription(item.data.sku);
  return (
    <Modal open={Boolean(item)} onClose={onClose} title={item.data.name} label={`QUICK VIEW / ${item.data.sku}`} className="shop-quick-view">
      <ProductVisual item={item} />
      <div>
        <p>{item.data.brand} · {item.data.pack_size} · {item.data.category}</p>
        {description && <p className="shop-quick-description">{description}</p>}
        <Money value={item.data.owner_price_minor_units} currency={item.data.owner_price_currency} />
        <small>{item.data.label.replaceAll("_", " ")}</small>
      </div>
    </Modal>
  );
}

export default function PackageShop() {
  const { propertyId = "" } = useParams();
  const { session: controlSession } = useControlSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const searchRef = useRef<HTMLInputElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const effectiveDate = useRef(nextEffectiveDate());
  const saveSequence = useRef(0);
  const lastDragEndedAt = useRef(0);
  const markerTimer = useRef<number | null>(null);
  const justAddedTimer = useRef<number | null>(null);
  const exitTimers = useRef<number[]>([]);
  const firstDropPending = useRef(false);
  const [cart, setCart] = useState<CartLine[]>([]);
  const [cartHydrated, setCartHydrated] = useState(false);
  const [exitingIds, setExitingIds] = useState<Set<string>>(() => new Set());
  const [policy, setPolicy] = useState<PackagePolicy>("owner_approval");
  const [approvePriceIncrease, setApprovePriceIncrease] = useState(true);
  const [approveNewSku, setApproveNewSku] = useState(true);
  const [monthlyBudget, setMonthlyBudget] = useState("");
  const [savedSignature, setSavedSignature] = useState("");
  const [packageVersion, setPackageVersion] = useState<PackageResource | null>(null);
  const [costs, setCosts] = useState<PackageResource["data"] | null>(null);
  const [settling, setSettling] = useState(false);
  const [activating, setActivating] = useState(false);
  const [activeDrag, setActiveDrag] = useState<CatalogResource | null>(null);
  const [markerVisible, setMarkerVisible] = useState(false);
  const [justAddedId, setJustAddedId] = useState<string | null>(null);
  const [filterOpen, setFilterOpen] = useState(false);
  const [cartOpen, setCartOpen] = useState(false);
  const [cartModalOpen, setCartModalOpen] = useState(false);
  const [offline, setOffline] = useState(() => !navigator.onLine);
  const [reducedMotion, setReducedMotion] = useState(() => window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [quickView, setQuickView] = useState<CatalogResource | null>(null);
  const [presets, setPresets] = useState<PresetMap>(() => loadPresets(propertyId));
  const [showPresetSave, setShowPresetSave] = useState(false);
  const [presetSaveName, setPresetSaveName] = useState("");
  const [swapConfirmPreset, setSwapConfirmPreset] = useState<string | null>(null);

  const catalogQuery = useQuery({ queryKey: ["catalog"], queryFn: getCatalog });
  const propertiesQuery = useQuery({ queryKey: ["properties"], queryFn: getProperties });
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 6 } }),
  );

  const allItems = useMemo(
    () => (catalogQuery.data?.items ?? []).filter((item) => item.data.status === "active"),
    [catalogQuery.data],
  );
  const property = propertiesQuery.data?.items.find((entry) => entry.id === propertyId);

  useEffect(() => {
    if (property) {
      const label = property.data.service_address.line2 ?? property.data.service_address.line1;
      setCurrentProperty({ id: property.id, label });
    } else {
      clearCurrentProperty();
    }
    return clearCurrentProperty;
  }, [property]);

  useEffect(() => {
    setPresets(loadPresets(propertyId));
    setShowPresetSave(false);
    setPresetSaveName("");
    setSwapConfirmPreset(null);
  }, [propertyId]);

  useEffect(() => {
    setCart([]);
    setCartHydrated(false);
  }, [propertyId]);

  useEffect(() => {
    if (cartHydrated || allItems.length === 0) return;
    const restored = getPendingPurchase(propertyId).flatMap((pending) => {
      const item = allItems.find((candidate) =>
        candidate.id === pending.catalogItemId || candidate.data.sku === pending.sku,
      );
      return item ? [{
        item,
        quantity: clampInteger(pending.quantity, 1),
        monthlyUse: clampInteger(pending.monthlyUse ?? pending.quantity, 0),
      }] : [];
    });
    setCart(restored);
    setCartHydrated(true);
  }, [allItems, cartHydrated, propertyId]);

  useEffect(() => {
    if (!cartHydrated) return;
    if (packageVersion?.data.status === "active") {
      clearPendingPurchase(propertyId);
      return;
    }
    savePendingPurchase(propertyId, cart.map((line) => ({
      catalogItemId: line.item.id,
      name: line.item.data.name,
      sku: line.item.data.sku,
      quantity: line.quantity,
      monthlyUse: line.monthlyUse,
      unitPriceMinorUnits: line.item.data.owner_price_minor_units,
      currency: line.item.data.owner_price_currency,
      draftPackageId: packageVersion?.data.status === "draft" ? packageVersion.id : undefined,
    })));
  }, [cart, cartHydrated, packageVersion, propertyId]);

  function handleSavePreset() {
    const name = presetSaveName.trim();
    if (!name || cart.length === 0) return;
    const entry: PresetEntry[] = cart.map((line) => ({
      sku: line.item.data.sku,
      quantity: line.quantity,
      monthlyUse: line.monthlyUse,
    }));
    const next: PresetMap = { ...presets, [name]: entry };
    setPresets(next);
    savePresets(propertyId, next);
    setShowPresetSave(false);
    setPresetSaveName("");
  }

  function handleLoadPreset(name: string) {
    const entry = presets[name];
    if (!entry) return;
    const catalogItems = allItems;
    const lines: CartLine[] = [];
    for (const row of entry) {
      const item = catalogItems.find((i) => i.data.sku === row.sku);
      if (!item) continue;
      const existing = lines.find((l) => l.item.id === item.id);
      if (existing) {
        existing.quantity += row.quantity;
      } else {
        lines.push({ item, quantity: row.quantity, monthlyUse: row.monthlyUse });
      }
    }
    setCart(lines);
    setSwapConfirmPreset(null);
  }

  function handleDeletePreset(name: string) {
    const next = { ...presets };
    delete next[name];
    setPresets(next);
    savePresets(propertyId, next);
  }

  function requestLoadPreset(name: string) {
    if (cart.length > 0) {
      setSwapConfirmPreset(name);
    } else {
      handleLoadPreset(name);
    }
  }
  const categories = useMemo(
    () => Array.from(new Set(allItems.map((item) => item.data.category))).sort(),
    [allItems],
  );
  const priceCeiling = useMemo(
    () => Math.max(100, ...allItems.map((item) => Math.ceil(item.data.owner_price_minor_units / 100))),
    [allItems],
  );
  const selectedCategories = parseList(searchParams.get("category"));
  const selectedLabels = parseList(searchParams.get("label"));
  const query = searchParams.get("q") ?? "";
  const minimumPrice = clampInteger(Number(searchParams.get("min") ?? 0), 0, priceCeiling);
  const maximumPrice = clampInteger(Number(searchParams.get("max") ?? priceCeiling), 0, priceCeiling);
  const hasFilters = query !== "" || selectedCategories.size > 0 || selectedLabels.size > 0 || minimumPrice > 0 || maximumPrice < priceCeiling;

  const filteredItems = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return allItems.filter((item) => {
      const price = item.data.owner_price_minor_units / 100;
      return (
        (selectedCategories.size === 0 || selectedCategories.has(item.data.category)) &&
        (selectedLabels.size === 0 || selectedLabels.has(item.data.label)) &&
        price >= Math.min(minimumPrice, maximumPrice) &&
        price <= Math.max(minimumPrice, maximumPrice) &&
        (!normalizedQuery || `${item.data.name} ${item.data.sku}`.toLowerCase().includes(normalizedQuery))
      );
    });
  }, [allItems, maximumPrice, minimumPrice, query, selectedCategories, selectedLabels]);

  const signature = useMemo(
    () => JSON.stringify({
      policy,
      approvePriceIncrease,
      approveNewSku,
      items: cart.map((line) => [line.item.id, line.quantity, line.monthlyUse]),
    }),
    [approveNewSku, approvePriceIncrease, cart, policy],
  );

  useEffect(() => {
    const handleOnline = () => setOffline(false);
    const handleOffline = () => setOffline(true);
    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOffline);
    return () => {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOffline);
    };
  }, []);

  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(query.matches);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    function focusSearch(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      if (event.key === "/" && !target?.matches("input, textarea, select, [contenteditable='true']")) {
        event.preventDefault();
        searchRef.current?.focus();
      }
    }
    window.addEventListener("keydown", focusSearch);
    return () => window.removeEventListener("keydown", focusSearch);
  }, []);

  useEffect(() => {
    if (!contextMenu) return;
    const firstButton = menuRef.current?.querySelector<HTMLButtonElement>("button");
    firstButton?.focus();
    const close = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setContextMenu(null);
    };
    window.addEventListener("pointerdown", close);
    return () => window.removeEventListener("pointerdown", close);
  }, [contextMenu]);

  useEffect(() => {
    if (cart.length === 0) {
      setSettling(false);
      setSavedSignature("");
      setPackageVersion(null);
      setCosts(null);
      return;
    }
    if (offline) {
      setSettling(false);
      return;
    }

    setSettling(true);
    const sequence = ++saveSequence.current;
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      try {
        const result = await createPackageDraft(
          propertyId,
          {
            effective_date: effectiveDate.current,
            substitution_policy: policy,
            require_approval_for_price_increase: approvePriceIncrease,
            require_approval_for_new_sku: approveNewSku,
            items: cart.map((line, orderIndex) => ({
              catalog_item_id: line.item.id,
              quantity: line.quantity,
              expected_monthly_consumption: line.monthlyUse,
              order_index: orderIndex,
            })),
            bundles: [],
          },
          controller.signal,
        );
        if (sequence !== saveSequence.current) return;
        setPackageVersion(result);
        setCosts(result.data);
        setSavedSignature(signature);
        if (firstDropPending.current) {
          firstDropPending.current = false;
          if (sessionStorage.getItem("cc_shop_first_drop") !== "seen") {
            sessionStorage.setItem("cc_shop_first_drop", "seen");
            setMarkerVisible(true);
            if (markerTimer.current) window.clearTimeout(markerTimer.current);
            markerTimer.current = window.setTimeout(() => setMarkerVisible(false), 1200);
          }
        }
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") return;
      } finally {
        if (sequence === saveSequence.current) setSettling(false);
      }
    }, 400);

    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [approveNewSku, approvePriceIncrease, cart, offline, policy, propertyId, signature]);

  useEffect(() => () => {
    if (markerTimer.current) window.clearTimeout(markerTimer.current);
    if (justAddedTimer.current) window.clearTimeout(justAddedTimer.current);
    exitTimers.current.forEach((timer) => window.clearTimeout(timer));
  }, []);

  function updateSearchParam(key: string, values: Set<string> | string | number | null) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      const value = values instanceof Set ? Array.from(values).sort().join(",") : values === null ? "" : String(values);
      if (!value || (key === "min" && value === "0") || (key === "max" && value === String(priceCeiling))) next.delete(key);
      else next.set(key, value);
      return next;
    }, { replace: true });
  }

  function toggleSetParam(key: string, value: string, current: Set<string>) {
    const next = new Set(current);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    updateSearchParam(key, next);
  }

  function addToCart(item: CatalogResource) {
    setJustAddedId(item.id);
    if (justAddedTimer.current) window.clearTimeout(justAddedTimer.current);
    justAddedTimer.current = window.setTimeout(() => setJustAddedId(null), 1100);
    setCart((current) => {
      const existing = current.find((line) => line.item.id === item.id);
      if (existing) return current.map((line) => line.item.id === item.id ? { ...line, quantity: line.quantity + 1 } : line);
      if (current.length === 0) firstDropPending.current = true;
      return [...current, { item, quantity: 1, monthlyUse: defaultMonthlyUse(item) }];
    });
  }

  function removeFromCart(itemId: string) {
    if (exitingIds.has(itemId)) return;
    setExitingIds((current) => new Set(current).add(itemId));
    const timer = window.setTimeout(() => {
      setCart((current) => current.filter((line) => line.item.id !== itemId));
      setExitingIds((current) => {
        const next = new Set(current);
        next.delete(itemId);
        return next;
      });
    }, 180);
    exitTimers.current.push(timer);
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveDrag(event.active.data.current?.item as CatalogResource | null);
  }

  function handleDragEnd(event: DragEndEvent) {
    const kind = event.active.data.current?.kind;
    const item = event.active.data.current?.item as CatalogResource | undefined;
    if (item && kind === "catalog" && (event.over?.id === "cart-zone" || event.over?.id === "cart-mobile-zone")) addToCart(item);
    if (item && kind === "cart" && event.over?.id === "grid-zone") removeFromCart(item.id);
    lastDragEndedAt.current = performance.now();
    setActiveDrag(null);
  }

  function openContextMenu(event: ReactMouseEvent<HTMLElement>, item: CatalogResource) {
    event.preventDefault();
    event.stopPropagation();
    setContextMenu({ item, x: event.clientX, y: event.clientY, returnFocus: event.currentTarget });
  }

  function openQuickView(item: CatalogResource) {
    if (performance.now() - lastDragEndedAt.current < 250) return;
    setQuickView(item);
  }

  async function activate() {
    if (!packageVersion || savedSignature !== signature || packageVersion.data.status !== "draft") return;
    setActivating(true);
    try {
      const active = await activatePackage(propertyId, packageVersion.id);
      setPackageVersion(active);
      setCosts(active.data);
      clearPendingPurchase(propertyId);
      toast.success("Package active");
    } finally {
      setActivating(false);
    }
  }

  if (!getToken()) return <Navigate to="/login" replace />;
  if (propertiesQuery.isSuccess && !property) return <Navigate to="/login" replace />;

  const title = property?.data.service_address.line2 ?? property?.data.service_address.line1 ?? "PROPERTY INVENTORY";
  const cartCount = cart.reduce((total, line) => total + line.quantity, 0);
  const canActivate = cart.length > 0 && !settling && !activating && savedSignature === signature && packageVersion?.data.status === "draft";
  const displayCurrency = costs?.currency ?? "INR";
  // Read-only running total, surfaced the same way per-item prices are
  // (see ProductCard's addSurface above): the label itself IS the data --
  // there is no separate numeric field in what Superhost receives. This
  // is what lets it check its own running total against a stated budget
  // ("under 3000 INR") as it adds items, instead of adding blind.
  // SETUP cost leads and is named as the budget figure explicitly -- a
  // stated budget ("under 2000 INR") is checked against this number, never
  // the monthly one. Confirmed live: a cart landed over budget on setup
  // while its monthly total was comfortably under it, consistent with the
  // two getting mixed up when they were presented as two same-weight
  // numbers side by side.
  const cartTotalSurface = useAgentSurface(
    "shop-cart-running-total",
    ["focus"] as AgentAction[],
    costs
      ? `Package cart running total so far -- SETUP COST (this is the number a stated budget applies to): ${formatMoney(costs.setup_cost_minor_units, displayCurrency)}. Separately, monthly recurring cost (not the budget figure): ${formatMoney(costs.monthly_cost_minor_units, displayCurrency)}. ${cartCount} item${cartCount === 1 ? "" : "s"} total.`
      : "Package cart is currently empty: 0 setup cost (the budget figure), 0 monthly cost, 0 items.",
  );

  return (
    <DndContext collisionDetection={pointerWithin} sensors={sensors} onDragStart={handleDragStart} onDragCancel={() => setActiveDrag(null)} onDragEnd={handleDragEnd}>
      <main className="shop-shell registration-frame">
        <header className="shop-header">
          <span>01 / INVENTORY</span>
          <strong>{title}</strong>
          <button type="button" className="shop-cart-summary-button" onClick={() => setCartModalOpen(true)} disabled={cart.length === 0}>
            CART / {cartCount} ⤢
          </button>
        </header>

        <aside className="shop-filter-column" data-open={filterOpen} data-lenis-prevent aria-label="Catalog filters">
          <div className="shop-filter-heading">
            <strong>FILTERS</strong>
            <button type="button" onClick={() => setFilterOpen(false)}>CLOSE ×</button>
          </div>
          <label className="shop-search">
            <span>SEARCH /</span>
            <input ref={searchRef} type="search" value={query} placeholder="NAME OR SKU" onChange={(event) => updateSearchParam("q", event.currentTarget.value)} />
          </label>
          <fieldset className="shop-filter-group">
            <legend>CATEGORY</legend>
            {categories.map((category) => (
              <label key={category}>
                <input type="checkbox" checked={selectedCategories.has(category)} onChange={() => toggleSetParam("category", category, selectedCategories)} />
                <span>{category}</span>
                <b>{allItems.filter((item) => item.data.category === category).length}</b>
              </label>
            ))}
          </fieldset>
          <fieldset className="shop-filter-group shop-price-filter">
            <legend>PRICE / INR</legend>
            <div className="shop-range-values"><span>MIN · ₹{minimumPrice}</span><span>MAX · ₹{maximumPrice}</span></div>
            <div
              className="shop-range-control"
              style={{
                "--range-start": `${(minimumPrice / priceCeiling) * 100}%`,
                "--range-end": `${(maximumPrice / priceCeiling) * 100}%`,
              } as CSSProperties}
            >
              <span className="shop-range-track" aria-hidden="true"><i /></span>
              <input aria-label="Minimum price" type="range" min="0" max={priceCeiling} value={minimumPrice} onChange={(event) => updateSearchParam("min", Math.min(Number(event.currentTarget.value), maximumPrice))} />
              <input aria-label="Maximum price" type="range" min="0" max={priceCeiling} value={maximumPrice} onChange={(event) => updateSearchParam("max", Math.max(Number(event.currentTarget.value), minimumPrice))} />
            </div>
          </fieldset>
          <fieldset className="shop-filter-group">
            <legend>LABEL</legend>
            {LABELS.map((entry) => (
              <label key={entry.value}>
                <input type="checkbox" checked={selectedLabels.has(entry.value)} onChange={() => toggleSetParam("label", entry.value, selectedLabels)} />
                <span>{entry.label}</span>
                <b>{allItems.filter((item) => item.data.label === entry.value).length}</b>
              </label>
            ))}
          </fieldset>
          {hasFilters && <button className="shop-clear" type="button" onClick={() => setSearchParams({}, { replace: true })}>[ CLEAR ]</button>}
        </aside>

        <DropArea id="grid-zone" className="shop-results" label="Catalog results">
          <div className="shop-mobile-tools">
            <button type="button" aria-expanded={filterOpen} onClick={() => setFilterOpen(true)}>FILTERS +</button>
            <button type="button" onClick={() => setCartModalOpen(true)} disabled={cart.length === 0}>CART / {cartCount}</button>
            <Link to="/login">ACCESS DESK</Link>
          </div>
          <div className="shop-result-count">
            <span>{allItems.length} ITEMS{hasFilters ? ` · ${filteredItems.length} FILTERED` : ""}</span>
            <span>{property?.data.service_address.city ?? "LUCKNOW"} · INR</span>
          </div>
          {catalogQuery.isLoading ? (
            <div className="shop-grid shop-skeleton-grid" aria-label="Loading catalog">
              {Array.from({ length: 8 }, (_, index) => <div className="shop-skeleton" key={index} />)}
            </div>
          ) : filteredItems.length === 0 ? (
            <div className="shop-no-results"><strong>nothing matches.</strong><button type="button" onClick={() => setSearchParams({}, { replace: true })}>CLEAR FILTERS</button></div>
          ) : (
            <div className="shop-grid">
              {filteredItems.map((item) => (
                <ProductCard
                  key={item.id}
                  item={item}
                  quantity={cart.find((line) => line.item.id === item.id)?.quantity ?? 0}
                  justAdded={justAddedId === item.id}
                  onAdd={addToCart}
                  onOpen={openQuickView}
                  onContextMenu={openContextMenu}
                />
              ))}
            </div>
          )}
        </DropArea>

        <DropArea id="cart-zone" className={`shop-cart-column${cartOpen ? " is-open" : ""}`} label="Package cart">
          <div className="shop-cart-heading">
            <span>02 / YOUR CART</span>
            <span className="shop-status" data-status={packageVersion?.data.status ?? "draft"}>{(packageVersion?.data.status ?? "draft").toUpperCase()}</span>
            <button type="button" onClick={() => setCartOpen(false)}>CLOSE ×</button>
          </div>
          {offline && <div className="shop-offline">OFFLINE · CHANGES NOT SAVED</div>}
          {activeDrag && <div className="shop-drop-label">DROP TO ADD</div>}
          <section className="shop-cart-presets">
            <div className="shop-cart-presets-head">
              <span>PRESETS</span>
              {cart.length > 0 && (
                <button type="button" disabled={showPresetSave} onClick={() => { setShowPresetSave(true); setPresetSaveName(""); }}>
                  {showPresetSave ? "SAVING" : "+ SAVE"}
                </button>
              )}
            </div>
            {showPresetSave && (
              <div className="shop-preset-name-input">
                <input
                  value={presetSaveName}
                  placeholder="NAME"
                  onChange={(e) => setPresetSaveName(e.currentTarget.value)}
                  onKeyDown={(e) => { if (e.key === "Enter") handleSavePreset(); if (e.key === "Escape") setShowPresetSave(false); }}
                  autoFocus
                />
                <button type="button" onClick={handleSavePreset}>SAVE</button>
              </div>
            )}
            {Object.keys(presets).length > 0 ? (
              <div className="shop-preset-list">
                {Object.entries(presets).map(([name, entry]) => (
                  <div className="shop-preset-row" key={name}>
                    <span><strong>{name}</strong><br /><small>{entry.length} ITEM{entry.length !== 1 ? "S" : ""}</small></span>
                    {swapConfirmPreset === name ? (
                      <div style={{ display: "flex", gap: "6px", alignItems: "center", gridColumn: "2 / span 2" }}>
                        DISCARD CART?
                        <button type="button" onClick={() => handleLoadPreset(name)}>OK</button>
                        <button type="button" onClick={() => setSwapConfirmPreset(null)}>—</button>
                      </div>
                    ) : (
                      <>
                        <button type="button" onClick={() => requestLoadPreset(name)}>LOAD</button>
                        <button type="button" className="danger" onClick={() => handleDeletePreset(name)}>DEL</button>
                      </>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <p style={{ margin: "8px 0 0", fontFamily: "'JetBrains Mono Variable', monospace", fontSize: "9px", color: "var(--ink-60)", letterSpacing: "0.03em" }}>NONE SAVED</p>
            )}
          </section>
          <div className="shop-cart-rows">
            {cart.length === 0 ? (
              <div className="shop-empty-cart"><strong>empty.</strong><em>for now.</em></div>
            ) : cart.map((line) => (
              <CartRow
                key={line.item.id}
                line={line}
                active={packageVersion?.data.status === "active" && savedSignature === signature}
                exiting={exitingIds.has(line.item.id)}
                onRemove={() => removeFromCart(line.item.id)}
                onChange={(patch) => setCart((current) => current.map((entry) => entry.item.id === line.item.id ? { ...entry, ...patch } : entry))}
              />
            ))}
          </div>
          <div className="shop-cart-checkout">
            <PaymentBoundary boundaryId={`package-checkout-${propertyId}`}>
              <section className="shop-costs" ref={cartTotalSurface.ref} data-settling={settling} aria-live="polite">
                <div><span>SETUP</span><strong>{costs ? <Money value={costs.setup_cost_minor_units} currency={displayCurrency} /> : "—"}</strong></div>
                <div className="shop-monthly-total" data-marker={markerVisible}><span>MONTHLY</span><strong>{costs ? <Money value={costs.monthly_cost_minor_units} currency={displayCurrency} /> : "—"}</strong><svg viewBox="0 0 180 64" aria-hidden="true"><path d="M10 34C15 8 156 2 170 29C181 52 32 66 11 43C4 35 9 23 25 15" /></svg></div>
                <em>recalculated by our warehouse</em>
              </section>
              <details className="shop-rules">
                <summary>03 / RULES</summary>
                <fieldset>
                  <legend>SUBSTITUTION POLICY</legend>
                  {(["owner_approval", "automatic", "restricted"] as PackagePolicy[]).map((value) => (
                    <label key={value} data-selected={policy === value}><input type="radio" name="policy" value={value} checked={policy === value} onChange={() => setPolicy(value)} /><span>{value.replaceAll("_", " ")}</span></label>
                  ))}
                </fieldset>
                <label className="shop-toggle"><input type="checkbox" checked={approvePriceIncrease} onChange={(event) => setApprovePriceIncrease(event.currentTarget.checked)} /><span>APPROVE PRICE INCREASES</span></label>
                <label className="shop-toggle"><input type="checkbox" checked={approveNewSku} onChange={(event) => setApproveNewSku(event.currentTarget.checked)} /><span>APPROVE NEW SKUS</span></label>
                <label className="shop-budget"><span>MONTHLY BUDGET LIMIT / INR</span><input type="number" min="0" inputMode="numeric" value={monthlyBudget} placeholder="OPTIONAL" onChange={(event) => setMonthlyBudget(event.currentTarget.value)} /><small>DISPLAY LIMIT · NOT ENFORCED BY BACKEND</small></label>
              </details>
              {controlSession.state !== "granted" && (
                <PaymentBoundaryButton
                  className="shop-activate"
                  type="button"
                  disabled={!canActivate}
                  onClick={() => void activate()}
                >
                  {activating ? "ACTIVATING" : settling ? "SETTLING" : packageVersion?.data.status === "active" ? "ACTIVE" : "REVIEW & ACTIVATE"}
                </PaymentBoundaryButton>
              )}
            </PaymentBoundary>
            {controlSession.state === "granted" && (
              <PaymentBoundaryTriggerButton
                className="shop-activate"
                type="button"
                boundaryId={`package-checkout-${propertyId}`}
                agentId={`package-payment-handoff-${propertyId}`}
                agentLabel="Finish the package draft and request owner review"
              >
                REVIEW & ACTIVATE
              </PaymentBoundaryTriggerButton>
            )}
          </div>
        </DropArea>

        <MobileCartBar
          open={cartOpen}
          itemCount={cartCount}
          monthlyCost={costs ? formatMoney(costs.monthly_cost_minor_units, displayCurrency) : "—"}
          onOpen={() => setCartOpen(true)}
        />
      </main>

      <DragOverlay dropAnimation={reducedMotion ? null : { duration: 200, easing: "cubic-bezier(.34,1.56,.64,1)" }}>
        {activeDrag ? <div className="shop-drag-overlay"><ProductVisual item={activeDrag} /><div className="shop-product-copy"><strong>{activeDrag.data.name}</strong><span>{activeDrag.data.sku}</span><Money value={activeDrag.data.owner_price_minor_units} currency={activeDrag.data.owner_price_currency} /></div></div> : null}
      </DragOverlay>

      {contextMenu && (
        <div
          ref={menuRef}
          className="shop-context-menu"
          role="menu"
          style={{ left: Math.min(contextMenu.x, window.innerWidth - 190), top: Math.min(contextMenu.y, window.innerHeight - 110) }}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              const target = contextMenu.returnFocus;
              setContextMenu(null);
              window.setTimeout(() => target.focus());
            }
            if (event.key === "Tab") setContextMenu(null);
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              const buttons = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>("button") ?? []);
              const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
              const step = event.key === "ArrowDown" ? 1 : -1;
              buttons[(index + step + buttons.length) % buttons.length]?.focus();
            }
          }}
        >
          <span>ACTIONS / {contextMenu.item.data.sku}</span>
          <button role="menuitem" type="button" onClick={() => { addToCart(contextMenu.item); setContextMenu(null); contextMenu.returnFocus.focus(); }}>ADD TO CART</button>
           <button role="menuitem" type="button" onClick={() => { openQuickView(contextMenu.item); setContextMenu(null); }}>QUICK VIEW</button>
        </div>
      )}
       <QuickView item={quickView} onClose={() => setQuickView(null)} />
      <CartModal
        open={cartModalOpen}
        onClose={() => setCartModalOpen(false)}
        cart={cart}
        currency={displayCurrency}
        monthlyTotalMinorUnits={costs?.monthly_cost_minor_units ?? null}
        onChangeLine={(itemId, patch) => setCart((current) => current.map((entry) => entry.item.id === itemId ? { ...entry, ...patch } : entry))}
        onRemoveLine={removeFromCart}
        onClearAll={() => setCart([])}
      />
    </DndContext>
  );
}
