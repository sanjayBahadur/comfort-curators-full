import { useLocation } from "react-router-dom";
import "./global-superhost-button.css";

export default function GlobalSuperhostButton({
  onOpen,
  open,
  activeInBackground,
  needsInput,
}: {
  onOpen: () => void;
  open: boolean;
  activeInBackground: boolean;
  needsInput: boolean;
}) {
  const location = useLocation();
  if (location.pathname === "/login") return null;

  const label = open
    ? "Minimize Superhost"
    : needsInput
      ? "Open Superhost — waiting on your approval"
      : activeInBackground
        ? "Open Superhost — active in background"
        : "Open Superhost";

  return (
    <button
      type="button"
      className="global-superhost-button"
      data-open={open}
      data-active={activeInBackground}
      // needsInput takes the button over visually even while active --
      // "come approve this" is the more urgent of the two states.
      data-needs-input={needsInput}
      data-control-exempt="true"
      onClick={onOpen}
      aria-label={label}
      aria-expanded={open}
    >
      <span className="global-superhost-button-status" aria-hidden="true" />
      SUPERHOST <span aria-hidden="true">{open ? "—" : "↗"}</span>
      {activeInBackground && !needsInput && <span className="sr-only">Active in background</span>}
      {needsInput && <span className="sr-only">Waiting on your approval</span>}
    </button>
  );
}
