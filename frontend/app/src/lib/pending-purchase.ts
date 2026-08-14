export type PendingPurchaseLine = {
  catalogItemId?: string;
  name: string;
  sku: string;
  quantity: number;
  monthlyUse?: number;
  unitPriceMinorUnits: number;
  currency: string;
  draftPackageId?: string;
};

const STORAGE_KEY = (propertyId: string) => `cc_pending_purchase_${propertyId}`;
const snapshots = new Map<string, PendingPurchaseLine[]>();
const listeners = new Set<() => void>();

function read(propertyId: string): PendingPurchaseLine[] {
  if (snapshots.has(propertyId)) return snapshots.get(propertyId) ?? [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY(propertyId));
    const value = raw ? JSON.parse(raw) : [];
    const lines = Array.isArray(value) ? value as PendingPurchaseLine[] : [];
    snapshots.set(propertyId, lines);
    return lines;
  } catch {
    snapshots.set(propertyId, []);
    return [];
  }
}

function notify() {
  for (const listener of listeners) listener();
}

export function savePendingPurchase(propertyId: string, lines: PendingPurchaseLine[]): void {
  const next = lines.map((line) => ({ ...line }));
  snapshots.set(propertyId, next);
  try {
    window.localStorage.setItem(STORAGE_KEY(propertyId), JSON.stringify(next));
  } catch {
    // The in-memory snapshot still keeps same-tab consumers in sync.
  }
  notify();
}

export function clearPendingPurchase(propertyId: string): void {
  snapshots.set(propertyId, []);
  try {
    window.localStorage.removeItem(STORAGE_KEY(propertyId));
  } catch {
    // Clearing the in-memory snapshot is sufficient if storage is unavailable.
  }
  notify();
}

export function getPendingPurchase(propertyId: string): PendingPurchaseLine[] {
  return read(propertyId);
}

export function subscribePendingPurchase(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === null) {
      snapshots.clear();
      notify();
      return;
    }
    if (!event.key.startsWith("cc_pending_purchase_")) return;
    const propertyId = event.key.slice("cc_pending_purchase_".length);
    snapshots.delete(propertyId);
    notify();
  });
}
