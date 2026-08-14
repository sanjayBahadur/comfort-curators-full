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
