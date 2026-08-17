import type { AgentIntent, AgentSurfaceRegistry, IntentResult } from "../agent-surface/types";
import { applyAgentIntent } from "../agent-surface/driver";
import { canAct as checkCanAct, setAgentInFlight, type ControlSession } from "./control-session";
import type { RevokeReason } from "./ControlSession";
import "./ring.css";

export type GatedIntentResult =
  | IntentResult
  | {
      ok: false;
      intent: AgentIntent;
      reason: "not_granted" | "expired" | "action_capped" | "too_fast" | "target_not_found";
      detail: string;
    };

export type GatedDriverCtx = {
  canAct: true | "not_granted" | "expired" | "action_capped" | "too_fast";
  recordAction: () => void;
  revoke: (reason: RevokeReason) => void;
  session: ControlSession;
};

const reducedMotionQuery = typeof window !== "undefined"
  ? window.matchMedia("(prefers-reduced-motion: reduce)")
  : null;
const testMotion = import.meta.env.MODE === "test";

function isFullyVisible(element: HTMLElement): boolean {
  const rect = element.getBoundingClientRect();
  if (rect.width === 0 && rect.height === 0) return false;
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth;
  return rect.top >= 0 && rect.left >= 0 && rect.bottom <= viewportHeight && rect.right <= viewportWidth;
}

// Every gated action scrolls its real target into view first, unconditionally
// -- not only when the model separately asks for it. Ring and drag-ghost
// positions come from getBoundingClientRect() at the moment they're drawn;
// an item scrolled out of the catalog's own viewport still has a real rect
// (it's off-screen, not display:none), so the old code drew the whole
// cursor/ghost sequence starting from wherever that off-screen rect
// happened to be -- confirmed live, the drag visibly started from the
// wrong place whenever the target card wasn't already on screen. This is
// what makes "scroll down, find the item, then pick it up" the actual
// sequence a person watches, instead of "click a place I can't see."
function nextFrame(): Promise<void> {
  return new Promise((resolve) => window.requestAnimationFrame(() => resolve()));
}

async function scrollIntoViewAndSettle(element: HTMLElement): Promise<void> {
  if (testMotion) return;
  // A rect read on the same frame a page just mounted/re-rendered (right
  // after a navigation, right after a loading state resolves) can be
  // transiently wrong -- layout not yet settled, still reflecting a
  // previous frame. Reported live: the ring/cursor appeared once, briefly,
  // somewhere above the real page content instead of on the actual target.
  // One rendered frame of margin before trusting any rect costs nothing
  // visible and removes that race.
  await nextFrame();
  if (isFullyVisible(element)) return;
  const reduced = reducedMotionQuery?.matches === true;
  element.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "center", inline: "nearest" });
  const settleMs = reduced ? 60 : 500;
  await new Promise<void>((resolve) => window.setTimeout(resolve, settleMs));
}

// A human moving a mouse to a field takes a beat -- the original 400ms felt
// like a value teleporting into place with a flash around it. 550ms is still
// brisk (nobody wants to sit through a slow demo) but reads as travel, not a
// snap.
const RING_TRAVEL_MS = 550;

function animateRingTo(element: HTMLElement): Promise<void> {
  return new Promise((resolve) => {
    const rect = element.getBoundingClientRect();
    const reduced = reducedMotionQuery?.matches === true;

    const ring = document.createElement("div");
    ring.className = reduced ? "control-ring-reduced" : "control-ring";
    ring.style.left = `${rect.left}px`;
    ring.style.top = `${rect.top}px`;
    ring.style.width = `${rect.width}px`;
    ring.style.height = `${rect.height}px`;
    document.body.appendChild(ring);

    const duration = testMotion ? 1 : reduced ? 100 : RING_TRAVEL_MS;

    setTimeout(() => {
      ring.remove();
      resolve();
    }, duration);
  });
}

// A short, distinct flash right at the moment of the click itself -- the
// ring above means "I've arrived here," this means "and now I'm pressing
// it," so a click reads as its own decisive beat instead of blurring into
// the same arrival ring a hover or a field-fill also shows.
function animateClickPulse(element: HTMLElement): Promise<void> {
  return new Promise((resolve) => {
    const reduced = reducedMotionQuery?.matches === true;
    if (reduced || testMotion) {
      resolve();
      return;
    }
    const rect = element.getBoundingClientRect();
    const size = Math.min(rect.width, rect.height, 48);
    const pulse = document.createElement("div");
    pulse.className = "control-click-pulse";
    pulse.style.left = `${rect.left + rect.width / 2 - size / 2}px`;
    pulse.style.top = `${rect.top + rect.height / 2 - size / 2}px`;
    pulse.style.width = `${size}px`;
    pulse.style.height = `${size}px`;
    document.body.appendChild(pulse);

    const duration = 220;
    setTimeout(() => {
      pulse.remove();
      resolve();
    }, duration);
  });
}

// Types a value one character at a time instead of dropping the whole
// string in at once, so a form fill reads as someone actually typing
// rather than a paste. Each keystroke is a real native-setter + `input`
// event (the same mechanism the instant version used), so any controlled
// React input stays in sync at every step, not just at the end.
function typeValueIntoElement(element: HTMLElement, value: string): Promise<void> {
  const reduced = reducedMotionQuery?.matches === true;
  if (
    testMotion ||
    reduced ||
    !(element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement)
  ) {
    return Promise.resolve();
  }

  const descriptor = Object.getOwnPropertyDescriptor(
    element instanceof HTMLInputElement ? window.HTMLInputElement.prototype : window.HTMLTextAreaElement.prototype,
    "value",
  );
  if (!descriptor?.set) return Promise.resolve();

  element.focus();

  return new Promise((resolve) => {
    let index = 0;
    const step = () => {
      index += 1;
      descriptor.set!.call(element, value.slice(0, index));
      element.dispatchEvent(new Event("input", { bubbles: true }));
      if (index >= value.length) {
        resolve();
        return;
      }
      // 22-48ms per character with jitter -- fast enough not to stall a
      // longer sentence, uneven enough not to read as a metronome.
      window.setTimeout(step, 22 + Math.random() * 26);
    };
    step();
  });
}

export function createGatedDriver(getCtx: () => GatedDriverCtx) {
  return async function gatedApplyAgentIntent(
    registry: AgentSurfaceRegistry,
    intent: AgentIntent,
  ): Promise<GatedIntentResult> {
    let ctx = getCtx();
    let gate = checkCanAct(ctx.session, Date.now());

    // Multiple tool calls from one model response are deliberately handled in
    // a serial promise chain. After an action succeeds, recordAction stamps its
    // completion time; checking the next action immediately used to reject it
    // as `too_fast`, so a request such as "add coffee and a welcome kit" could
    // only ever perform the first click. The 250 ms budget is a minimum visual
    // spacing, not a reason to discard an already-authorized queued action.
    //
    // A loop, not one attempt: a single wait-then-recheck assumes nothing
    // else touches lastActionTime in that window, which isn't guaranteed --
    // observed live, a `too_fast` still surfaced as a visible "blocked" line
    // on rare occasions. Retrying the shortfall a few times costs nothing
    // when the gate is already open (the loop exits immediately) and gives
    // real margin when it isn't, instead of surfacing a transient timing
    // gap as a hard failure.
    for (let attempt = 0; gate === "too_fast" && ctx.session.state === "granted" && attempt < 5; attempt++) {
      const elapsed = Date.now() - ctx.session.lastActionTime;
      const waitMs = Math.max(20, ctx.session.minSpacing - elapsed);
      await new Promise<void>((resolve) => window.setTimeout(resolve, waitMs));
      ctx = getCtx();
      gate = checkCanAct(ctx.session, Date.now());
    }

    if (gate !== true) {
      return {
        ok: false,
        intent,
        reason: gate,
        detail: `control session check failed: ${gate}`,
      };
    }

    const entry = registry.get(intent.id);
    if (!entry) {
      // Ids containing "-link-" are this app's convention for a real
      // cross-page navigation control (see dashboard.tsx's
      // dashboard-package-link-*). Observed live: after successfully
      // clicking one of these (a genuine page navigation, which unmounts
      // it), the model would sometimes try the exact same id again on the
      // new page and read the resulting generic "not registered" as some
      // kind of failure -- worth a plainer, more specific explanation here
      // than for an ordinary missing surface, since the likely truth is
      // the opposite of a failure: the navigation already worked and this
      // id simply doesn't exist on the page it left you on.
      const detail = intent.id.includes("-link-")
        ? `id "${intent.id}" is not registered on the current page. If this is a navigation link you already clicked once, that click already worked -- you are on the new page now, and this id only ever existed on the one you left. Do not click it again; look at "Available UI surfaces" for what this page actually offers.`
        : `id "${intent.id}" is not registered`;
      return {
        ok: false,
        intent,
        reason: "target_not_found",
        detail,
      };
    }

    // setAgentInFlight covers every real DOM event the animations below
    // dispatch, not just the final click/set -- typeValueIntoElement fires
    // a genuine `input` event per simulated keystroke, and without this
    // flag up for the whole sequence, ControlSession's generic "the human
    // took the wheel back" listener (see its isAgentInFlight() check)
    // would read Superhost's own typing as a real person typing and
    // revoke control mid-action.
    setAgentInFlight(true);
    try {
      await scrollIntoViewAndSettle(entry.element);
      await animateRingTo(entry.element);

      // The flying product-card ghost (an earlier animateDragToTarget,
      // since removed) read as awkward rather than convincing -- a plain
      // click after the cursor arrives, same as every other action, is
      // the clearer version. data-agent-drag-target stays on the cart-add
      // buttons themselves (harmless if unused) rather than pulling that
      // markup out of package-shop.tsx for a purely visual call site.

      if (intent.type === "ui.set_value") {
        await typeValueIntoElement(entry.element, intent.value);
      }
      if (intent.type === "ui.click") {
        await animateClickPulse(entry.element);
      }

      ctx = getCtx();
      const postAnimationGate = checkCanAct(ctx.session, Date.now());
      if (postAnimationGate !== true && postAnimationGate !== "too_fast") {
        return {
          ok: false,
          intent,
          reason: postAnimationGate,
          detail: `control session changed during ring animation: ${postAnimationGate}`,
        };
      }

      const result = applyAgentIntent(registry, intent);
      if (result.ok) {
        ctx.recordAction();
      }
      return result;
    } finally {
      setAgentInFlight(false);
    }
  };
}
