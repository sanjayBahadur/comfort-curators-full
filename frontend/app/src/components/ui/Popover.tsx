import {
  cloneElement,
  useEffect,
  useId,
  useRef,
  type HTMLAttributes,
  type ReactElement,
  type ReactNode,
} from "react";
import "./Popover.css";

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

export type PopoverProps = {
  trigger: ReactElement<HTMLAttributes<HTMLElement>>;
  open: boolean;
  onClose: () => void;
  label: string;
  children: ReactNode;
  className?: string;
};

export default function Popover({ trigger, open, onClose, label, children, className }: PopoverProps) {
  const generatedId = useId();
  const panelId = `popover-${generatedId}`;
  const wrapperRef = useRef<HTMLSpanElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  function focusTrigger() {
    const wrapper = wrapperRef.current;
    const triggerNode = wrapper?.firstElementChild;
    if (triggerNode instanceof HTMLElement) triggerNode.focus();
  }

  useEffect(() => {
    if (!open) return;
    const panel = panelRef.current;
    if (!panel) return;
    const focusable = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
    (focusable[0] ?? panel).focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;

    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        focusTrigger();
      }
    };

    const closeOnOutsidePointer = (event: globalThis.PointerEvent) => {
      if (!(event.target instanceof Node) || wrapperRef.current?.contains(event.target)) return;
      onClose();
      focusTrigger();
    };

    document.addEventListener("keydown", closeOnEscape);
    document.addEventListener("pointerdown", closeOnOutsidePointer);
    return () => {
      document.removeEventListener("keydown", closeOnEscape);
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
    };
  }, [open, onClose]);

  return (
    <span ref={wrapperRef} className={`popover ${className ?? ""}`.trim()}>
      {cloneElement(trigger, {
        "aria-haspopup": "dialog",
        "aria-expanded": open,
        "aria-controls": panelId,
      })}
      {open && (
        <div
          ref={panelRef}
          id={panelId}
          className="popover-panel"
          role="dialog"
          aria-label={label}
          tabIndex={-1}
        >
          {children}
        </div>
      )}
    </span>
  );
}
