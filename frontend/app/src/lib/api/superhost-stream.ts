import { useEffect, useRef, useState } from "react";
import { getToken } from "../auth/session";

export type SuperhostStreamEvent = {
  event_id: string;
  run_id: string;
  event_name: string;
  event_data?: unknown;
  occurred_at: string;
};

export type StreamConnectionState = "connecting" | "open" | "error" | "done";

export class SuperhostStreamError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "SuperhostStreamError";
    this.status = status;
  }
}

function parseEventBlock(block: string): SuperhostStreamEvent | "done" | null {
  const data = block
    .split(/\r?\n/)
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart())
    .join("\n");

  if (!data) return null;
  if (data === "[DONE]") return "done";
  return JSON.parse(data) as SuperhostStreamEvent;
}

/** Reads the bearer-authenticated stream without relying on EventSource. */
export async function* streamSuperhostEvents(
  threadId: string,
  signal?: AbortSignal,
): AsyncGenerator<SuperhostStreamEvent> {
  const token = getToken();
  const response = await fetch(`/api/v1/superhost/threads/${encodeURIComponent(threadId)}/stream`, {
    headers: {
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal,
  });

  if (!response.ok) throw new SuperhostStreamError(response.status, response.statusText || "Stream request failed");
  if (!response.body) throw new Error("Stream response did not include a body");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const result = await reader.read();
      if (result.done) {
        buffer += decoder.decode();
        const finalBlock = buffer.trim();
        if (finalBlock) {
          const parsed = parseEventBlock(finalBlock);
          if (parsed && parsed !== "done") yield parsed;
        }
        return;
      }

      buffer += decoder.decode(result.value, { stream: true });
      let boundary = buffer.indexOf("\n\n");
      while (boundary !== -1) {
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        const parsed = parseEventBlock(block);
        if (parsed === "done") return;
        if (parsed) yield parsed;
        boundary = buffer.indexOf("\n\n");
      }
    }
  } finally {
    await reader.cancel().catch(() => undefined);
  }
}

const CACHE_PREFIX = "superhost-cache:";
const CACHE_MAX_EVENTS = 60;

function readCache(cacheKey: string | null): SuperhostStreamEvent[] {
  if (!cacheKey) return [];
  try {
    const raw = window.localStorage.getItem(CACHE_PREFIX + cacheKey);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as SuperhostStreamEvent[]) : [];
  } catch {
    return [];
  }
}

function writeCache(cacheKey: string | null, events: SuperhostStreamEvent[]) {
  if (!cacheKey) return;
  try {
    window.localStorage.setItem(CACHE_PREFIX + cacheKey, JSON.stringify(events.slice(-CACHE_MAX_EVENTS)));
  } catch {
    // Storage can be full or disabled (private browsing) -- this is a
    // "skip the loading flash" nicety, not load-bearing, so a failure
    // here should never surface as a real error.
  }
}

// cacheKey identifies a conversation across page loads/reconnects --
// stable and known *before* threadId resolves (SuperhostMount passes its
// own idempotencyKey), unlike threadId itself, which only exists after
// the create-thread round trip completes. Caching under threadId alone
// couldn't show anything until that round trip finished; caching under
// this key means the last known state renders immediately on mount,
// replaced by the real thing once the real connection catches up, so
// reopening Superhost doesn't read as "loading" every time.
export function useSuperhostStream(threadId: string | null, enabled = true, generation = 0, cacheKey: string | null = null) {
  const [events, setEvents] = useState<SuperhostStreamEvent[]>(() => readCache(cacheKey));
  const [state, setState] = useState<StreamConnectionState>("done");
  const [error, setError] = useState<Error | null>(null);
  const cacheKeyRef = useRef(cacheKey);
  cacheKeyRef.current = cacheKey;
  const connectedThreadIdRef = useRef<string | null>(null);

  // cacheKey can change (a different property/routeKey mounts the same
  // component) after the initial lazy read above already ran -- rehydrate
  // from the new key's cache rather than keep showing the old one's.
  useEffect(() => {
    setEvents(readCache(cacheKey));
  }, [cacheKey]);

  useEffect(() => {
    writeCache(cacheKeyRef.current, events);
  }, [events]);

  useEffect(() => {
    if (!threadId || !enabled) return;
    const controller = new AbortController();
    let mounted = true;
    const isReconnect = connectedThreadIdRef.current === threadId;
    connectedThreadIdRef.current = threadId;
    // On a genuinely new (non-reconnect) thread, start from whatever the
    // cache says for the *current* cacheKey -- [] when no cacheKey was
    // given (a caller not opted into caching, same as the old always-
    // clear behavior), or that key's real cached history when one was.
    // Real events merge in below by event_id regardless, so a cache
    // entry that turns out stale/wrong is naturally superseded, never
    // duplicated -- this only decides the *starting* point, not the
    // ongoing merge.
    if (!isReconnect) setEvents(readCache(cacheKeyRef.current));
    setError(null);
    setState("connecting");

    void (async () => {
      try {
        for await (const event of streamSuperhostEvents(threadId, controller.signal)) {
          if (!mounted) return;
          setState("open");
          setEvents((current) => {
            const index = current.findIndex((existing) => existing.event_id === event.event_id);
            if (index === -1) return [...current, event];
            // A real, persisted event never arrives twice with different
            // content, so replacing in place is a no-op for it -- but
            // AgentRunToken.v1 (see behavior.ts) is a synthetic, live-
            // updating frame that reuses one event_id across a whole
            // streaming turn precisely so each new delta *replaces* the
            // last one here instead of appending a fresh line per tick.
            if (current[index] === event) return current;
            const next = current.slice();
            next[index] = event;
            return next;
          });
        }
        if (mounted) setState("done");
      } catch (caught) {
        if (!mounted || controller.signal.aborted) return;
        setError(caught instanceof Error ? caught : new Error(String(caught)));
        setState("error");
      }
    })();

    return () => {
      mounted = false;
      controller.abort();
    };
  }, [enabled, generation, threadId]);

  return { events, state, error };
}
