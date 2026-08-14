import { useState, type AnimationEvent } from "react";
import "./CookieSlip.css";

const CONSENT_KEY = "cc_cookie_consent";

type Consent = "all" | "necessary";

function hasStoredConsent(): boolean {
  return typeof window !== "undefined" && window.localStorage.getItem(CONSENT_KEY) !== null;
}

export default function CookieSlip() {
  const [mounted, setMounted] = useState(() => !hasStoredConsent());
  const [exiting, setExiting] = useState(false);

  function choose(consent: Consent) {
    window.localStorage.setItem(CONSENT_KEY, consent);
    setExiting(true);
  }

  function handleAnimationEnd(event: AnimationEvent<HTMLElement>) {
    if (exiting && event.currentTarget === event.target) setMounted(false);
  }

  if (!mounted) return null;

  return (
    <aside
      className={`cookie-slip ${exiting ? "cookie-slip-exiting" : ""}`.trim()}
      aria-label="Cookie preferences"
      onAnimationEnd={handleAnimationEnd}
    >
      <div className="cookie-slip-tape" aria-hidden="true" />
      <div className="cookie-slip-halftone" aria-hidden="true" />
      <header className="cookie-slip-header" aria-hidden="true">
        ACCEPT ALL <span>/</span> NECESSARY ONLY
      </header>
      <p className="cookie-slip-copy">
        A few keep the lights on. The rest help us remember what you came for.
      </p>
      <div className="cookie-slip-actions">
        <button type="button" onClick={() => choose("all")}>ACCEPT ALL</button>
        <button type="button" onClick={() => choose("necessary")}>NECESSARY ONLY</button>
      </div>
      <footer className="cookie-slip-footer">PRIVACY · 01</footer>
    </aside>
  );
}
