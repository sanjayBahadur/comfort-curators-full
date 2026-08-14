import type { ChecklistTask, ChecklistTaskState } from "./task-checklist";
import type { TerminalLineKind } from "./Terminal";
import "./TaskChecklist.css";

const taskPresentation: Record<ChecklistTaskState, { marker: string; label: string }> = {
  not_started: { marker: "[ ]", label: "NOT STARTED" },
  running: { marker: "[~]", label: "RUNNING" },
  done: { marker: "[x]", label: "DONE" },
  waiting: { marker: "[?]", label: "WAITING FOR APPROVAL" },
  denied: { marker: "[!]", label: "DENIED" },
  blocked: { marker: "[!]", label: "BLOCKED" },
  unknown: { marker: "[?]", label: "STATUS UNKNOWN" },
};

const stepPrefix: Record<TerminalLineKind, string> = {
  agent: "> ",
  operator: "$ ",
  denial: "! ",
  system: "· ",
};

export default function TaskChecklist({ tasks }: { tasks: ChecklistTask[] }) {
  if (tasks.length === 0) return null;

  return (
    <ol className="superhost-task-list" aria-label="Superhost task checklist">
      {tasks.map((task) => {
        const presentation = taskPresentation[task.state];
        // The last step is always the one that actually matters -- the
        // real final answer for a done task, the denial reason for a
        // denied one, the approval summary for one waiting on you. It
        // used to be buried inside the same collapsed trace as every
        // intermediate "proposed X"/"run queued" line, which meant
        // collapsing the noisy trace (see below) also hid the one thing
        // a normal user actually came here to read. Always show it;
        // collapse everything before it.
        const lastStep = task.steps.at(-1);
        const priorSteps = task.steps.slice(0, -1);
        return (
          <li className={`superhost-task superhost-task-${task.state}`} key={task.id}>
            <div className="superhost-task-heading">
              <span className="superhost-task-marker" aria-hidden="true">{presentation.marker}</span>
              <span className="superhost-task-title superhost-task-wrap">{task.title}</span>
              <span className="superhost-task-state">{presentation.label}</span>
            </div>
            {lastStep && (
              <p className={`superhost-task-answer superhost-terminal-line-${lastStep.kind}`} role={lastStep.kind === "denial" ? "alert" : undefined}>
                <span className="superhost-task-branch" aria-hidden="true">|-- </span>
                <span className="superhost-terminal-prefix" aria-hidden="true">{stepPrefix[lastStep.kind]}</span>
                {lastStep.text}
              </p>
            )}
            {priorSteps.length > 0 && (
              // Closed by default: the raw tool-call trace (proposed X,
              // policy result, run state...) is real and worth keeping
              // inspectable, but showing it unconditionally under every
              // task is what made the terminal read as "too complicated
              // for a normal user". <details> keeps every line, just not
              // open by default.
              <details
                className="superhost-task-steps-detail"
                open={task.state === "denied" || task.state === "blocked"}
              >
                <summary>
                  {priorSteps.length} earlier step{priorSteps.length === 1 ? "" : "s"}
                </summary>
                <ol className="superhost-task-steps" aria-label={`Steps for ${task.title}`}>
                  {priorSteps.map((step) => (
                    <li className={`superhost-task-step superhost-terminal-line-${step.kind}`} key={step.id}>
                      <span className="superhost-task-branch" aria-hidden="true">|-- </span>
                      <span className="superhost-terminal-prefix" aria-hidden="true">{stepPrefix[step.kind]}</span>
                      <span className="superhost-task-wrap" role={step.kind === "denial" ? "alert" : undefined}>{step.text}</span>
                    </li>
                  ))}
                </ol>
              </details>
            )}
          </li>
        );
      })}
    </ol>
  );
}
