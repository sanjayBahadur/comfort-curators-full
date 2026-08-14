import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { getProperties } from "../lib/api/shop";
import "./intro.css";

const INTRO_SEEN_KEY = "cc_intro_seen";
const BEAT_DURATION = 900;
const MAX_DURATION = 5000;

type IntroStats = {
  properties: number | null;
  tickets: null;
  curators: null;
};

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

export default function Intro() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [started, setStarted] = useState(false);
  const [beat, setBeat] = useState(0);
  const [stats, setStats] = useState<IntroStats>({ properties: null, tickets: null, curators: null });

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
    const properties = queryClient
      .fetchQuery({ queryKey: ["properties"], queryFn: getProperties })
      .then((payload) => payload.items.length)
      .catch(() => null);
    const fonts = document.fonts?.ready.catch(() => undefined) ?? Promise.resolve();
    const work = Promise.all([wait(1800), wait(BEAT_DURATION * 4), properties, fonts]);
    const ceiling = wait(MAX_DURATION);

    void properties.then((count) => {
      if (!cancelled) setStats({ properties: count, tickets: null, curators: null });
    });

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
  }, [navigate, queryClient]);

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

      <section className="intro-beat intro-beat-two" aria-label="A ruled operating system">
        <div className="intro-registration intro-registration-top" />
        <div className="intro-rule-grid" aria-hidden="true">
          {Array.from({ length: 12 }, (_, index) => <span key={index} style={{ "--column": index } as React.CSSProperties} />)}
        </div>
        <div className="intro-registration intro-registration-bottom" />
        <p className="intro-grid-label">02 / A SYSTEM WITH EDGES</p>
      </section>

      <section className="intro-beat intro-beat-three" aria-label="Operating status">
        <p className="intro-index">03 / LIVE READOUT</p>
        <div className="intro-stats">
          <Stat label="PROPERTIES" value={stats.properties} />
          <Stat label="OPEN TICKETS" value={stats.tickets} />
          <Stat label="CURATORS ON SHIFT" value={stats.curators} />
        </div>
      </section>

      <section className="intro-beat intro-beat-four" aria-label="Comfort Curators">
        <span className="intro-wordmark">COMFORT<br />CURATORS</span>
        <p className="intro-index">04 / HANDING OFF TO ACCESS</p>
      </section>
    </main>
  );
}

function Stat({ label, value }: { label: string; value: number | null }) {
  return (
    <div className="intro-stat">
      <span>{label}</span>
      <strong>{value === null ? "—" : value}</strong>
    </div>
  );
}
