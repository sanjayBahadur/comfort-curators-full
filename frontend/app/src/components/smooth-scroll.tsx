import { useEffect } from "react";
import Lenis from "lenis";

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

export default function SmoothScroll() {
  useEffect(() => {
    const reducedMotion = window.matchMedia(REDUCED_MOTION_QUERY);
    let lenis: Lenis | null = null;

    const stop = () => {
      lenis?.destroy();
      lenis = null;
      document.documentElement.removeAttribute("data-lenis-active");
    };

    const syncPreference = () => {
      stop();

      if (reducedMotion.matches) return;

      lenis = new Lenis({
        anchors: true,
        autoRaf: true,
        lerp: 0.1,
        smoothWheel: true,
        stopInertiaOnNavigate: true,
        respectReducedMotion: true,
        // Lenis does NOT look at [data-lenis-prevent] on its own -- that
        // attribute is only meaningful if something evaluates it, via this
        // exact callback. Without it, every element marked
        // data-lenis-prevent (the Superhost terminal, the package cart
        // column, the filter sidebar, debug/expansion panels...) still has
        // its own internal wheel scrolling hijacked by Lenis's page-level
        // smooth scroll, so it reads as "unscrollable" even though the CSS
        // (overflow-y: auto) is correct.
        prevent: (node) => Boolean(node.closest("[data-lenis-prevent]")),
      });
      document.documentElement.setAttribute("data-lenis-active", "true");
    };

    syncPreference();
    reducedMotion.addEventListener("change", syncPreference);

    return () => {
      reducedMotion.removeEventListener("change", syncPreference);
      stop();
    };
  }, []);

  return null;
}
