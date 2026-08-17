import { createPortal } from "react-dom";
import { useEffect, useId, useRef, useState, type AnimationEvent, type KeyboardEvent, type ReactNode } from "react";
import "./Modal.css";

export type ModalProps = {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  label: string;
  children: ReactNode;
  className?: string;
  closeLabel?: string;
};

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "area[href]",
  "button:not(:disabled)",
  "input:not(:disabled)",
  "select:not(:disabled)",
  "textarea:not(:disabled)",
  "iframe",
  "object",
  "embed",
  "[contenteditable]",
  "[tabindex]:not([tabindex=\"-1\"])",
].join(",");

export default function Modal({
  open,
  onClose,
  title,
  label,
  children,
  className,
  closeLabel = "Close popup",
}: ModalProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const [mounted, setMounted] = useState(open);

  useEffect(() => {
    if (open) {
      setMounted(true);
      if (!returnFocusRef.current && document.activeElement instanceof HTMLElement) {
        returnFocusRef.current = document.activeElement;
      }
      return;
    }

    returnFocusRef.current?.focus();
  }, [open]);

  useEffect(() => {
    if (!mounted || !open) return;

    const dialog = dialogRef.current;
    if (!dialog) return;

    const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
    (focusable[0] ?? dialog).focus();
  }, [mounted, open]);

  useEffect(() => () => {
    returnFocusRef.current?.focus();
  }, []);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }

    if (event.key !== "Tab") return;

    const dialog = dialogRef.current;
    if (!dialog) return;
    const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
    if (focusable.length === 0) {
      event.preventDefault();
      dialog.focus();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  function handleAnimationEnd(event: AnimationEvent<HTMLDivElement>) {
    if (!open && event.currentTarget === event.target) setMounted(false);
  }

  if (!mounted) return null;

  return createPortal(
    <div className="modal-portal">
      <div
        ref={dialogRef}
        className={`modal ${!open ? "modal-exiting" : ""} ${className ?? ""}`.trim()}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        onAnimationEnd={handleAnimationEnd}
        // Same bug as the Superhost terminal and package cart column: this
        // has real overflow-y: auto + max-height, but Lenis hijacks its
        // wheel events unless it's opted out via data-lenis-prevent (see
        // smooth-scroll.tsx's prevent callback). Every popup built on this
        // shared Modal -- including "YOUR PACKAGE", the full cart view --
        // needs this, not just the callers that happened to add it
        // themselves elsewhere.
        data-lenis-prevent
      >
        {/* Callers should show at most one popup per session. */}
        <p className="modal-index">{label}</p>
        <button className="modal-close" type="button" aria-label={closeLabel} onClick={onClose}>
          CLOSE ×
        </button>
        <h2 id={titleId} className="modal-title">{title}</h2>
        <div className="modal-content">{children}</div>
      </div>
    </div>,
    document.body,
  );
}
