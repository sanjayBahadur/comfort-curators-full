import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import "./intro.css";

const INTRO_SEEN_KEY = "cc_intro_seen";
const BEAT_DURATION = 900;
const MAX_DURATION = 5000;

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

export default function Intro() {
  const navigate = useNavigate();
  const [started, setStarted] = useState(false);
  const [beat, setBeat] = useState(0);

  useEffect(() => {
    if (sessionStorage.getItem(INTRO_SEEN_KEY) === "true") {
      navigate("/login", { replace: true });
      return;
    }

    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      sessionStorage.setItem(INTRO_SEEN_KEY, "true");
      navigate("/login", { replace: true });
      return;
    }

    setStarted(true);
    let cancelled = false;
    // No live data fetch here anymore -- see git history for the prior
    // version, which tried to show a real properties/tickets/curators
    // "LIVE READOUT" on a page that loads before login. /v1/properties
    // (like nearly everything else in this API) requires an authenticated
    // session, so for a genuinely first-time visitor -- the only audience
    // this page is ever actually shown to -- that fetch 401ed every time
    // and silently resolved to null. All three numbers were permanently
    // empty for the real audience; confirmed live, not a timing fluke.
    // Beat three is real, static copy instead of a live readout that
    // can't be live here.
    const fonts = document.fonts?.ready.catch(() => undefined) ?? Promise.resolve();
    const work = Promise.all([wait(1800), wait(BEAT_DURATION * 4), fonts]);
    const ceiling = wait(MAX_DURATION);

    const beatTimer = window.setInterval(() => {
      setBeat((current) => Math.min(current + 1, 3));
    }, BEAT_DURATION);

    void Promise.race([work, ceiling]).then(() => {
      if (!cancelled) {
        sessionStorage.setItem(INTRO_SEEN_KEY, "true");
        navigate("/login", { replace: true });
      }
    });

    return () => {
      cancelled = true;
      window.clearInterval(beatTimer);
    };
  }, [navigate]);

  function skip() {
    sessionStorage.setItem(INTRO_SEEN_KEY, "true");
    navigate("/login", { replace: true });
  }

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") skip();
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  if (!started) return null;

  return (
    <main className="intro-page" data-beat={beat}>
      <button className="intro-skip" type="button" onClick={skip}>
        SKIP · ESC
      </button>

      <section className="intro-beat intro-beat-one" aria-label="A property should feel handled">
        <p className="intro-index">COMFORT CURATORS / ENTRY SEQUENCE</p>
        <h1>
          A property should feel
          <em>handled.</em>
        </h1>
        <span className="intro-mark">01</span>
      </section>

      <section className="intro-beat intro-beat-two" aria-label="One system, every moving part">
        <p className="intro-index">02 / A SYSTEM WITH EDGES</p>
        <h2 className="intro-beat-two-heading">
          Turnover. Dispatch. Billing.
          <br />
          Documents. Compliance.
          <em> One system, not five tools stitched together.</em>
        </h2>
        <div className="intro-registration intro-registration-top" />
        <div className="intro-rule-grid" aria-hidden="true">
          {Array.from({ length: 12 }, (_, index) => <span key={index} style={{ "--column": index } as React.CSSProperties} />)}
        </div>
        <div className="intro-registration intro-registration-bottom" />
      </section>

      <section className="intro-beat intro-beat-three" aria-label="Meet Superhost">
        <p className="intro-index">03 / MEET SUPERHOST</p>
        <h2 className="intro-beat-three-heading">An AI supervisor for the property, not a chatbot bolted on.</h2>
        <div className="intro-stats">
          <Capability
            label="PROPOSES"
            copy="Every action starts as a proposal. Nothing real happens until it's actually allowed."
          />
          <Capability
            label="A HUMAN DECIDES"
            copy="Real changes — a ticket, an order, an approval — still need a person to say yes."
          />
          <Capability
            label="WHERE YOU ALREADY ARE"
            copy="The owner dashboard, a property's own page, a guest's stay — one assistant, real actions."
          />
        </div>
      </section>

      <section className="intro-beat intro-beat-four" aria-label="Comfort Curators">
        <span className="intro-wordmark">COMFORT<br />CURATORS</span>
        <p className="intro-index">04 / HANDING OFF TO ACCESS</p>
      </section>
    </main>
  );
}

function Capability({ label, copy }: { label: string; copy: string }) {
  return (
    <div className="intro-stat">
      <span>{label}</span>
      <p className="intro-capability-copy">{copy}</p>
    </div>
  );
}

