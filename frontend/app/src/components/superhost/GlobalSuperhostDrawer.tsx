import { useEffect, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";
import { useQuery } from "@tanstack/react-query";
import SuperhostMount from "./SuperhostMount";
import { getCurrentProperty, setCurrentProperty, subscribeCurrentProperty } from "../../lib/current-property";
import { getOpsProperties } from "../../lib/api/ops";
import { propertyName } from "../../routes/ops-format";
import "./global-superhost-drawer.css";

// The drawer is mounted once at the app root with no route params of its
// own -- most pages set a "current property" (see current-property.ts),
// but a page that manages more than one property (the owner dashboard,
// when there's more than a single property) deliberately doesn't, so a
// page-embedded SuperhostMount never assumes which one you mean.
//
// The drawer itself no longer needs a property before it can do
// anything, though: with no current property in view, it now connects a
// portfolio-scoped thread (SuperhostMount's allowPortfolio) that can see
// and act across every property this account manages in one
// conversation -- "manage individual properties in parallel" without
// forcing a pick first. This picker is offered as an optional way to
// narrow the conversation to one property instead (e.g. if you know
// you'll only be talking about one), not a gate blocking the composer
// the way it used to be.
//
// Built as its own terminal-style list, not the shared <Select> --
// dropping a light-chrome form control into the dark phosphor terminal
// looked like a foreign widget grafted onto the machine surface. This
// reads as part of the console: mono labels, phosphor-green rule/hover,
// no border-radius/shadow, keyboard-navigable like the rest of the app's
// controls.
function PropertyFallbackPicker() {
  const propertiesQuery = useQuery({ queryKey: ["global-superhost", "properties"], queryFn: getOpsProperties });
  const properties = propertiesQuery.data?.items ?? [];

  if (propertiesQuery.isLoading || propertiesQuery.isError || properties.length === 0) {
    return null;
  }

  return (
    <details className="gsh-drawer-picker">
      <summary className="gsh-drawer-picker-label">scope to one property (optional) /</summary>
      <ul className="gsh-drawer-picker-list">
        {properties.map((property) => (
          <li key={property.id}>
            <button
              type="button"
              className="gsh-drawer-picker-option"
              onClick={() => setCurrentProperty({ id: property.id, label: propertyName(property) })}
            >
              <span aria-hidden="true">{"> "}</span>
              {propertyName(property)}
            </button>
          </li>
        ))}
      </ul>
    </details>
  );
}

export default function GlobalSuperhostDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const property = useSyncExternalStore(subscribeCurrentProperty, getCurrentProperty);

  useEffect(() => {
    if (!open) return undefined;
    const handleKey = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [open, onClose]);

  return createPortal(
    <div className="gsh-drawer-portal" data-open={open} data-control-exempt="true">
      <button className="gsh-drawer-scrim" type="button" aria-label="Close Superhost" onClick={onClose} />
      <aside className="gsh-drawer" role="dialog" aria-modal={open ? "true" : undefined} aria-hidden={!open} aria-labelledby="gsh-drawer-title">
        <header className="gsh-drawer-header">
          <div>
            <span>SUPERHOST</span>
            <h2 id="gsh-drawer-title">{property ? property.label : "Whole portfolio"}</h2>
          </div>
          <button type="button" className="gsh-drawer-close" onClick={onClose} aria-label="Minimize Superhost">—</button>
        </header>

        {/* Handing over control now lives inside SuperhostMount itself
            (see SuperhostMount.tsx) so every embedding -- this drawer and
            every page-embedded mount -- gets the same grant flow. A
            second, separate grant button here would be a confusing
            duplicate. */}
        {!property && <PropertyFallbackPicker />}

        <div className="gsh-drawer-mount">
          <SuperhostMount
            propertyId={property?.id ?? null}
            routeKey="global"
            purpose={property ? `Global Superhost assistance for ${property.label}` : "Global Superhost assistance across the whole portfolio"}
            emptyMessage="not connected"
            allowPortfolio
          />
        </div>
      </aside>
    </div>,
    document.body,
  );
}
