import type { ReactNode } from "react";
import "./Terminal.css";

export type TerminalLineKind = "agent" | "operator" | "denial" | "system";

export type TerminalLine = {
  id: string;
  kind: TerminalLineKind;
  text: string;
};

export type TerminalProps = {
  lines: TerminalLine[];
  /** Structured activity rendered after ordinary prelude lines. */
  activity?: ReactNode;
  /** Show the CSS-only cursor while the caller waits for the next event. */
  cursorVisible?: boolean;
  /** Content such as the approval block, rendered after the line list. */
  children?: ReactNode;
  className?: string;
};

const linePrefix: Record<TerminalLineKind, string> = {
  agent: "> ",
  operator: "$ ",
  denial: "! ",
  system: "· ",
};

export default function Terminal({
  lines,
  activity,
  cursorVisible = false,
  children,
  className,
}: TerminalProps) {
  const terminalClassName = ["superhost-terminal", className].filter(Boolean).join(" ");

  return (
    // data-lenis-prevent: this site runs a global Lenis smooth-scroll
    // instance that captures wheel events at the document level by
    // default -- without this opt-out (the same pattern expansion.tsx
    // and debug.tsx already use for their own inner-scrolling panels),
    // scrolling inside this bounded, overflow-y: auto terminal instead
    // scrolled the whole page.
    <div className={terminalClassName} data-lenis-prevent>
      <ol className="superhost-terminal-lines" aria-label="Superhost activity">
        {lines.map((line) => (
          <li className={`superhost-terminal-line superhost-terminal-line-${line.kind}`} key={line.id}>
            <span className="superhost-terminal-prefix" aria-hidden="true">{linePrefix[line.kind]}</span>
            <span className="superhost-terminal-text" role={line.kind === "denial" ? "alert" : undefined}>{line.text}</span>
          </li>
        ))}
      </ol>
      {activity}
      {cursorVisible && <span className="superhost-terminal-cursor" aria-hidden="true" />}
      {children}
    </div>
  );
}
