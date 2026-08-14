// ARCHITECTURE.md §4. Shapes verified live — see INTEGRATION.md.
import { getToken } from "../auth/session";
import { notifySessionExpired } from "../auth/session-expiry";
import { toast } from "sonner";

export type Envelope<T> = { id: string; version: number; data: T };

// NB: explicit fields, not TS parameter properties — the Vite react-ts template
// enables `erasableSyntaxOnly`, which rejects `constructor(readonly x: T)`.
export class ApiError extends Error {
  status: number;
  code: string;
  requestId: string;

  constructor(status: number, code: string, message: string, requestId: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers = new Headers(init.headers);

  headers.set("Accept", "application/json");
  if (init.body !== undefined && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(`/api${path}`, {
    ...init,
    headers,
  });
  const body = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  if (!res.ok) {
    const error = new ApiError(
      res.status,
      typeof body.code === "string" ? body.code : "UNKNOWN",
      typeof body.message === "string" ? body.message : res.statusText,
      typeof body.request_id === "string"
        ? body.request_id
        : (res.headers.get("X-Request-Id") ?? ""),
    );

    if (error.requestId) {
      console.error(`[api:${error.requestId}] ${error.code}: ${error.message}`);
    }
    // A real session expiry deserves an explanation the user can't miss, not
    // a toast that can go unnoticed and leave every next click failing the
    // same way silently.
    if (error.status === 401) {
      notifySessionExpired(error.message);
    } else {
      toast.error(error.message);
    }
    throw error;
  }
  return body as T;
}

/** Writes return {id,version,data}; most reads return the payload directly. */
export const unwrap = <T,>(e: Envelope<T>): T => e.data;
