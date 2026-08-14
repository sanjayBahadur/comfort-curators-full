// Browser-only demo token store. ARCHITECTURE.md §5.
// The backend mints a session for any role with no credential check, so this is
// deliberately simple. Post-demo this is replaced by real OTP auth.
import { toast } from "sonner";

const TENANT = import.meta.env.VITE_DEMO_TENANT_ID as string;
const TOKEN_KEY = "cc_session";
const ROLE_KEY = "cc_role";

export type Role = "owner" | "staff" | "guest";

type SessionResponse = {
  roles: string[];
  session_token: string;
  user_id: string;
};

let token: string | null = sessionStorage.getItem(TOKEN_KEY);
let role: Role | null = sessionStorage.getItem(ROLE_KEY) as Role | null;

export const getToken = () => token;
export const getRole = () => role;

export async function signIn(role: Role, contact: string): Promise<void> {
  if (!TENANT) {
    const message = "Demo tenant is not configured";
    toast.error(message);
    throw new Error(message);
  }

  const res = await fetch("/api/auth/session/create", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tenant_id: TENANT, contact, roles: [role] }),
  });
  const body = (await res.json().catch(() => ({}))) as Partial<SessionResponse> & {
    message?: string;
  };

  if (!res.ok) {
    const message = body.message ?? res.statusText;
    toast.error(message);
    throw new Error(message);
  }
  if (!body.session_token) {
    const message = "Session response did not include a token";
    toast.error(message);
    throw new Error(message);
  }

  token = body.session_token;
  sessionStorage.setItem(TOKEN_KEY, body.session_token);
  sessionStorage.setItem(ROLE_KEY, role);
  setStoredRole(role);
}

export function signOut(): void {
  token = null;
  setStoredRole(null);
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(ROLE_KEY);
}

function setStoredRole(nextRole: Role | null): void {
  role = nextRole;
}
