import { useEffect, useRef, useState, type CSSProperties } from "react";
import { useControlSession } from "./ControlSession";
import "./agent-cursor.css";

type Point = { x: number; y: number };

type TrailParticle = {
  id: number;
  from: Point;
  to: Point;
};

function Particle({
  from,
  to,
  index,
  duration,
}: {
  from: Point;
  to: Point;
  index: number;
  duration: number;
}) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const raf = requestAnimationFrame(() => setReady(true));
    return () => cancelAnimationFrame(raf);
  }, []);

  const staggerMs = index * 60;
  const style: CSSProperties = ready
    ? {
        transform: `translate(${to.x}px, ${to.y}px) translate(-50%, -50%)`,
        opacity: 0,
        transition: `all ${duration}ms var(--ease-expo-out) ${staggerMs}ms`,
      }
    : {
        transform: `translate(${from.x}px, ${from.y}px) translate(-50%, -50%)`,
        opacity: 1,
      };

  return <span className="agent-cursor-particle" style={style} />;
}

const PARTICLE_COUNT = 5;
const PARTICLE_CLEANUP_EXTRA_MS = 400;

export default function AgentCursor() {
  const { session } = useControlSession();
  const [target, setTarget] = useState<Point | null>(null);
  const [visible, setVisible] = useState(false);
  const [reduced, setReduced] = useState(false);
  const [particles, setParticles] = useState<TrailParticle[]>([]);
  const particleIdRef = useRef(0);
  const cleanupTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(mq.matches);
    const handler = (e: MediaQueryListEvent) => setReduced(e.matches);
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  useEffect(() => {
    if (session.state !== "granted") {
      setVisible(false);
      setTarget(null);
      setParticles([]);
      return;
    }

    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          if (!(node instanceof HTMLElement)) continue;
          if (
            !node.classList.contains("control-ring") &&
            !node.classList.contains("control-ring-reduced")
          ) {
            continue;
          }
          const left = parseFloat(node.style.left);
          const top = parseFloat(node.style.top);
          const width = parseFloat(node.style.width);
          const height = parseFloat(node.style.height);
          if (isNaN(left) || isNaN(top) || isNaN(width) || isNaN(height)) continue;

          const cx = left + width / 2;
          const cy = top + height / 2;

          setTarget((prev) => {
            const next: Point = { x: cx, y: cy };
            if (prev && !reduced) {
              const duration = reduced ? 100 : 400;
              const newParticles: TrailParticle[] = Array.from(
                { length: PARTICLE_COUNT },
                () => ({
                  id: particleIdRef.current++,
                  from: prev,
                  to: next,
                }),
              );
              setParticles((existing) => [...existing, ...newParticles]);

              const maxDelay = (PARTICLE_COUNT - 1) * 60;
              const cleanupAfter = duration + maxDelay + PARTICLE_CLEANUP_EXTRA_MS;
              const idsToRemove = new Set(newParticles.map((p) => p.id));
              const timer = setTimeout(() => {
                setParticles((existing) =>
                  existing.filter((p) => !idsToRemove.has(p.id)),
                );
              }, cleanupAfter);
              cleanupTimersRef.current.push(timer);
            }
            return next;
          });
          setVisible(true);
        }
      }
    });

    observer.observe(document.body, { childList: true });
    return () => {
      observer.disconnect();
      for (const t of cleanupTimersRef.current) clearTimeout(t);
      cleanupTimersRef.current = [];
    };
  }, [session.state, reduced]);

  if (session.state !== "granted") return null;

  const duration = reduced ? 100 : 400;
  const idlePos: CSSProperties = {
    transform: "translate(50vw, 50vh) translate(-50%, -50%)",
  };
  const targetPos: CSSProperties = target
    ? {
        transform: `translate(${target.x}px, ${target.y}px) translate(-50%, -50%)`,
      }
    : {};

  const baseStyle: CSSProperties = reduced
    ? { transition: "none" }
    : { transition: `transform ${duration}ms var(--ease-expo-out), opacity 200ms ease-out` };

  return (
    <>
      <div
        className={
          `agent-cursor` +
          (visible ? " agent-cursor--visible" : " agent-cursor--idle")
        }
        style={target ? { ...baseStyle, ...targetPos } : { ...baseStyle, ...idlePos }}
      />
      {particles.map((p, i, arr) => {
        const staggerIndex = arr.length - 1 - i;
        return (
          <Particle
            key={p.id}
            from={p.from}
            to={p.to}
            index={staggerIndex}
            duration={duration}
          />
        );
      })}
    </>
  );
}
