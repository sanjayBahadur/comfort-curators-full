import { api } from "./client";

export type AgentRun = {
  run_id: string;
  run_kind: "superhost" | "hermes" | string;
  tenant_id: string;
  property_id?: string;
  actor_id?: string;
  trigger_type?: string;
  trigger_id?: string;
  correlation_id?: string;
  state: string;
  state_version: number;
  attempt: number;
  max_attempts: number;
  provider: string;
  model: string;
  prompt_template_version?: string;
  input_schema_version?: string;
  output_schema_version?: string;
  input_data?: unknown;
  output_data?: unknown;
  error_message?: string;
  usage_minor_units: number;
  usage_currency: string;
  usage_input_tokens: number;
  usage_output_tokens: number;
  usage_total_tokens: number;
  usage_known: boolean;
  created_at: string;
  updated_at: string;
};

export type AgentRunEvent = {
  event_id: string;
  run_id: string;
  event_name: string;
  event_data?: unknown;
  occurred_at: string;
};

export type AgentRunEvents = { run_id: string; events: AgentRunEvent[] };

export const getAgentRun = (runId: string) => api<AgentRun>(`/v1/agent-runs/${encodeURIComponent(runId)}`);
export const getAgentRunEvents = (runId: string) => api<AgentRunEvents>(`/v1/agent-runs/${encodeURIComponent(runId)}/events`);

export type UISurfaceInput = { id: string; label: string; actions: string[] };

export type SendMessageResponse = { request_id: string; status: string; resource_id: string };

export function sendSuperhostMessage(
  threadId: string,
  content: string,
  uiSurfaces: UISurfaceInput[],
): Promise<SendMessageResponse> {
  return api<SendMessageResponse>(
    `/v1/superhost/threads/${encodeURIComponent(threadId)}/messages`,
    {
      method: "POST",
      body: JSON.stringify({
        idempotency_key: `superhost-msg:${crypto.randomUUID()}`,
        content,
        ui_surfaces: uiSurfaces,
      }),
    },
  );
}
