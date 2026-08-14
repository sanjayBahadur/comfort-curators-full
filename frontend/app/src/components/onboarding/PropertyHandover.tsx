import { useMemo, useState, type FormEvent } from "react";
import propertyPhoto from "../../assets/properties/gomti-riverside-2bhk.webp";
import type { Envelope } from "../../lib/api/client";
import type { NewOwnerPropertyInput, OnboardingCaseData, OwnerPropertyData } from "../../lib/api/onboarding";
import "./PropertyHandover.css";

export type PropertyHandoverPayload = {
  property: Omit<NewOwnerPropertyInput, "owner_authority_id" | "timezone" | "access_method" | "status">;
  listingTitle: string;
  propertyType: string;
  documentRefs: string[];
};

type Chapter = 0 | 1 | 2 | 3 | 4;

const CHAPTERS = ["Listing", "Details", "Authority", "Documents", "Handover"];
const DEMO_LINK = "https://www.airbnb.com/rooms/cc-demo-noida-137";

function displayName(property: Envelope<OwnerPropertyData>) {
  return property.data.service_address.line2 || property.data.service_address.line1;
}

export default function PropertyHandover({
  properties,
  cases,
  busy,
  onHandover,
  onStartExisting,
  onResume,
}: {
  properties: Array<Envelope<OwnerPropertyData>>;
  cases: Array<Envelope<OnboardingCaseData>>;
  busy: boolean;
  onHandover: (payload: PropertyHandoverPayload) => void;
  onStartExisting: (property: Envelope<OwnerPropertyData>) => void;
  onResume: (caseId: string) => void;
}) {
  const [chapter, setChapter] = useState<Chapter>(0);
  const [furthest, setFurthest] = useState<Chapter>(0);
  const [listingUrl, setListingUrl] = useState(DEMO_LINK);
  const [listingTitle, setListingTitle] = useState("Noida Skyline Residence");
  const [propertyType, setPropertyType] = useState("apartment");
  const [line1, setLine1] = useState("Tower 18, Sector 137");
  const [line2, setLine2] = useState("Noida Expressway residence");
  const [city, setCity] = useState("Noida");
  const [state, setState] = useState("Uttar Pradesh");
  const [postalCode, setPostalCode] = useState("201305");
  const [occupancy, setOccupancy] = useState(4);
  const [authorized, setAuthorized] = useState(false);
  const [documentRefs, setDocumentRefs] = useState<string[]>([]);
  const [draggingFiles, setDraggingFiles] = useState(false);
  const [selectedExistingId, setSelectedExistingId] = useState(properties[0]?.id ?? "");
  const resumable = cases.filter((entry) => entry.data.status !== "activated");
  const projection = useMemo(() => {
    const midpoint = 3450 + Math.max(1, occupancy) * 260;
    return { midpoint, low: Math.round(midpoint * .82 / 50) * 50, high: Math.round(midpoint * 1.27 / 50) * 50 };
  }, [occupancy]);

  function go(next: Chapter) {
    setChapter(next);
    setFurthest((current) => Math.max(current, next) as Chapter);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!authorized) return;
    onHandover({
      listingTitle,
      propertyType,
      documentRefs,
      property: {
        service_address: { line1, line2, city, state, postal_code: postalCode, country: "IN" },
        maximum_occupancy: occupancy,
      },
    });
  }

  return (
    <section className="handover-page" aria-labelledby="handover-title">
      <aside className="handover-index" aria-label="Property handover chapters">
        <p>PROPERTY DOSSIER</p>
        <ol>{CHAPTERS.map((label, index) => <li key={label} data-active={chapter === index} data-complete={index < chapter || index < furthest}><button type="button" disabled={index > furthest} onClick={() => setChapter(index as Chapter)}><b>{String(index + 1).padStart(2, "0")}</b><span>{label}</span><i aria-hidden="true" /></button></li>)}</ol>
      </aside>

      <form className="handover-sheet" onSubmit={submit}>
        <div className="handover-chapter" key={chapter}>
          {chapter === 0 && <section className="handover-opening">
            <div className="handover-kicker"><span>01 / PROPERTY HANDOVER</span><i aria-hidden="true" /></div>
            <h1 id="handover-title">Bring us the home <em>you already host.</em></h1>
            <p>Share the public listing you want Comfort Curators to prepare for onboarding. No Airbnb login or account access is required.</p>
            <label className="handover-link"><span>PUBLIC AIRBNB LISTING</span><div><input type="url" value={listingUrl} onChange={(event) => setListingUrl(event.target.value)} required /><button type="button" disabled={!listingUrl.trim()} onClick={() => go(1)}>FIND THE PROPERTY →</button></div><small>DEMO LISTING LOOKUP / THE LINK IS NOT FETCHED FROM AIRBNB IN THIS BUILD</small></label>
            {(resumable.length > 0 || properties.length > 0) && <div className="handover-existing"><span>OR CONTINUE AN OPERATING RECORD</span>{resumable.slice(0, 2).map((entry) => { const property = properties.find((candidate) => candidate.id === entry.data.property_id); return property ? <button key={entry.id} type="button" onClick={() => onResume(entry.id)}><strong>{displayName(property)}</strong><small>{entry.data.status.replaceAll("_", " ")} · RESUME →</small></button> : null; })}{resumable.length === 0 && properties.length > 0 && <div><select value={selectedExistingId} onChange={(event) => setSelectedExistingId(event.target.value)}>{properties.map((property) => <option key={property.id} value={property.id}>{displayName(property)} · {property.data.service_address.city}</option>)}</select><button type="button" onClick={() => { const selected = properties.find((property) => property.id === selectedExistingId); if (selected) onStartExisting(selected); }}>START EXISTING PROPERTY →</button></div>}</div>}
          </section>}

          {chapter === 1 && <section className="handover-found">
            <figure><img src={propertyPhoto} alt="Exterior of the demo property" /><figcaption>DEMO LISTING PHOTOGRAPH / OWNER REVIEW REQUIRED</figcaption><i aria-hidden="true" /></figure>
            <div className="handover-found-copy"><span>LISTING FOUND / DEMO SOURCE</span><h2>{listingTitle}</h2><p>{city}, {state}</p><dl><div><dt>HOME</dt><dd>Entire {propertyType}</dd></div><div><dt>CAPACITY</dt><dd>{occupancy} guests</dd></div><div><dt>AMENITIES</dt><dd>12 listed</dd></div></dl><p className="handover-running-list">WI-FI / LIFT / AIR CONDITIONING / KITCHEN / PARKING</p><div className="handover-buttons"><button type="button" onClick={() => go(2)}>THIS IS MY PROPERTY →</button><button type="button" onClick={() => document.getElementById("handover-title-input")?.focus()}>CORRECT THE DETAILS</button></div></div>
            <div className="handover-specification">
              <label><span>PROPERTY NAME</span><input id="handover-title-input" value={listingTitle} onChange={(event) => setListingTitle(event.target.value)} /></label>
              <label><span>PROPERTY TYPE</span><select value={propertyType} onChange={(event) => setPropertyType(event.target.value)}><option value="apartment">Apartment</option><option value="house">House</option><option value="studio">Studio</option><option value="villa">Villa</option></select></label>
              <label><span>MAXIMUM GUESTS</span><input type="number" min="1" max="20" value={occupancy} onChange={(event) => setOccupancy(Number(event.target.value))} /></label>
              <label><span>ADDRESS LINE 1</span><input value={line1} onChange={(event) => setLine1(event.target.value)} /></label>
              <label><span>ADDRESS LINE 2</span><input value={line2} onChange={(event) => setLine2(event.target.value)} /></label>
              <label><span>CITY</span><input value={city} onChange={(event) => setCity(event.target.value)} /></label>
              <label><span>STATE</span><input value={state} onChange={(event) => setState(event.target.value)} /></label>
              <label><span>POSTAL CODE</span><input value={postalCode} onChange={(event) => setPostalCode(event.target.value)} /></label>
            </div>
            <aside className="handover-outlook"><header><span>DEMO MARKET OUTLOOK</span><small>NOT LIVE AIRBNB DATA</small></header><div><strong>₹{projection.midpoint.toLocaleString("en-IN")}</strong><p>illustrative nightly midpoint</p></div><dl><div><dt>ILLUSTRATIVE RANGE</dt><dd>₹{projection.low.toLocaleString("en-IN")}—₹{projection.high.toLocaleString("en-IN")}</dd></div><div><dt>OCCUPANCY RANGE</dt><dd>64–72%</dd></div><div><dt>WEEKEND SIGNAL</dt><dd>+18%</dd></div><div><dt>LEAD TIME</dt><dd>12 days</dd></div></dl><p>Illustrative planning note only—not a valuation, market feed, or promised rate.</p></aside>
          </section>}

          {chapter === 2 && <section className="handover-authority">
            <header><span>03 / AUTHORITY</span><h2>You decide what <em>crosses the threshold.</em></h2></header>
            <div className="handover-authority-columns"><section><h3>Comfort Curators may use</h3><ul><li>Public listing information</li><li>Property photographs</li><li>Capacity and amenities</li><li>Documents you explicitly provide</li></ul></section><section><h3>Comfort Curators will not receive</h3><ul><li>Airbnb password or credentials</li><li>Messages or payment information</li><li>Control of the Airbnb account</li><li>Permission to publish changes</li></ul></section></div>
            <label className="handover-signature"><input type="checkbox" checked={authorized} onChange={(event) => setAuthorized(event.target.checked)} /><span>I authorize the reviewed public listing information to be used for this property’s onboarding record.</span></label>
            <div className="handover-buttons"><button type="button" disabled={!authorized} onClick={() => go(3)}>AUTHORIZE + CONTINUE →</button><button type="button" onClick={() => go(1)}>← REVIEW PROPERTY</button></div>
          </section>}

          {chapter === 3 && <section className="handover-documents">
            <header><span>04 / DOCUMENTS</span><h2>Build the evidence <em>trail.</em></h2><p>Add references now, or continue and provide them during the normal onboarding review.</p></header>
            <label className="handover-file" data-dragging={draggingFiles} onDragEnter={() => setDraggingFiles(true)} onDragLeave={() => setDraggingFiles(false)} onDrop={() => setDraggingFiles(false)}>
              <input type="file" multiple accept=".pdf,.csv,.json,.zip" onChange={(event) => setDocumentRefs(Array.from(event.target.files ?? []).map((file) => file.name))} />
              <span className="handover-file-illustration" aria-hidden="true"><i /><b>PDF</b></span>
              <span className="handover-file-copy"><small>AIRBNB EXPORT / OPTIONAL</small><strong>{draggingFiles ? "Release to add the documents." : "Drop your files into the dossier."}</strong><em>or <u>browse files</u> from your device</em></span>
              <span className="handover-file-meta"><b>{documentRefs.length ? `${documentRefs.length} FILE${documentRefs.length === 1 ? "" : "S"} READY` : "PDF / CSV / JSON / ZIP"}</b><small>Filenames become references. File contents are not uploaded in this build.</small></span>
            </label>
            <div className="handover-document-ledger"><section><h3>Carried into the handover</h3>{documentRefs.length ? <ul>{documentRefs.map((name) => <li key={name}><strong>{name}</strong><span>REFERENCE READY / FILE NOT UPLOADED</span></li>)}</ul> : <p>No export references selected. You may continue safely.</p>}</section><section><h3>Needed before activation</h3><ul><li><strong>Ownership or authority evidence</strong><span>NOW OR LATER</span></li><li><strong>Address evidence</strong><span>ON REVIEW</span></li><li><strong>Safety documentation</strong><span>BEFORE ACTIVATION</span></li><li><strong>Inspection photographs</strong><span>ON INSPECTION</span></li></ul></section></div>
            <div className="handover-buttons"><button type="button" onClick={() => go(4)}>REVIEW THE HANDOVER →</button><button type="button" onClick={() => go(2)}>← AUTHORITY</button></div>
          </section>}

          {chapter === 4 && <section className="handover-final">
            <div className="handover-kicker"><span>05 / HANDOVER</span><i aria-hidden="true" /></div><h2>The listing becomes <em>an operating record.</em></h2>
            <dl><div><dt>PROPERTY</dt><dd>{listingTitle}</dd></div><div><dt>AUTHORITY</dt><dd>{authorized ? "Owner permission ready" : "Not authorized"}</dd></div><div><dt>IMPORTED</dt><dd>Listing details · amenities · {documentRefs.length} document reference{documentRefs.length === 1 ? "" : "s"}</dd></div><div><dt>STILL NEEDED</dt><dd>Ownership · safety · inspection</dd></div><div><dt>NEXT</dt><dd>Identity and authority review</dd></div></dl>
            <p>The final action creates a real lead property and server-backed onboarding case. It does not connect or modify an Airbnb account.</p>
            <div className="handover-buttons"><button type="submit" disabled={!authorized || busy}>{busy ? "CREATING THE RECORD…" : "CREATE THE PROPERTY RECORD →"}</button><button type="button" onClick={() => go(3)}>← DOCUMENTS</button></div>
          </section>}
        </div>
      </form>
    </section>
  );
}
