import { useState } from "react";
import { useLocation } from "react-router-dom";
import GlobalSuperhostButton from "./GlobalSuperhostButton";
import GlobalSuperhostDrawer from "./GlobalSuperhostDrawer";
import { useControlSession } from "./ControlSession";

// One mount point for the button + the drawer it opens, so main.tsx doesn't
// need its own state (it's a render call, not a component).
export default function GlobalSuperhost() {
  const [open, setOpen] = useState(false);
  const { pathname } = useLocation();
  const { session } = useControlSession();
  const activeInBackground = session.state === "granted" && !open;

  if (pathname === "/expansion") return null;

  return (
    <>
      <GlobalSuperhostButton
        open={open}
        activeInBackground={activeInBackground}
        onOpen={() => setOpen((current) => !current)}
      />
      <GlobalSuperhostDrawer open={open} onClose={() => setOpen(false)} />
    </>
  );
}
