// A tiny pub-sub so a plain async function (api()) can trigger UI that only a
// mounted React component can render, without either one importing the other.
// api() calls notifySessionExpired() on a real 401; SessionExpiredModal (mounted
// once at the app root) is the only subscriber, and renders the actual modal.

type Listener = (reason: string) => void;

const listeners = new Set<Listener>();

export function onSessionExpired(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function notifySessionExpired(reason: string): void {
  for (const listener of listeners) listener(reason);
}
