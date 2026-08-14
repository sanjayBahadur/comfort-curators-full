import { useLocation } from "react-router-dom";
import "./global-superhost-button.css";

export default function GlobalSuperhostButton({
  onOpen,
  open,
  activeInBackground,
}: {
  onOpen: () => void;
  open: boolean;
  activeInBackground: boolean;
}) {
  const location = useLocation();
  if (location.pathname === "/login") return null;

  return (
    <button
      type="button"
      className="global-superhost-button"
      data-open={open}
      data-active={activeInBackground}
      data-control-exempt="true"
      onClick={onOpen}
      aria-label={open ? "Minimize Superhost" : activeInBackground ? "Open Superhost — active in background" : "Open Superhost"}
      aria-expanded={open}
    >
      <span className="global-superhost-button-status" aria-hidden="true" />
      SUPERHOST <span aria-hidden="true">{open ? "—" : "↗"}</span>
      {activeInBackground && <span className="sr-only">Active in background</span>}
    </button>
  );
}
