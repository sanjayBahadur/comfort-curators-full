import { useNavigate, useLocation } from "react-router-dom";
import "./GlobalBackButton.css";

// A persistent, always-in-the-same-place way out of any screen. Several
// routes in this app are effectively dead ends without it -- reachable, but
// with no way back except re-typing a URL. This is the general-case fix;
// specific missing links (ops nav, dashboard nav, the guest portal) were
// also fixed directly where they were found, but this covers whatever is
// still missed.
export default function GlobalBackButton() {
  const navigate = useNavigate();
  const location = useLocation();

  // /login is the one true "there is nowhere further back" screen -- it's
  // where every dead end in this app is supposed to lead anyway.
  if (location.pathname === "/login") return null;
  if (typeof window !== "undefined" && window.history.length <= 1) return null;

  return (
    <button
      type="button"
      className="global-back-button"
      onClick={() => navigate(-1)}
      aria-label="Go back to the previous screen"
    >
      ← BACK
    </button>
  );
}
