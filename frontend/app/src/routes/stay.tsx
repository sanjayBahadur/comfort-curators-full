import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import Money from "../components/money";
import Modal from "../components/ui/Modal";
import Select from "../components/ui/Select";
import { getOpsProperties, getReservations, createTicket, type TicketType } from "../lib/api/ops";
import { getStoreCatalog, createStoreQuote, placeStoreOrder, type StoreCatalogItem, type StoreQuote } from "../lib/api/store";
import { getStoreCatalogImage } from "../lib/catalog-images";
import { setCurrentProperty, clearCurrentProperty } from "../lib/current-property";
import { propertyName } from "./ops-format";
import SuperhostMount from "../components/superhost/SuperhostMount";
import { PaymentBoundary, PaymentBoundaryButton } from "../components/superhost/PaymentBoundary";
import { useAgentSurface } from "../components/agent-surface/context";
import type { AgentAction } from "../components/agent-surface/types";
import "./stay.css";

const PROVIDERS = ["INSTAMART", "ZEPTO", "BLINKIT"];
const GUEST_TICKET_TYPES: Array<{ value: TicketType; label: string }> = [
  { value: "incident", label: "Something needs attention" },
  { value: "restock", label: "Restock an essential" },
  { value: "routine_maintenance", label: "Maintenance request" },
];

const localInput = (date: Date) => {
  const adjusted = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return adjusted.toISOString().slice(0, 16);
};

function StaySkeleton({ variant }: { variant: "card" | "catalog" }) {
  return (
    <div className={variant === "catalog" ? "stay-skeleton stay-skeleton--catalog" : "stay-skeleton stay-skeleton--card"} aria-label="Loading stay data" aria-busy="true">
      {variant === "catalog" ? Array.from({ length: 6 }, (_, index) => <span key={index} />) : <span />}
    </div>
  );
}

function StoreItemCard({ item, onAdd }: { item: StoreCatalogItem; onAdd: () => void }) {
  const photo = getStoreCatalogImage(item.id);
  return (
    <article className="stay-item">
      {photo && <div className="stay-item-visual"><img src={photo.src} alt="" loading="lazy" /><em>{photo.label}</em></div>}
      <div><span className="stay-meta">{item.category} · {item.provider}</span><h3>{item.name}</h3><small>{item.unit}</small></div>
      <footer><Money value={item.price.minor_units} currency={item.price.currency} /><button type="button" onClick={onAdd}>ADD +</button></footer>
    </article>
  );
}

export default function Stay() {
  const propertiesQuery = useQuery({ queryKey: ["stay", "properties"], queryFn: getOpsProperties });
  const [propertyId, setPropertyId] = useState("");
  const [query, setQuery] = useState("");
  const [provider, setProvider] = useState("");
  const [cart, setCart] = useState<Array<{ item: StoreCatalogItem; quantity: number }>>([]);
  const [cartOpen, setCartOpen] = useState(false);
  const [quote, setQuote] = useState<StoreQuote | null>(null);
  const [ticketType, setTicketType] = useState<TicketType>("incident");
  const [reason, setReason] = useState("");
  const [start, setStart] = useState(() => localInput(new Date(Date.now() + 60 * 60_000)));
  const [end, setEnd] = useState(() => localInput(new Date(Date.now() + 4 * 60 * 60_000)));
  const propertySurface = useAgentSurface("stay-property-selector", ["scroll_to", "open_panel"] as AgentAction[], "Choose the current property", () => document.getElementById("stay-property-selector-trigger")?.click());
  const ticketTypeSurface = useAgentSurface("stay-help-ticket-type", ["focus", "set", "scroll_to"] as AgentAction[], "Choose the help request type");
  const reasonSurface = useAgentSurface("stay-help-reason", ["focus", "set", "scroll_to"] as AgentAction[], "Describe what help is needed");
  const startSurface = useAgentSurface("stay-help-window-start", ["focus", "set"] as AgentAction[], "Set the help request start time");
  const endSurface = useAgentSurface("stay-help-window-end", ["focus", "set"] as AgentAction[], "Set the help request end time");
  const submitHelpSurface = useAgentSurface("stay-help-submit", ["focus", "click"] as AgentAction[], "Send the help request");
  const properties = useMemo(() => propertiesQuery.data?.items ?? [], [propertiesQuery.data?.items]);
  const property = properties.find((entry) => entry.id === propertyId);

  useEffect(() => {
    if (!propertiesQuery.isSuccess || properties.length === 0) return;
    if (!propertyId || !properties.some((entry) => entry.id === propertyId)) {
      setPropertyId(properties[0].id);
    }
  }, [properties, propertiesQuery.isSuccess, propertyId]);

  useEffect(() => {
    if (property) setCurrentProperty({ id: property.id, label: propertyName(property) });
    else clearCurrentProperty();
    return clearCurrentProperty;
  }, [property]);
  const catalogQuery = useQuery({
    queryKey: ["stay", "catalog", propertyId, query, provider],
    queryFn: () => getStoreCatalog(propertyId, query, provider),
    enabled: Boolean(propertyId),
  });
  const reservationsQuery = useQuery({ queryKey: ["stay", "reservations", propertyId], queryFn: () => getReservations(propertyId), enabled: Boolean(propertyId) });
  const ticketMutation = useMutation({
    mutationFn: createTicket,
    onSuccess: () => { setReason(""); toast.success("Help request sent to the property team"); },
  });
  const quoteMutation = useMutation({
    mutationFn: () => createStoreQuote(cart.map((line) => ({ catalog_item_id: line.item.id, quantity: line.quantity }))),
    onSuccess: (nextQuote) => setQuote(nextQuote),
  });
  const orderMutation = useMutation({
    mutationFn: () => placeStoreOrder({ tenant_id: import.meta.env.VITE_DEMO_TENANT_ID as string, property_id: propertyId, quote: quote! }),
    onSuccess: (order) => { setCart([]); setQuote(null); setCartOpen(false); toast.success(`Order ${order.order_id} placed`); },
  });

  function submitHelp(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const startDate = new Date(start); const endDate = new Date(end);
    if (!propertyId || !reason.trim()) return toast.error("Choose a property and describe the request");
    if (!Number.isFinite(startDate.getTime()) || !Number.isFinite(endDate.getTime()) || endDate <= startDate) return toast.error("Choose a valid request window");
    ticketMutation.mutate({ property_id: propertyId, type: ticketType, reason: reason.trim(), requested_window: { start: startDate.toISOString(), end: endDate.toISOString() } });
  }

  const reservation = reservationsQuery.data?.items.map((entry) => entry.data).find((entry) => {
    const now = Date.now(); return !["cancelled", "blocked"].includes(entry.status) && new Date(entry.start_at).getTime() <= now && new Date(entry.end_at).getTime() >= now;
  });

  return (
    <main className="stay-shell">
      <header className="stay-header">
        {/* This used to have its own "ACCESS DESK" back-to-login link
            here, in the page's own top-left corner -- exactly where
            GlobalBackButton (a fixed-position, always-present way out,
            added later and covering every route including this one) now
            sits. The two overlapped and rendered as clipped, colliding
            text. GlobalBackButton already does this link's job. */}
        <div><span className="stay-kicker">GUEST PORTAL / LUCKNOW</span><h1>Stay, considered.</h1><p>A useful place for the things that make a stay easier. Select the property you are visiting to begin.</p></div>
        <div className="stay-property"><label ref={propertySurface.ref}><span className="stay-label">CURRENT PROPERTY</span><Select id="stay-property-selector-trigger" value={propertyId} onChange={setPropertyId} placeholder="SELECT PROPERTY" options={[{ value: "", label: "SELECT PROPERTY" }, ...properties.map((entry) => ({ value: entry.id, label: propertyName(entry) }))]} /></label></div>
      </header>
      <SuperhostMount
        propertyId={propertyId || null}
        routeKey="stay"
        purpose={propertyId ? `Stay assistance for property ${propertyId}` : ""}
        emptyMessage="not connected: select a property for stay assistance"
      />
      {propertiesQuery.isError && (
        <div className="stay-error" role="alert">
          <span>Could not load the property list.</span>
          <button type="button" onClick={() => void propertiesQuery.refetch()}>TRY AGAIN</button>
        </div>
      )}
      <div className="stay-grid">
        <section className="stay-section" aria-labelledby="stay-current"><header><h2 id="stay-current">Current stay</h2><span>01</span></header>{!propertyId ? <div className="stay-state">Select a property to check for an active reservation.</div> : reservationsQuery.isLoading ? <StaySkeleton variant="card" /> : reservationsQuery.isError ? <div className="stay-state stay-state--error"><span>We could not reach the reservation record.</span><button type="button" onClick={() => void reservationsQuery.refetch()}>TRY AGAIN</button></div> : reservation ? <div className="stay-stay-card"><strong>Active reservation</strong><span className="stay-meta">{reservation.start_at} → {reservation.end_at}</span><p className="stay-muted">{reservation.guest_summary || "No guest summary supplied."}</p></div> : <div className="stay-state">No active stay found for this property.</div>}</section>
        <section className="stay-section" aria-labelledby="stay-guide"><header><h2 id="stay-guide">House guide</h2><span>02</span></header>{property ? <div className="stay-guide"><article><small>ACCESS</small><p>{property.data.access_method || "No access method has been recorded for this property."}</p></article><article><small>EMERGENCY CONTACTS</small>{property.data.emergency_contacts?.length ? property.data.emergency_contacts.map((contact) => <p key={`${contact.name}-${contact.phone}`}>{contact.name} · {contact.phone}{contact.role ? ` · ${contact.role}` : ""}</p>) : <p>No emergency contacts have been recorded.</p>}</article><article><small>REFERENCE</small><p>For urgent danger, contact local emergency services. This is generic reference guidance, not a personalized property instruction.</p></article></div> : <div className="stay-state">Select a property to see recorded access and emergency details.</div>}</section>
        <section className="stay-section" aria-labelledby="stay-help"><header><h2 id="stay-help">Request help</h2><span>03</span></header><form className="stay-form" onSubmit={submitHelp}><label><span className="stay-label">REQUEST TYPE</span><select ref={ticketTypeSurface.ref} value={ticketType} onChange={(event) => setTicketType(event.currentTarget.value as TicketType)}>{GUEST_TICKET_TYPES.map((entry) => <option key={entry.value} value={entry.value}>{entry.label}</option>)}</select></label><label><span className="stay-label">WHAT DO YOU NEED?</span><textarea ref={reasonSurface.ref} rows={3} maxLength={500} value={reason} onChange={(event) => setReason(event.currentTarget.value)} placeholder="Tell the property team what would help." required /></label><label><span className="stay-label">REQUESTED WINDOW</span><input ref={startSurface.ref} type="datetime-local" value={start} onChange={(event) => setStart(event.currentTarget.value)} required /><input ref={endSurface.ref} type="datetime-local" value={end} onChange={(event) => setEnd(event.currentTarget.value)} required /></label><button ref={submitHelpSurface.ref} type="submit" disabled={!propertyId || ticketMutation.isPending}>{ticketMutation.isPending ? "SENDING…" : "SEND REQUEST →"}</button>{ticketMutation.isSuccess && <p className="stay-form-message">Your request is in the property team’s queue.</p>}</form></section>
        <section className="stay-section stay-section--wide" aria-labelledby="stay-store"><header><h2 id="stay-store">The store</h2><button className="stay-cart-button" type="button" onClick={() => setCartOpen(true)} disabled={cart.length === 0}>CART / {cart.reduce((total, line) => total + line.quantity, 0)}</button><span>04</span></header>{!propertyId ? <div className="stay-state">Select a property to browse Lucknow essentials.</div> : <><div className="stay-store-toolbar"><input className="stay-search" type="search" value={query} onChange={(event) => setQuery(event.currentTarget.value)} placeholder="SEARCH CATALOG" aria-label="Search store catalog" /><div className="stay-providers">{PROVIDERS.map((entry) => <button className="stay-provider" data-active={provider === entry} type="button" key={entry} onClick={() => setProvider(provider === entry ? "" : entry)}>{entry}</button>)}</div></div>{catalogQuery.isLoading ? <StaySkeleton variant="catalog" /> : catalogQuery.isError ? <div className="stay-state stay-state--error"><span>Catalog unavailable.</span><button type="button" onClick={() => void catalogQuery.refetch()}>TRY AGAIN</button></div> : catalogQuery.data?.items.length ? <div className="stay-catalog">{catalogQuery.data.items.map((item) => <StoreItemCard key={item.id} item={item} onAdd={() => setCart((current) => { const found = current.find((line) => line.item.id === item.id); return found ? current.map((line) => line.item.id === item.id ? { ...line, quantity: line.quantity + 1 } : line) : [...current, { item, quantity: 1 }]; })} />)}</div> : <div className="stay-state">No catalog items match this search.</div>}</>}</section>
      </div>
      <Modal open={cartOpen} onClose={() => setCartOpen(false)} title="Your order" label="POPUP 03 / QUICK VIEW"><div className="stay-cart-list">{cart.map((line) => <div className="stay-cart-line" key={line.item.id}><span>{line.item.name}<br /><small>{line.item.provider} · {line.quantity} × <Money value={line.item.price.minor_units} currency={line.item.price.currency} /></small></span><button type="button" aria-label={`Remove ${line.item.name}`} onClick={() => setCart((current) => current.filter((entry) => entry.item.id !== line.item.id))}>×</button></div>)}</div>{quote && <p className="stay-muted">Quote {quote.id} · provider {quote.provider}</p>}<PaymentBoundary><div className="stay-quote">{quote ? <strong><Money value={quote.total.minor_units} currency={quote.total.currency} /></strong> : <span>Get a live quote before confirming.</span>}{quote ? <PaymentBoundaryButton className="stay-button" type="button" disabled={orderMutation.isPending || !import.meta.env.VITE_DEMO_TENANT_ID} onClick={() => orderMutation.mutate()}>{orderMutation.isPending ? "PLACING…" : "CONFIRM ORDER"}</PaymentBoundaryButton> : <button className="stay-button" type="button" disabled={quoteMutation.isPending} onClick={() => quoteMutation.mutate()}>{quoteMutation.isPending ? "QUOTING…" : "GET QUOTE →"}</button>}</div></PaymentBoundary>{!import.meta.env.VITE_DEMO_TENANT_ID && <p className="stay-muted">Ordering is unavailable because the demo tenant is not configured.</p>}</Modal>
    </main>
  );
}
