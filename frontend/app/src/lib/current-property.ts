// A tiny external store (useSyncExternalStore-compatible) so the global
// Superhost drawer -- mounted once at the app root, with no route params of
// its own -- can know which property the page you're actually looking at is
// scoped to. Pages that track a clear "current property" call
// setCurrentProperty() in an effect; the drawer reads it via
// useSyncExternalStore. Pages that never call it leave the drawer showing
// SuperhostMount's existing, honest "not connected" state -- the same
// pattern already used for the inline mounts, not a new invention.

export type CurrentProperty = { id: string; label: string } | null;

let current: CurrentProperty = null;
const listeners = new Set<() => void>();
let propertyRevision = 0;

export function setCurrentProperty(next: CurrentProperty): void {
  propertyRevision += 1;
  current = next;
  for (const listener of listeners) listener();
}

export function clearCurrentProperty(): void {
  // Route effect cleanup runs just before the next route's setup. Clearing
  // synchronously made the global Superhost briefly jump from the selected
  // property thread to the portfolio thread during dashboard -> package
  // navigation, aborting/restarting its stream while UI actions were queued.
  // A same-commit property setup supersedes this microtask; a genuine exit
  // still clears normally at the end of the turn.
  const clearRevision = ++propertyRevision;
  queueMicrotask(() => {
    if (propertyRevision !== clearRevision) return;
    current = null;
    for (const listener of listeners) listener();
  });
}

export function subscribeCurrentProperty(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getCurrentProperty(): CurrentProperty {
  return current;
}
