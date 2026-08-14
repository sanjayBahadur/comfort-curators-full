import { Navigate, useSearchParams } from "react-router-dom";
import Intro from "./intro";
import { homeFor } from "../lib/auth/roles";
import { getRole, getToken } from "../lib/auth/session";

export default function EntryRoute() {
  const [searchParams] = useSearchParams();
  const role = getRole();

  if (searchParams.get("replay") === "intro") return <Intro />;
  if (!getToken()) return <Intro />;
  if (!role) return <Navigate to="/login" replace />;

  return <Navigate to={homeFor(role)} replace />;
}
