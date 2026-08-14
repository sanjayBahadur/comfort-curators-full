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

    const duration = testMotion ? 1 : reduced ? 100 : 400;

    setTimeout(() => {
      ring.remove();
      resolve();
    }, duration);
  });
}

function animateDragToTarget(source: HTMLElement, targetId: string): Promise<void> {
  const target = Array.from(document.querySelectorAll<HTMLElement>("[data-agent-drop-zone]"))
    .find((candidate) => candidate.dataset.agentDropZone === targetId);
  const card = source.closest<HTMLElement>(".shop-product-card") ?? source;
  if (!target) return Promise.resolve();

  const sourceRect = card.getBoundingClientRect();
  const targetRect = target.getBoundingClientRect();
  const ghost = card.cloneNode(true) as HTMLElement;
  ghost.className = "agent-drag-ghost";
  ghost.removeAttribute("data-agent");
  ghost.querySelectorAll("[data-agent]").forEach((node) => node.removeAttribute("data-agent"));
  ghost.style.left = `${sourceRect.left}px`;
  ghost.style.top = `${sourceRect.top}px`;
  ghost.style.width = `${sourceRect.width}px`;
  ghost.style.height = `${sourceRect.height}px`;
  document.body.appendChild(ghost);

  return new Promise((resolve) => {
    requestAnimationFrame(() => {
      const scale = Math.min(0.42, Math.max(0.22, 140 / Math.max(sourceRect.width, 1)));
      const targetX = targetRect.left + targetRect.width / 2 - sourceRect.left - sourceRect.width / 2;
      const targetY = targetRect.top + Math.min(120, targetRect.height / 2) - sourceRect.top - sourceRect.height / 2;
      ghost.style.transform = `translate(${targetX}px, ${targetY}px) scale(${scale}) rotate(-2deg)`;
      ghost.style.opacity = "0.35";

      const destinationRing = document.createElement("div");
      destinationRing.className = "control-ring";
      destinationRing.style.left = `${targetRect.left}px`;
      destinationRing.style.top = `${targetRect.top}px`;
      destinationRing.style.width = `${targetRect.width}px`;
      destinationRing.style.height = `${Math.min(targetRect.height, 160)}px`;
      document.body.appendChild(destinationRing);

      window.setTimeout(() => {
        ghost.remove();
        destinationRing.remove();
        resolve();
      }, testMotion ? 1 : 650);
    });
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
    if (gate === "too_fast" && ctx.session.state === "granted") {
      const elapsed = Date.now() - ctx.session.lastActionTime;
      const waitMs = Math.max(0, ctx.session.minSpacing - elapsed);
      if (waitMs > 0) {
        await new Promise<void>((resolve) => window.setTimeout(resolve, waitMs));
      }
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
      return {
        ok: false,
        intent,
        reason: "target_not_found",
        detail: `id "${intent.id}" is not registered`,
      };
    }

    await animateRingTo(entry.element);

    const dragTarget = entry.element.dataset.agentDragTarget;
    if (dragTarget) {
      await animateDragToTarget(entry.element, dragTarget);
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

    setAgentInFlight(true);
    try {
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
