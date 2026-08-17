import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import GlobalSuperhostButton from "./GlobalSuperhostButton";
import GlobalSuperhostDrawer from "./GlobalSuperhostDrawer";
import { useControlSession } from "./ControlSession";
import { SUPERHOST_MESSAGE_SENT_EVENT } from "./SuperhostMount";
import { SUPERHOST_NEEDS_INPUT_EVENT } from "./ConfirmBlock";

// One mount point for the button + the drawer it opens, so main.tsx doesn't
// need its own state (it's a render call, not a component).
export default function GlobalSuperhost() {
  const [open, setOpen] = useState(false);
  const [needsInput, setNeedsInput] = useState(false);
  const { pathname } = useLocation();
  const { session } = useControlSession();
  const activeInBackground = session.state === "granted" && !open;

  // A pending approval is a real "come look" signal regardless of which
  // page/thread raised it (a page-embedded terminal counts too, not just
  // the global drawer) -- the launcher is the one thing visible from
  // anywhere, so it's the one thing that should glow. This does not
  // auto-open the drawer; it only flags that opening it is worth doing.
  useEffect(() => {
    function handleNeedsInput(event: Event) {
      setNeedsInput(Boolean((event as CustomEvent<boolean>).detail));
    }
    window.addEventListener(SUPERHOST_NEEDS_INPUT_EVENT, handleNeedsInput);
    return () => window.removeEventListener(SUPERHOST_NEEDS_INPUT_EVENT, handleNeedsInput);
  }, []);

  // Get out of the way once Superhost actually has something to do, not
  // the instant control is granted: the composer that lets you direct it
  // lives inside this same drawer, so minimizing on grant made it
  // impossible to type the very instruction "hand over control" is
  // supposed to be followed by (verified live -- the input becomes
  // invisible before a single character can be typed). Minimizing on
  // send instead matches the manual demo script's own sequencing (HAND
  // OVER CONTROL, fill/type, SEND, then minimize) while removing the
  // step a presenter used to have to remember by hand. Global-drawer
  // sends only -- SuperhostMount fires this event solely from its
  // routeKey="global" embedding, so a page-embedded terminal (stay.tsx,
  // ops-ticket-detail.tsx) sending a message never closes anything here.
  useEffect(() => {
    function handleMessageSent() {
      setOpen(false);
    }
    window.addEventListener(SUPERHOST_MESSAGE_SENT_EVENT, handleMessageSent);
    return () => window.removeEventListener(SUPERHOST_MESSAGE_SENT_EVENT, handleMessageSent);
  }, []);

  if (pathname === "/expansion") return null;

  return (
    <>
      <GlobalSuperhostButton
        open={open}
        activeInBackground={activeInBackground}
        needsInput={needsInput}
        onOpen={() => setOpen((current) => !current)}
      />
      <GlobalSuperhostDrawer open={open} onClose={() => setOpen(false)} />
    </>
  );
}
