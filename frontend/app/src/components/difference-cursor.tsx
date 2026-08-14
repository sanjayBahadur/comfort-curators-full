import { useEffect, useRef } from "react";

const LERP = 0.18;
const POINTER_QUERY = "(hover: hover) and (pointer: fine)";
const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

const lerp = (from: number, to: number, amount: number) =>
  from + (to - from) * amount;

export default function DifferenceCursor() {
  const cursorRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const cursor = cursorRef.current;
    if (!cursor) return;

    const finePointer = window.matchMedia(POINTER_QUERY);
    const reducedMotion = window.matchMedia(REDUCED_MOTION_QUERY);
    const position = { currentX: 0, currentY: 0, targetX: 0, targetY: 0 };
    let currentScale = 1;
    let targetScale = 1;
    let hasPointerPosition = false;
    let keyboardMode = false;
    let frame = 0;

    const isCapable = () => finePointer.matches && !reducedMotion.matches;

    const hide = () => {
      cursor.dataset.visible = "false";
      document.documentElement.classList.remove("has-custom-cursor");
    };

    const show = () => {
      if (!isCapable() || keyboardMode || !hasPointerPosition) {
        hide();
        return;
      }

      cursor.dataset.visible = "true";
      document.documentElement.classList.add("has-custom-cursor");
    };

    const onPointerMove = (event: PointerEvent) => {
      if (!isCapable()) return;

      position.targetX = event.clientX;
      position.targetY = event.clientY;
      keyboardMode = false;

      if (!hasPointerPosition) {
        position.currentX = event.clientX;
        position.currentY = event.clientY;
        hasPointerPosition = true;
      }

      show();
    };

    const onPointerOver = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) {
        targetScale = 1;
        return;
      }

      const scaledTarget = target.closest<HTMLElement>("[data-cursor-scale]");
      const requestedScale = Number(scaledTarget?.dataset.cursorScale);

      if (scaledTarget && Number.isFinite(requestedScale) && requestedScale > 0) {
        targetScale = requestedScale;
        return;
      }

      targetScale = target.closest("a, button, input, select, textarea, [data-cursor-grow]")
        ? 1.9
        : 1;
    };

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Tab") return;
      keyboardMode = true;
      hide();
    };

    const onCapabilityChange = () => {
      if (isCapable()) show();
      else hide();
    };

    const render = () => {
      position.currentX = lerp(position.currentX, position.targetX, LERP);
      position.currentY = lerp(position.currentY, position.targetY, LERP);
      currentScale = lerp(currentScale, targetScale, LERP);
      cursor.style.transform = `translate3d(${position.currentX}px, ${position.currentY}px, 0) translate(-50%, -50%) scale(${currentScale})`;
      frame = requestAnimationFrame(render);
    };

    window.addEventListener("pointermove", onPointerMove, { passive: true });
    window.addEventListener("pointerover", onPointerOver, { passive: true });
    window.addEventListener("blur", hide);
    document.addEventListener("keydown", onKeyDown);
    finePointer.addEventListener("change", onCapabilityChange);
    reducedMotion.addEventListener("change", onCapabilityChange);
    frame = requestAnimationFrame(render);

    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerover", onPointerOver);
      window.removeEventListener("blur", hide);
      document.removeEventListener("keydown", onKeyDown);
      finePointer.removeEventListener("change", onCapabilityChange);
      reducedMotion.removeEventListener("change", onCapabilityChange);
      hide();
    };
  }, []);

  return <div ref={cursorRef} className="difference-cursor" data-visible="false" aria-hidden="true" />;
}
