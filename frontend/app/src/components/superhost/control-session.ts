export type ControlSessionState = "idle" | "granted";

export type ControlSession = {
  state: ControlSessionState;
  startedAt: number;
  ttl: number;
  actionCount: number;
  maxActions: number;
  lastActionTime: number;
  minSpacing: number;
};

// A live model turn can include navigation, catalog loading, several narrated
// drags, and a server-priced draft save. Ninety seconds was short enough for
// the grant to expire while those authorized actions were still queued. Keep
// the session bounded, but give the demo a realistic five-minute window.
export const DEFAULT_TTL = 5 * 60_000;
export const DEFAULT_MAX_ACTIONS = 25;
export const DEFAULT_MIN_SPACING = 250;

let _inFlight = false;

export function setAgentInFlight(v: boolean) {
  _inFlight = v;
}

export function isAgentInFlight() {
  return _inFlight;
}

export function createIdleSession(): ControlSession {
  return {
    state: "idle",
    startedAt: 0,
    ttl: DEFAULT_TTL,
    actionCount: 0,
    maxActions: DEFAULT_MAX_ACTIONS,
    lastActionTime: 0,
    minSpacing: DEFAULT_MIN_SPACING,
  };
}

export function grantSession(now: number): ControlSession {
  return {
    state: "granted",
    startedAt: now,
    ttl: DEFAULT_TTL,
    actionCount: 0,
    maxActions: DEFAULT_MAX_ACTIONS,
    lastActionTime: 0,
    minSpacing: DEFAULT_MIN_SPACING,
  };
}

export function isExpired(session: ControlSession, now: number): boolean {
  if (session.state !== "granted") return false;
  return now - session.startedAt >= session.ttl;
}

export function isActionCapped(session: ControlSession): boolean {
  if (session.state !== "granted") return false;
  return session.actionCount >= session.maxActions;
}

export function spacingElapsed(session: ControlSession, now: number): boolean {
  if (session.state !== "granted") return false;
  if (session.lastActionTime === 0) return true;
  return now - session.lastActionTime >= session.minSpacing;
}

export function recordAction(session: ControlSession, now: number): ControlSession {
  if (session.state !== "granted") return session;
  return {
    ...session,
    // Sliding lease: every successful authorized action proves the run is
    // still actively progressing. Five idle minutes still expires the grant.
    startedAt: now,
    actionCount: session.actionCount + 1,
    lastActionTime: now,
  };
}

export function canAct(session: ControlSession, now: number): true | "not_granted" | "expired" | "action_capped" | "too_fast" {
  if (session.state !== "granted") return "not_granted";
  if (isExpired(session, now)) return "expired";
  if (isActionCapped(session)) return "action_capped";
  if (!spacingElapsed(session, now)) return "too_fast";
  return true;
}

export function remainingTime(session: ControlSession, now: number): number {
  if (session.state !== "granted") return 0;
  const elapsed = now - session.startedAt;
  return Math.max(0, session.ttl - elapsed);
}
