import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import Modal from "./ui/Modal";
import { onSessionExpired } from "../lib/auth/session-expiry";
import { signOut } from "../lib/auth/session";

// Mounted once at the app root (main.tsx). Subscribes to the session-expiry
// pub-sub api() publishes to on a real 401 -- the one place a plain async
// function needs to make a mounted React component do something. Route
// guards (RequireRole, StaffGate, OwnerGate) still silently redirect for the
// "never had a session" case; this modal is specifically for "had one, and
// it just ended," which deserves an explanation, not a silent bounce.
export default function SessionExpiredModal() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    return onSessionExpired(() => {
      signOut();
      setOpen(true);
    });
  }, []);

  function handleSignInAgain() {
    setOpen(false);
    navigate("/login", { replace: true });
  }

  return (
    <Modal
      open={open}
      onClose={handleSignInAgain}
      title="Your session ended"
      label="SESSION / EXPIRED"
      closeLabel="Sign in again"
    >
      <p>
        The demo session backing this tab is no longer valid — it may have
        expired, or been reset from another tab. Nothing you had open was
        saved automatically; the server is the record of what actually
        happened.
      </p>
      <button type="button" className="button button-solid" onClick={handleSignInAgain}>
        SIGN IN AGAIN →
      </button>
    </Modal>
  );
}
