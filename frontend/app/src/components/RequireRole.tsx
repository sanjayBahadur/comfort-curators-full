import type { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { getRole, getToken, type Role } from "../lib/auth/session";
import { homeFor } from "../lib/auth/roles";

type RequireRoleProps = {
  allow: readonly Role[];
  children: ReactNode;
};

export function RequireRole({ allow, children }: RequireRoleProps) {
  const token = getToken();
  const role = getRole();

  if (!token || !role) return <Navigate to="/login" replace />;
  if (!allow.includes(role)) return <Navigate to={homeFor(role)} replace />;

  return children;
}

export default RequireRole;
