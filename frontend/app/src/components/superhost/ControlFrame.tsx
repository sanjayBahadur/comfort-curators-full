import { useControlSession } from "./ControlSession";
import "./control-frame.css";

export function ControlFrame() {
  const { session, revoke, timeDisplay } =
    useControlSession();

  // The grant entry point lives in GlobalSuperhostDrawer now -- the
  // terminal is where "hand over control" actually makes sense to a user,
  // not a floating button that had nothing to do with what it was granting
  // access to. This component now only renders the live-session frame.
  if (session.state !== "granted") {
    return null;
  }

  return (
    <>
      <div className="control-frame-page-border" aria-hidden="true" />
      <div
        className="control-frame-strip"
        data-control-revoke
        onClick={() => revoke("click_strip")}
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        <span className="control-frame-strip-text">
          <span>▌ SUPERHOST HAS CONTROL</span>
          <span>·</span>
          <span>{timeDisplay} REMAINING</span>
          <span>·</span>
          <span>{String(session.actionCount).padStart(2, "0")}/{session.maxActions} ACTIONS</span>
        </span>
        <span className="control-frame-revoke-hint">ESC TO REVOKE</span>
      </div>
    </>
  );
}
