import type { AgentIntent, AgentSurfaceRegistry, IntentResult } from "./types";

function isInteractable(element: HTMLElement): string | null {
  if (!element.isConnected) return "element is not in the DOM";

  const style = getComputedStyle(element);
  if (style.display === "none") return "element has display:none";
  if (style.visibility === "hidden") return "element has visibility:hidden";

  const disabled =
    "disabled" in element &&
    (element as HTMLInputElement | HTMLButtonElement | HTMLSelectElement | HTMLTextAreaElement)
      .disabled;
  if (disabled) return "element is disabled";

  return null;
}

function intentToAction(intent: AgentIntent): string {
  switch (intent.type) {
    case "ui.focus":
      return "focus";
    case "ui.set_value":
      return "set";
    case "ui.click":
      return "click";
    case "ui.scroll_to":
      return "scroll_to";
    case "ui.open_panel":
      return "open_panel";
  }
}

const reducedMotionQuery = typeof window !== "undefined"
  ? window.matchMedia("(prefers-reduced-motion: reduce)")
  : null;

export function applyAgentIntent(
  registry: AgentSurfaceRegistry,
  intent: AgentIntent,
): IntentResult {
  const { id } = intent;

  const entry = registry.get(id);
  if (!entry) {
    return {
      ok: false,
      intent,
      reason: "unregistered_id",
      detail: `id "${id}" is not registered on the current route`,
    };
  }

  const actionKey = intentToAction(intent);
  if (!entry.actions.has(actionKey as never)) {
    return {
      ok: false,
      intent,
      reason: "action_not_allowed",
      detail: `action "${actionKey}" is not in data-agent-actions for "${id}"`,
    };
  }

  const element = entry.element;
  const blocked = isInteractable(element);
  if (blocked) {
    return {
      ok: false,
      intent,
      reason: "not_interactable",
      detail: `"${id}" is not interactable: ${blocked}`,
    };
  }

  switch (intent.type) {
    case "ui.focus": {
      element.focus();
      return { ok: true, intent };
    }

    case "ui.set_value": {
      const value = intent.value;

      if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
        const descriptor = Object.getOwnPropertyDescriptor(
          element instanceof HTMLInputElement
            ? window.HTMLInputElement.prototype
            : window.HTMLTextAreaElement.prototype,
          "value",
        );
        descriptor?.set?.call(element, value);
        element.dispatchEvent(new Event("input", { bubbles: true }));
        element.dispatchEvent(new Event("change", { bubbles: true }));
      } else if (element instanceof HTMLSelectElement) {
        element.value = value;
        element.dispatchEvent(new Event("change", { bubbles: true }));
      } else {
        element.setAttribute("value", value);
      }

      return { ok: true, intent };
    }

    case "ui.click": {
      element.click();
      return { ok: true, intent };
    }

    case "ui.scroll_to": {
      const smooth = reducedMotionQuery?.matches !== true;
      element.scrollIntoView({
        behavior: smooth ? "smooth" : "auto",
        block: "nearest",
      });
      return { ok: true, intent };
    }

    case "ui.open_panel": {
      if (!entry.onOpenPanel) {
        return {
          ok: false,
          intent,
          reason: "action_not_allowed",
          detail: `"${id}" does not have an onOpenPanel handler (not a disclosure element)`,
        };
      }
      entry.onOpenPanel();
      return { ok: true, intent };
    }
  }
}
