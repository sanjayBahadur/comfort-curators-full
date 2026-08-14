import { type FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate } from "react-router-dom";
import { toast } from "sonner";
import Money from "../components/money";
import Select from "../components/ui/Select";
import PropertyHandover, { type PropertyHandoverPayload } from "../components/onboarding/PropertyHandover";
import {
  acceptAgreement,
  createOwnerProperty,
  getOnboardingBootstrap,
  getOnboardingWorkspace,
  recordOnboardingEvidence,
  recordOnboardingInspection,
  saveOnboardingContacts,
  saveOnboardingSection,
  startOnboardingCase,
  type OnboardingCaseData,
  type OnboardingWorkspace,
  type NewOwnerPropertyInput,
  type OwnerPropertyData,
} from "../lib/api/onboarding";
import type { Envelope } from "../lib/api/client";
import { getRole, getToken } from "../lib/auth/session";
import "./onboarding.css";

const AUTONOMY_PREFIX = "autonomy_level:";
const STEPS = [
  { title: "Identity & authority", short: "Identity", keys: ["contacts"] },
  { title: "Address & basics", short: "Property", keys: ["portfolio", "goals"] },
  { title: "Documents", short: "Documents", keys: ["documents", "legal_evidence"] },
  { title: "Inspection evidence", short: "Inspection", keys: ["photographs", "safety", "safety_evidence", "inspections"] },
  { title: "Service preferences", short: "Preferences", keys: ["service_preferences", "budgets", "amenities", "furnishing", "remediation", "fit_score_inputs"] },
  { title: "Package", short: "Package", keys: [] },
  { title: "Contract & review", short: "Contract", keys: [] },
] as const;

type StepProps = {
  workspace: OnboardingWorkspace;
  property: Envelope<OwnerPropertyData>;
  busy: boolean;
  save: (work: () => Promise<unknown>, next: number, message: string) => Promise<void>;
};

function OwnerGate({ children }: { children: React.ReactNode }) {
  if (!getToken() || getRole() !== "owner") return <Navigate to="/login" replace />;
  return children;
}

function propertyName(property: Envelope<OwnerPropertyData>) {
  return property.data.service_address.line2 || property.data.service_address.line1;
}

function propertyAddress(property: Envelope<OwnerPropertyData>) {
  const address = property.data.service_address;
  return [address.line1, address.line2, address.city, address.state, address.postal_code, address.country]
    .filter(Boolean)
    .join(", ");
}

function progressMap(workspace: OnboardingWorkspace) {
  return new Map(workspace.progress.progress.map((entry) => [entry.key, entry.complete]));
}

function isStepComplete(index: number, workspace: OnboardingWorkspace) {
  const complete = progressMap(workspace);
  if (index === 5) return workspace.packages.some((entry) => entry.data.status === "active");
  if (index === 6) return workspace.agreements.some((entry) => entry.data.status === "accepted");
  return STEPS[index].keys.every((key) => complete.get(key) === true);
}

function firstIncompleteStep(workspace: OnboardingWorkspace) {
  const index = STEPS.findIndex((_, step) => !isStepComplete(step, workspace));
  return index === -1 ? 6 : index;
}

function autonomyLevel(data: OnboardingCaseData) {
  const marker = data.service_preferences?.automation_limits?.find((entry) => entry.startsWith(AUTONOMY_PREFIX));
  const value = marker?.slice(AUTONOMY_PREFIX.length);
  return value === "advisory" || value === "assisted" || value === "autonomous" ? value : "assisted";
}

function rupeesToMinor(value: FormDataEntryValue | null) {
  const parsed = Number(value ?? 0);
  return Number.isFinite(parsed) ? Math.max(0, Math.round(parsed)) * 100 : 0;
}

function minorToRupees(value?: number) {
  return value ? Math.round(value / 100) : 0;
}

async function metadataHash(value: string) {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function WizardSkeleton() {
  return <div className="onboarding-skeleton" aria-label="Loading onboarding" aria-busy="true"><span /><span /><span /></div>;
}

function StartOnboarding({
  properties,
  cases,
  busy,
  onStart,
  onHandover,
  onResume,
}: {
  properties: Array<Envelope<OwnerPropertyData>>;
  cases: Array<Envelope<OnboardingCaseData>>;
  busy: boolean;
  onStart: (property: Envelope<OwnerPropertyData>) => void;
  onHandover: (payload: PropertyHandoverPayload) => void;
  onResume: (caseId: string) => void;
}) {
  return <PropertyHandover properties={properties} cases={cases} busy={busy} onHandover={onHandover} onStartExisting={onStart} onResume={onResume} />;
}

function IdentityStep({ workspace, property, busy, save }: StepProps) {
  const contact = workspace.onboardingCase.data.contacts?.[0];
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await save(() => saveOnboardingContacts(workspace.onboardingCase.id, [{
      name: String(form.get("name")),
      role: String(form.get("role")),
      phone: String(form.get("phone")),
      email: String(form.get("email")),
    }]), 1, "Identity and authority saved");
  }
  return <form className="onboarding-form" onSubmit={(event) => void submit(event)}>
    <StepHeading index={0} title="Who can speak for this property?" copy="The property authority is already verified by your owner session. Add the person we should contact for decisions." />
    <div className="onboarding-authority"><span>VERIFIED OWNER AUTHORITY</span><strong>{propertyName(property)}</strong><small>Linked to this authenticated owner. The internal authority identifier stays out of the public display.</small></div>
    <Field label="CONTACT NAME"><input name="name" defaultValue={contact?.name ?? ""} autoComplete="name" required /></Field>
     <Field label="DECISION ROLE"><Select name="role" defaultValue={contact?.role ?? "owner"} options={[{ value: "owner", label: "Owner" }, { value: "authorized_representative", label: "Authorized representative" }, { value: "property_manager", label: "Property manager" }]} /></Field>
    <Field label="PHONE"><input name="phone" defaultValue={contact?.phone ?? ""} autoComplete="tel" required /></Field>
    <Field label="EMAIL"><input name="email" type="email" defaultValue={contact?.email ?? ""} autoComplete="email" /></Field>
    <Continue busy={busy} />
  </form>;
}

function PropertyStep({ workspace, property, busy, save }: StepProps) {
  const portfolio = workspace.onboardingCase.data.portfolio;
  const goals = workspace.onboardingCase.data.goals;
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await save(async () => {
      await saveOnboardingSection(workspace.onboardingCase.id, "portfolio", {
        property_name: String(form.get("property_name")), property_type: String(form.get("property_type")),
        purchase_year: Number(form.get("purchase_year")), managed_units: Number(form.get("managed_units")),
        primary_residence: form.get("primary_residence") === "on",
      });
      return saveOnboardingSection(workspace.onboardingCase.id, "goals", {
        primary_goal: String(form.get("primary_goal")), rental_strategy: String(form.get("rental_strategy")),
        occupancy_target: Number(form.get("occupancy_target")),
      });
    }, 2, "Property basics saved");
  }
  return <form className="onboarding-form" onSubmit={(event) => void submit(event)}>
    <StepHeading index={1} title="Tell us what this place is for." copy="The service address comes from the property record. Add only the operating basics we need for onboarding." />
    <div className="onboarding-address"><span>SERVICE ADDRESS / READ ONLY</span><strong>{propertyAddress(property)}</strong><small>{property.data.timezone} · MAXIMUM OCCUPANCY {property.data.maximum_occupancy}</small></div>
    <Field label="PROPERTY NAME"><input name="property_name" defaultValue={portfolio?.property_name ?? propertyName(property)} required /></Field>
     <Field label="PROPERTY TYPE"><Select name="property_type" defaultValue={portfolio?.property_type ?? "apartment"} options={[{ value: "apartment", label: "Apartment" }, { value: "house", label: "House" }, { value: "studio", label: "Studio" }, { value: "villa", label: "Villa" }]} /></Field>
    <Field label="PURCHASE YEAR"><input name="purchase_year" type="number" min="1900" max="2100" defaultValue={portfolio?.purchase_year || 2024} /></Field>
    <Field label="MANAGED UNITS"><input name="managed_units" type="number" min="1" defaultValue={portfolio?.managed_units || 1} required /></Field>
     <Field label="PRIMARY GOAL"><Select name="primary_goal" defaultValue={goals?.primary_goal ?? "reliable_operations"} options={[{ value: "reliable_operations", label: "Reliable operations" }, { value: "guest_readiness", label: "Guest readiness" }, { value: "hands_off_management", label: "Hands-off management" }]} /></Field>
     <Field label="RENTAL STRATEGY"><Select name="rental_strategy" defaultValue={goals?.rental_strategy ?? "short_term"} options={[{ value: "short_term", label: "Short-term stays" }, { value: "mid_term", label: "Mid-term stays" }, { value: "mixed", label: "Mixed stays" }]} /></Field>
    <Field label="OCCUPANCY TARGET / %"><input name="occupancy_target" type="number" min="0" max="100" defaultValue={goals?.occupancy_target || 70} /></Field>
    <label className="onboarding-check"><input name="primary_residence" type="checkbox" defaultChecked={portfolio?.primary_residence ?? false} /><span>This is also my primary residence.</span></label>
    <Continue busy={busy} />
  </form>;
}

function DocumentsStep({ workspace, busy, save }: StepProps) {
  const evidence = workspace.onboardingCase.data.evidence ?? [];
  const legal = evidence.find((entry) => entry.kind === "legal");
  const document = evidence.find((entry) => entry.kind === "document");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const legalRef = String(form.get("legal_ref"));
    const documentRef = String(form.get("document_ref"));
    await save(async () => {
      if (!legal || legal.object_ref !== legalRef) await recordOnboardingEvidence(workspace.onboardingCase.id, { kind: "legal", object_ref: legalRef, content_hash: await metadataHash(`${workspace.onboardingCase.id}:legal:${legalRef}`) });
      if (!document || document.object_ref !== documentRef) return recordOnboardingEvidence(workspace.onboardingCase.id, { kind: "document", object_ref: documentRef, content_hash: await metadataHash(`${workspace.onboardingCase.id}:document:${documentRef}`) });
      return Promise.resolve();
    }, 3, "Document references recorded");
  }
  return <form className="onboarding-form" onSubmit={(event) => void submit(event)}>
    <StepHeading index={2} title="Reference the documents we will review." copy="Phase 6 records metadata only. No file is uploaded and no document is described as verified before a human review." />
    <div className="onboarding-honesty"><strong>METADATA ONLY</strong><span>Use a filename, vault reference, or physical document reference. A new correction creates a new append-only evidence record.</span></div>
    <Field label="OWNERSHIP / AUTHORITY DOCUMENT REFERENCE"><input name="legal_ref" defaultValue={legal?.object_ref ?? ""} placeholder="e.g. registry-deed.pdf" required /></Field>
    <Field label="PROPERTY DOCUMENT REFERENCE"><input name="document_ref" defaultValue={document?.object_ref ?? ""} placeholder="e.g. utility-statement.pdf" required /></Field>
    <Continue busy={busy} />
  </form>;
}

function InspectionStep({ workspace, property, busy, save }: StepProps) {
  const data = workspace.onboardingCase.data;
  const photograph = data.photographs?.[0];
  const inspection = data.inspections?.[0];
  const safetyEvidence = data.evidence?.find((entry) => entry.kind === "safety");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const evidenceRef = String(form.get("evidence_ref"));
    const inspectedBy = String(form.get("inspected_by"));
    const findings = String(form.get("findings"));
    const status = String(form.get("overall_status"));
    await save(async () => {
      await saveOnboardingSection(workspace.onboardingCase.id, "photographs", [{ object_ref: evidenceRef, caption: "Owner-supplied inspection reference", captured_at: new Date().toISOString() }]);
      await saveOnboardingSection(workspace.onboardingCase.id, "safety", {
        smoke_detectors_installed: form.get("smoke") === "on", fire_extinguisher_present: form.get("fire") === "on",
        gas_leak_check_done: form.get("gas") === "on", electrical_safety_certified: form.get("electrical") === "on",
        notes: String(form.get("safety_notes")),
      });
      if (!safetyEvidence || safetyEvidence.object_ref !== evidenceRef) await recordOnboardingEvidence(workspace.onboardingCase.id, { kind: "safety", object_ref: evidenceRef, content_hash: await metadataHash(`${workspace.onboardingCase.id}:safety:${evidenceRef}`) });
      if (!inspection) return recordOnboardingInspection(workspace.onboardingCase.id, { property_id: property.id, inspected_by: inspectedBy, evidence_ref: evidenceRef, evidence_hash: await metadataHash(`${workspace.onboardingCase.id}:inspection:${evidenceRef}`), findings, overall_status: status });
      return Promise.resolve();
    }, 4, "Inspection evidence recorded");
  }
  return <form className="onboarding-form" onSubmit={(event) => void submit(event)}>
    <StepHeading index={3} title="Record what the inspection is based on." copy="This records immutable inspection metadata. It does not upload a photograph or replace an in-person safety review." />
    <Field label="INSPECTED BY"><input name="inspected_by" defaultValue={inspection?.inspected_by ?? ""} required /></Field>
    <Field label="EVIDENCE REFERENCE"><input name="evidence_ref" defaultValue={inspection?.evidence_ref ?? photograph?.object_ref ?? ""} placeholder="e.g. inspection-set-01" required /></Field>
     <Field label="OVERALL STATUS"><Select name="overall_status" defaultValue={inspection?.overall_status ?? "review_required"} options={[{ value: "review_required", label: "Review required" }, { value: "satisfactory", label: "Satisfactory" }, { value: "remediation_required", label: "Remediation required" }]} /></Field>
    <Field label="FINDINGS" wide><textarea name="findings" defaultValue={inspection?.findings ?? ""} rows={4} required /></Field>
    <fieldset className="onboarding-check-grid"><legend>OWNER-REPORTED SAFETY DETAILS</legend><label><input name="smoke" type="checkbox" defaultChecked={data.safety?.smoke_detectors_installed} /> Smoke detectors installed</label><label><input name="fire" type="checkbox" defaultChecked={data.safety?.fire_extinguisher_present} /> Fire extinguisher present</label><label><input name="gas" type="checkbox" defaultChecked={data.safety?.gas_leak_check_done} /> Gas leak check done</label><label><input name="electrical" type="checkbox" defaultChecked={data.safety?.electrical_safety_certified} /> Electrical safety certified</label></fieldset>
    <Field label="SAFETY NOTES" wide><textarea name="safety_notes" defaultValue={data.safety?.notes ?? ""} rows={3} /></Field>
    <Continue busy={busy} />
  </form>;
}

function PreferencesStep({ workspace, busy, save }: StepProps) {
  const data = workspace.onboardingCase.data;
  const preferences = data.service_preferences;
  const budgets = data.budgets;
  const scores = data.fit_score_inputs;
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const autonomy = String(form.get("autonomy"));
    const otherLimits = preferences?.automation_limits?.filter((entry) => !entry.startsWith(AUTONOMY_PREFIX)) ?? [];
    const remediation = String(form.get("remediation")).trim();
    await save(async () => {
      await saveOnboardingSection(workspace.onboardingCase.id, "service_preferences", {
        furnishing_preference: String(form.get("furnishing")), communication_channel: String(form.get("communication")),
        service_language: String(form.get("language")), guest_access_policy: String(form.get("access")),
        approval_threshold_minor_units: rupeesToMinor(form.get("approval_threshold")), currency: "INR",
        automation_limits: [...otherLimits, `${AUTONOMY_PREFIX}${autonomy}`],
      });
      await saveOnboardingSection(workspace.onboardingCase.id, "budgets", {
        monthly_budget_minor_units: rupeesToMinor(form.get("monthly_budget")), setup_budget_minor_units: rupeesToMinor(form.get("setup_budget")),
        renovation_budget_minor_units: rupeesToMinor(form.get("renovation_budget")), currency: "INR", overspend_policy: "owner_approval",
      });
      await saveOnboardingSection(workspace.onboardingCase.id, "amenities", [{ name: String(form.get("amenity")), quantity: Number(form.get("amenity_quantity")) }]);
      await saveOnboardingSection(workspace.onboardingCase.id, "furnishing", { furnishing_level: String(form.get("furnishing")), inventory_count: Number(form.get("inventory_count")), notes: "Owner-reported onboarding estimate" });
      await saveOnboardingSection(workspace.onboardingCase.id, "remediation", { open_items: remediation ? [{ description: remediation, resolved: false }] : [], completed_items: [] });
      return saveOnboardingSection(workspace.onboardingCase.id, "fit_score_inputs", {
        property_score: Number(form.get("property_score")), market_score: Number(form.get("market_score")), operations_score: Number(form.get("operations_score")),
        renovation_score: Number(form.get("renovation_score")), occupancy_score: Number(form.get("occupancy_score")),
      });
    }, 5, "Service preferences and autonomy saved");
  }
  const autonomy = autonomyLevel(data);
  return <form className="onboarding-form" onSubmit={(event) => void submit(event)}>
    <StepHeading index={4} title="How should we bring decisions to you?" copy="Set communication, limits, and your autonomy preference. These are onboarding inputs, not promises of automated behavior." />
    <fieldset className="onboarding-autonomy"><legend>AUTONOMY LEVEL</legend>{[
      ["advisory", "Advisory", "We prepare options for your review."], ["assisted", "Assisted", "We coordinate routine decisions with you."], ["autonomous", "Autonomous", "Your preference for fewer routine check-ins."],
    ].map(([value, label, description]) => <label key={value}><input type="radio" name="autonomy" value={value} defaultChecked={autonomy === value} /><strong>{label}</strong><span>{description}</span></label>)}<p>Recorded preference only. It does not change how work is approved or carried out yet.</p></fieldset>
     <Field label="COMMUNICATION"><Select name="communication" defaultValue={preferences?.communication_channel ?? "whatsapp"} options={[{ value: "whatsapp", label: "WhatsApp" }, { value: "email", label: "Email" }, { value: "phone", label: "Phone" }]} /></Field>
     <Field label="SERVICE LANGUAGE"><Select name="language" defaultValue={preferences?.service_language ?? "en-IN"} options={[{ value: "en-IN", label: "English" }, { value: "hi-IN", label: "Hindi" }]} /></Field>
     <Field label="GUEST ACCESS"><Select name="access" defaultValue={preferences?.guest_access_policy ?? "owner_provides"} options={[{ value: "owner_provides", label: "Owner provides access" }, { value: "managed_key", label: "Managed key" }, { value: "smart_lock", label: "Smart lock reference" }]} /></Field>
    <Field label="APPROVAL THRESHOLD / ₹"><input name="approval_threshold" type="number" min="0" defaultValue={minorToRupees(preferences?.approval_threshold_minor_units) || 5000} /></Field>
    <Field label="MONTHLY OPERATING BUDGET / ₹"><input name="monthly_budget" type="number" min="0" defaultValue={minorToRupees(budgets?.monthly_budget_minor_units) || 40000} /></Field>
    <Field label="SETUP BUDGET / ₹"><input name="setup_budget" type="number" min="0" defaultValue={minorToRupees(budgets?.setup_budget_minor_units) || 75000} /></Field>
    <Field label="RENOVATION BUDGET / ₹"><input name="renovation_budget" type="number" min="0" defaultValue={minorToRupees(budgets?.renovation_budget_minor_units)} /></Field>
     <Field label="FURNISHING"><Select name="furnishing" defaultValue={data.furnishing?.furnishing_level ?? preferences?.furnishing_preference ?? "fully_furnished"} options={[{ value: "fully_furnished", label: "Fully furnished" }, { value: "part_furnished", label: "Part furnished" }, { value: "unfurnished", label: "Unfurnished" }]} /></Field>
    <Field label="ONE KEY AMENITY"><input name="amenity" defaultValue={data.amenities?.[0]?.name ?? "Wi-Fi"} required /></Field>
    <Field label="AMENITY QUANTITY"><input name="amenity_quantity" type="number" min="1" defaultValue={data.amenities?.[0]?.quantity || 1} /></Field>
    <Field label="INVENTORY ITEM COUNT"><input name="inventory_count" type="number" min="0" defaultValue={data.furnishing?.inventory_count ?? 0} /></Field>
    <Field label="OPEN REMEDIATION" wide><textarea name="remediation" defaultValue={data.remediation?.open_items?.[0]?.description ?? ""} placeholder="Leave blank when nothing is known." rows={3} /></Field>
    <fieldset className="onboarding-score-grid"><legend>OWNER ESTIMATE / 0–100</legend>{["property", "market", "operations", "renovation", "occupancy"].map((key) => <label key={key}><span>{key.toUpperCase()}</span><input name={`${key}_score`} type="number" min="0" max="100" defaultValue={scores?.[`${key}_score` as keyof typeof scores] ?? 70} /></label>)}</fieldset>
    <Continue busy={busy} />
  </form>;
}

function PackageStep({ workspace, property, busy, save }: StepProps) {
  const active = workspace.packages.filter((entry) => entry.data.status === "active").sort((a, b) => b.data.version_number - a.data.version_number)[0];
  return <section className="onboarding-form">
    <StepHeading index={5} title="Choose the package that operates this property." copy="Package state and pricing remain in the real package aggregate. This wizard does not duplicate or recalculate them." />
    {active ? <div className="onboarding-package"><span>ACTIVE PACKAGE / V{active.data.version_number}</span><Money value={active.data.monthly_cost_minor_units} currency={active.data.currency} /><small>PER MONTH · SERVER PRICED</small><Link to={`/properties/${property.id}/package`}>REVIEW OR CHANGE PACKAGE →</Link></div> : <div className="onboarding-package onboarding-package--empty"><strong>No active package yet.</strong><p>Build and activate one before continuing. Returning here reads the server state.</p><Link to={`/properties/${property.id}/package`}>BUILD PACKAGE →</Link></div>}
    <div className="onboarding-actions"><button type="button" onClick={() => void save(() => Promise.resolve(), 6, "Active package confirmed")} disabled={!active || busy}>{busy ? "CHECKING…" : "CONFIRM ACTIVE PACKAGE →"}</button></div>
  </section>;
}

function ContractStep({ workspace, property, busy, save }: StepProps) {
  const accepted = workspace.agreements.find((entry) => entry.data.status === "accepted");
  const draft = workspace.agreements.find((entry) => entry.data.status === "draft");
  const autonomy = autonomyLevel(workspace.onboardingCase.data);
  const completed = workspace.progress.progress.filter((entry) => entry.complete).length;
  const [confirmed, setConfirmed] = useState(false);
  const draftTerms = draft?.data.versions.find((version) => version.version_number === draft.data.current_version)?.terms;
  return <section className="onboarding-form">
    <StepHeading index={6} title="Review what is recorded—and what is not." copy="Finishing this intake does not activate the property, accept unreviewed legal terms, or turn your autonomy preference into automation." />
    <dl className="onboarding-review"><div><dt>PROPERTY</dt><dd>{propertyName(property)}</dd></div><div><dt>SERVER CHECKLIST</dt><dd>{completed} / {workspace.progress.progress.length} recorded</dd></div><div><dt>AUTONOMY</dt><dd>{autonomy}</dd></div><div><dt>PACKAGE</dt><dd>{workspace.packages.some((entry) => entry.data.status === "active") ? "Active package linked by property" : "No active package"}</dd></div><div><dt>CONTRACT</dt><dd>{accepted ? `Accepted V${accepted.data.current_version}` : draft ? `Draft V${draft.data.current_version} awaits formal review` : "Not issued"}</dd></div></dl>
    {accepted ? <div className="onboarding-contract-state"><strong>ACCEPTED AGREEMENT ON FILE</strong><span>{accepted.data.acceptance ? `Accepted ${new Date(accepted.data.acceptance.accepted_at).toLocaleDateString("en-IN")}` : "Acceptance recorded by the contract service."}</span></div> : draft ? <><div className="onboarding-honesty"><strong>AGREEMENT V{draft.data.current_version} / REVIEW REQUIRED</strong><span>The exact server-held terms are shown below. Acceptance is recorded only after your explicit confirmation.</span></div><pre className="onboarding-contract-terms">{JSON.stringify(draftTerms ?? {}, null, 2)}</pre></> : <div className="onboarding-honesty"><strong>NO ACCEPTANCE RECORDED</strong><span>No service agreement has been issued. We will not fabricate terms or mark a contract accepted.</span></div>}
    <label className="onboarding-final-check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /><span>{draft ? `I have reviewed and accept the exact terms in agreement version ${draft.data.current_version}.` : "I confirm the onboarding information above is accurate. This is an intake acknowledgement, not contract acceptance."}</span></label>
    <div className="onboarding-actions"><button type="button" disabled={!confirmed || busy} onClick={() => void save(() => draft ? acceptAgreement(draft.id) : Promise.resolve(), 6, draft ? "Agreement accepted" : "Onboarding intake review complete")}>{busy ? "FINISHING…" : draft ? "ACCEPT AGREEMENT →" : "FINISH INTAKE →"}</button></div>
  </section>;
}

function StepHeading({ index, title, copy }: { index: number; title: string; copy: string }) {
  return <header className="onboarding-step-heading"><span>{String(index + 1).padStart(2, "0")} / 07</span><h2 id="onboarding-step-title" tabIndex={-1}>{title}</h2><p>{copy}</p></header>;
}

function Field({ label, wide = false, children }: { label: string; wide?: boolean; children: React.ReactNode }) {
  return <label className="onboarding-field" data-wide={wide}><span>{label}</span>{children}</label>;
}

function Continue({ busy }: { busy: boolean }) {
  return <div className="onboarding-actions"><button type="submit" disabled={busy}>{busy ? "SAVING…" : "SAVE & CONTINUE →"}</button></div>;
}

export default function Onboarding() {
  const queryClient = useQueryClient();
  const bootstrapQuery = useQuery({ queryKey: ["owner", "onboarding", "bootstrap"], queryFn: getOnboardingBootstrap });
  const [caseId, setCaseId] = useState<string | null>(null);
  const [allowAutoResume, setAllowAutoResume] = useState(false);
  const [activeStep, setActiveStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [savedNote, setSavedNote] = useState("Server progress will appear here.");
  const initializedCase = useRef<string | null>(null);
  const workspaceQuery = useQuery({ queryKey: ["owner", "onboarding", caseId], queryFn: () => getOnboardingWorkspace(caseId!), enabled: Boolean(caseId) });
  const workspace = workspaceQuery.data;
  const property = useMemo(() => bootstrapQuery.data?.properties.find((entry) => entry.id === workspace?.onboardingCase.data.property_id), [bootstrapQuery.data?.properties, workspace?.onboardingCase.data.property_id]);

  useEffect(() => {
    if (caseId || !allowAutoResume || !bootstrapQuery.data) return;
    const open = bootstrapQuery.data.cases.find((entry) => entry.data.status !== "activated");
    if (open) setCaseId(open.id);
  }, [allowAutoResume, bootstrapQuery.data, caseId]);

  useEffect(() => {
    if (!workspace || initializedCase.current === workspace.onboardingCase.id) return;
    initializedCase.current = workspace.onboardingCase.id;
    setActiveStep(firstIncompleteStep(workspace));
    setSavedNote(`Restored from server · ${workspace.progress.progress.filter((entry) => entry.complete).length} of ${workspace.progress.progress.length} checks recorded.`);
  }, [workspace]);

  async function start(property: Envelope<OwnerPropertyData>) {
    setBusy(true);
    try {
      const created = await startOnboardingCase(property);
      await queryClient.invalidateQueries({ queryKey: ["owner", "onboarding", "bootstrap"] });
      initializedCase.current = null;
      setAllowAutoResume(true);
      setCaseId(created.id);
      setActiveStep(0);
      toast.success("Onboarding case started");
    } catch {
      // The shared API client already surfaces the backend message.
    } finally {
      setBusy(false);
    }
  }

  async function createAndStart(
    input: Omit<NewOwnerPropertyInput, "owner_authority_id" | "timezone" | "access_method" | "status">,
    handover?: Omit<PropertyHandoverPayload, "property">,
  ) {
    if (busy) return;
    setBusy(true);
    try {
      const normalize = (value: string) => value.trim().toLocaleLowerCase("en-IN");
      const existing = bootstrapQuery.data?.properties.find((property) =>
        normalize(property.data.service_address.line1) === normalize(input.service_address.line1)
        && normalize(property.data.service_address.city) === normalize(input.service_address.city)
        && normalize(property.data.service_address.postal_code) === normalize(input.service_address.postal_code),
      );
      const property = existing ?? await createOwnerProperty({
        ...input,
        owner_authority_id: crypto.randomUUID(),
        timezone: "Asia/Kolkata",
        access_method: "owner_coordinates",
        status: "lead",
      });
      const created = await startOnboardingCase(property);
      if (handover) {
        await Promise.all([
          saveOnboardingSection(created.id, "portfolio", {
            property_name: handover.listingTitle,
            property_type: handover.propertyType,
            purchase_year: 2024,
            managed_units: 1,
            primary_residence: false,
          }),
          saveOnboardingSection(created.id, "goals", {
            primary_goal: "reliable_operations",
            rental_strategy: "short_term",
            occupancy_target: 70,
          }),
          ...handover.documentRefs.map(async (documentRef) => recordOnboardingEvidence(created.id, {
            kind: "document",
            object_ref: `airbnb-export/${documentRef}`,
            content_hash: await metadataHash(`${created.id}:airbnb-export:${documentRef}`),
          })),
        ]);
      }
      await queryClient.invalidateQueries({ queryKey: ["owner", "onboarding", "bootstrap"] });
      initializedCase.current = null;
      setAllowAutoResume(true);
      setCaseId(created.id);
      setActiveStep(0);
      toast.success(handover ? "Property record created; onboarding started" : existing ? "Existing property matched; onboarding case started" : "Lead property and onboarding case created");
    } catch {
      // The shared API client already surfaces the backend message.
    } finally {
      setBusy(false);
    }
  }

  async function save(work: () => Promise<unknown>, next: number, message: string) {
    if (!caseId || busy) return;
    setBusy(true);
    try {
      await work();
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["owner", "onboarding", caseId] }),
        queryClient.invalidateQueries({ queryKey: ["owner", "onboarding", "bootstrap"] }),
      ]);
      setActiveStep(next);
      setSavedNote(`${message}. Restored from backend on reload.`);
      toast.success(message);
      requestAnimationFrame(() => document.getElementById("onboarding-step-title")?.focus());
    } catch {
      setSavedNote("Save did not complete. Your last server-confirmed progress is unchanged.");
    } finally {
      setBusy(false);
    }
  }

  const completed = workspace?.progress.progress.filter((entry) => entry.complete).length ?? 0;
  const total = workspace?.progress.progress.length ?? 15;
  const currentLimit = workspace ? firstIncompleteStep(workspace) : 0;
  const Step = workspace && property ? [IdentityStep, PropertyStep, DocumentsStep, InspectionStep, PreferencesStep, PackageStep, ContractStep][activeStep] : null;

  return <OwnerGate><main className="onboarding-shell registration-frame">
    <header className="onboarding-header"><Link to="/dashboard">COMFORT CURATORS / OWNER</Link><span>02 / ONBOARDING</span><nav aria-label="Owner navigation"><Link to="/dashboard">DASHBOARD</Link><Link to="/onboarding" aria-current="page">ONBOARD</Link><Link to="/login">ACCESS DESK</Link></nav></header>
    {bootstrapQuery.isLoading ? <WizardSkeleton /> : bootstrapQuery.isError || !bootstrapQuery.data ? <section className="onboarding-error" role="alert"><p>ONBOARDING UNAVAILABLE</p><h1>We could not read your saved cases.</h1><button type="button" onClick={() => void bootstrapQuery.refetch()}>TRY AGAIN →</button></section> : !caseId ? <StartOnboarding properties={bootstrapQuery.data.properties} cases={bootstrapQuery.data.cases} busy={busy} onStart={(selected) => void start(selected)} onHandover={(input) => void createAndStart(input.property, input)} onResume={(selectedCaseId) => { setAllowAutoResume(true); setCaseId(selectedCaseId); }} /> : workspaceQuery.isError ? <section className="onboarding-error" role="alert"><p>CASE UNAVAILABLE</p><h1>Saved progress could not be restored.</h1><button type="button" onClick={() => void workspaceQuery.refetch()}>RETRY THIS CASE →</button></section> : workspaceQuery.isLoading || !workspace || !property || !Step ? <WizardSkeleton /> : <div className="onboarding-workspace">
      <aside className="onboarding-rail" aria-label="Onboarding steps"><p>CASE / {workspace.onboardingCase.data.status.replaceAll("_", " ")}</p><ol>{STEPS.map((step, index) => { const done = isStepComplete(index, workspace); const allowed = done || index <= currentLimit || index === activeStep; return <li key={step.title} data-active={index === activeStep} data-complete={done}><button type="button" disabled={!allowed || busy} aria-current={index === activeStep ? "step" : undefined} onClick={() => setActiveStep(index)}><b>{String(index + 1).padStart(2, "0")}</b><span>{step.short}</span><small>{done ? "RECORDED" : index === activeStep ? "CURRENT" : "PENDING"}</small></button></li>; })}</ol><button className="onboarding-new-case" type="button" onClick={() => { setAllowAutoResume(false); setCaseId(null); initializedCase.current = null; }}>CHOOSE ANOTHER CASE</button></aside>
      <section className="onboarding-stage" aria-live="polite"><Step workspace={workspace} property={property} busy={busy} save={save} /></section>
      <aside className="onboarding-summary"><span>SERVER PROGRESS</span><div className="onboarding-progress" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={completed} aria-valuetext={`${completed} of ${total} backend checks recorded`}><i style={{ width: `${(completed / total) * 100}%` }} /></div><strong>{completed} / {total}</strong><p>{savedNote}</p><dl><div><dt>PROPERTY</dt><dd>{propertyName(property)}</dd></div><div><dt>CASE STATUS</dt><dd>{workspace.onboardingCase.data.status.replaceAll("_", " ")}</dd></div><div><dt>AUTONOMY</dt><dd>{autonomyLevel(workspace.onboardingCase.data)}</dd></div></dl><small>Autonomy is recorded as an onboarding preference only. It is not enforced.</small></aside>
    </div>}
  </main></OwnerGate>;
}
