import { cloneElement, useId, type HTMLAttributes, type ReactElement, type ReactNode } from "react";
import "./Tooltip.css";

export type TooltipProps = {
  label: ReactNode;
  children: ReactElement<HTMLAttributes<HTMLElement>>;
  id?: string;
  className?: string;
};

export default function Tooltip({ label, children, id, className }: TooltipProps) {
  const generatedId = useId();
  const tooltipId = id ?? `tooltip-${generatedId}`;

  return (
    <span className={`tooltip ${className ?? ""}`.trim()}>
      {cloneElement(children, { "aria-describedby": tooltipId })}
      <span id={tooltipId} role="tooltip" className="tooltip-bubble">
        {label}
      </span>
    </span>
  );
}
