export type AgentAction = "focus" | "set" | "click" | "scroll_to" | "open_panel";

export type AgentIntent =
  | { type: "ui.focus"; id: string }
  | { type: "ui.set_value"; id: string; value: string }
  | { type: "ui.click"; id: string }
  | { type: "ui.scroll_to"; id: string }
  | { type: "ui.open_panel"; id: string };

export type AgentSurfaceEntry = {
  id: string;
  element: HTMLElement;
  actions: Set<AgentAction>;
  label: string;
  onOpenPanel?: () => void;
};

export type IntentOk = { ok: true; intent: AgentIntent };

export type IntentFail = {
  ok: false;
  intent: AgentIntent;
  reason: "unregistered_id" | "action_not_allowed" | "not_interactable";
  detail: string;
};

export type IntentResult = IntentOk | IntentFail;

export type AgentSurfaceRegistry = Map<string, AgentSurfaceEntry>;
