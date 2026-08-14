import { useEffect, useRef, useState } from "react";
import { createTimeline, stagger } from "animejs";
import type { Timeline } from "animejs";
import { Link, useNavigate } from "react-router-dom";
import Terminal, { type TerminalLine } from "../components/superhost/Terminal";
import superhostPhoto from "../assets/expansion/superhost-stage.webp";
import crewPhoto from "../assets/expansion/crew-stage.webp";
import "./expansion.css";

const slideLabels = [
  "Thesis",
  "Small economy",
  "Evidence",
  "SuperhostOS",
  "Authority",
  "Capability",
  "Matching",
  "Three companies",
  "Training ground",
  "Compounding",
  "Build outward",
];

const propertySequence = ["Checkout", "Inspect", "Clean", "Repair", "Restock", "Verify", "Ready"];
const operatingLoop = ["Understand", "Assign", "Guide", "Verify", "Learn"];
const compoundingLoop = [
  ["Operate", "Coordinate real stays and capture the mess."],
  ["Learn", "Turn verified work into operating memory."],
  ["Deliver", "Route approved intent to capable people."],
  ["Compound", "Earn more demand through better execution."],
];

const incidentLines: TerminalLine[] = [
  { id: "guest", kind: "operator", text: "guest: the air conditioner is not cooling." },
  { id: "context", kind: "agent", text: "property context loaded · unit 204 · split AC · last service 41 days ago" },
  { id: "match", kind: "agent", text: "qualified capability found · available in zone · procedure attached" },
  { id: "work", kind: "system", text: "WORK ORDER TRACKED · COMPLETION EVIDENCE REQUIRED" },
  { id: "verify", kind: "agent", text: "cooling restored · evidence verified · operating memory updated" },
];

const sources = [
  {
    label: "Government of India / Ministry of Tourism / PIB",
    note: "84.63 million direct and indirect tourism jobs in India during 2023–24.",
    href: "https://www.pib.gov.in/PressReleasePage.aspx?PRID=2240657&lang=1&reg=1",
  },
  {
    label: "International Labour Organization / NITI Aayog estimate",
    note: "India's gig workforce: 7.7 million in 2020, projected to 23.5 million by 2029–30.",
    href: "https://www.ilo.org/publications/expansion-gig-and-platform-economy-india-opportunities-employer-and",
  },
  {
    label: "World Economic Forum / Future of Jobs Report 2025",
    note: "Employers expect 39% of workers' core skills to change by 2030.",
    href: "https://www.weforum.org/publications/the-future-of-jobs-report-2025/in-full/3-skills-outlook/",
  },
];

const reducedMotionPreferred = () =>
  typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

function ChapterMeta({ number, label }: { number: string; label: string }) {
  return (
    <div className="story-meta" data-animate="reveal">
      <strong>{number}</strong>
      <span>{label}</span>
    </div>
  );
}

function DocumentaryPhoto({ src, alt, caption }: { src: string; alt: string; caption: string }) {
  return (
    <figure className="story-photo" data-animate="reveal">
      <img src={src} alt={alt} loading="lazy" />
      <figcaption>{caption}</figcaption>
    </figure>
  );
}

function ArrowSequence({ items, ariaLabel }: { items: string[]; ariaLabel: string }) {
  return (
    <ol className="story-sequence" aria-label={ariaLabel}>
      {items.map((item, index) => (
        <li key={item} data-animate="reveal">
          <span>{String(index + 1).padStart(2, "0")}</span>
          <strong>{item}</strong>
        </li>
      ))}
    </ol>
  );
}

function CompanyFlywheel() {
  return (
    <figure className="company-flywheel" data-animate="reveal" aria-labelledby="company-flywheel-caption">
      <figcaption id="company-flywheel-caption">THREE COMPANIES / ONE CLOSED OPERATING LOOP</figcaption>
      <div className="company-flywheel-canvas">
        <article className="company-card company-card-comfort">
          <span>01 / BEACHHEAD</span>
          <strong>Comfort Curators</strong>
          <p>Runs the operation, earns revenue, and captures real turnovers, inspections, restocks, maintenance, and owner decisions.</p>
        </article>
        <div className="company-link company-link-context"><span>CONTEXT + REVENUE</span><i aria-hidden="true" /></div>
        <article className="company-card company-card-system">
          <span>02 / DEEP-TECH CORE</span>
          <strong>SuperhostOS</strong>
          <p>Turns context into policy-checked proposals, procedure, coordination, and operating memory.</p>
        </article>
        <div className="company-link-pair">
          <div className="company-link company-link-forward"><span>APPROVED WORK + GUIDANCE</span><i aria-hidden="true" /></div>
          <div className="company-link company-link-back"><i aria-hidden="true" /><span>EVIDENCE + EXCEPTIONS</span></div>
        </div>
        <article className="company-card company-card-crew">
          <span>03 / EXECUTION NETWORK</span>
          <strong>Curators Crew</strong>
          <p>Matches approved work to capability and returns completion evidence or field exceptions.</p>
        </article>
        <div className="company-return"><i aria-hidden="true" /><span>VERIFIED OUTCOMES RETURN TO OPERATIONS</span></div>
      </div>
    </figure>
  );
}

export default function Expansion() {
  const navigate = useNavigate();
  const pageRef = useRef<HTMLElement>(null);
  const activeSlideRef = useRef(0);
  const timelinesRef = useRef(new Map<HTMLElement, Timeline>());
  const [activeSlide, setActiveSlide] = useState(0);
  const [reducedMotion, setReducedMotion] = useState(reducedMotionPreferred);

  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(query.matches);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const timelines = timelinesRef.current;
    const slides = Array.from(page.querySelectorAll<HTMLElement>("[data-expansion-slide]"));

    const enter = (slide: HTMLElement) => {
      if (reducedMotion) return;
      timelines.get(slide)?.revert();
      const reveals = slide.querySelectorAll<HTMLElement>("[data-animate='reveal']");
      if (!reveals.length) return;
      const timeline = createTimeline({ defaults: { ease: "outExpo" } }).add(reveals, {
        opacity: [0, 1],
        y: [38, 0],
        duration: 760,
        delay: stagger(55),
      });
      timelines.set(slide, timeline);
    };

    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        const slide = entry.target as HTMLElement;
        const index = Number(slide.dataset.expansionSlide ?? 0);
        activeSlideRef.current = index;
        setActiveSlide(index);
        enter(slide);
      });
    }, { root: page, threshold: 0.58 });

    slides.forEach((slide) => observer.observe(slide));
    enter(slides[0]);

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return;
      if (event.key === "Escape") {
        navigate("/login");
        return;
      }
      const delta = event.key === "ArrowDown" || event.key === "PageDown"
        ? 1
        : event.key === "ArrowUp" || event.key === "PageUp"
          ? -1
          : 0;
      if (!delta) return;
      const target = event.target as HTMLElement | null;
      if (target?.matches("input, textarea, select, [contenteditable='true']")) return;
      event.preventDefault();
      const next = Math.max(0, Math.min(slides.length - 1, activeSlideRef.current + delta));
      slides[next]?.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "start" });
    };

    window.addEventListener("keydown", onKeyDown);
    return () => {
      observer.disconnect();
      window.removeEventListener("keydown", onKeyDown);
      timelines.forEach((timeline) => timeline.revert());
      timelines.clear();
    };
  }, [navigate, reducedMotion]);

  const goToSlide = (index: number) => {
    pageRef.current
      ?.querySelector<HTMLElement>(`[data-expansion-slide='${index}']`)
      ?.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "start" });
  };

  return (
    <main ref={pageRef} className="expansion-page registration-frame" data-lenis-prevent>
      <header className="expansion-header">
        <Link to="/login" className="expansion-brand">COMFORT CURATORS / SUPERHOSTOS</Link>
        <span aria-live="polite">{String(activeSlide).padStart(2, "0")} / {slideLabels[activeSlide]}</span>
        <Link to="/login" className="expansion-exit">ESC / EXIT</Link>
      </header>

      <section className="expansion-slide story-opening" data-expansion-slide="0" aria-labelledby="story-title">
        <p className="story-kicker" data-animate="reveal">Comfort Curators / SuperhostOS</p>
        <h1 id="story-title" data-animate="reveal">One home is already<br /><span>a small <em>economy.</em></span></h1>
        <div className="story-opening-copy" data-animate="reveal">
          <p>A guest arrives. Another checks out. A cleaner is delayed. The air conditioner stops cooling. Supplies need restocking. The owner wants an update.</p>
          <p>Comfort Curators makes that complexity feel invisible to the owner and effortless to the guest.</p>
        </div>
        <p className="story-cue" data-animate="reveal">Scroll, swipe, or press ↓</p>
      </section>

      <section className="expansion-slide story-work" data-expansion-slide="1" aria-labelledby="work-title">
        <ChapterMeta number="01" label="The work underneath" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">Every stay is a sequence of dependent decisions</p>
          <h2 id="work-title" data-animate="reveal">Hospitality outside.<br /><em>Structured work underneath.</em></h2>
          <p data-animate="reveal">Every task needs the right person, instructions, access, and deadline. The property is only ready when the result is verified.</p>
        </div>
        <ArrowSequence items={propertySequence} ariaLabel="Property readiness sequence" />
      </section>

      <section className="expansion-slide story-evidence" data-expansion-slide="2" aria-labelledby="evidence-title">
        <ChapterMeta number="02" label="Evidence becomes memory" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">Every completed task leaves evidence</p>
          <h2 id="evidence-title" data-animate="reveal">Not an activity log.<br /><em>A working model.</em></h2>
          <p data-animate="reveal">Together, verified records show how real-world property operations succeed, fail, and improve. That operational intelligence is the foundation of SuperhostOS.</p>
        </div>
        <div className="evidence-ledger" data-animate="reveal">
          {[
            ["Need", "What the situation required"],
            ["Person", "Who performed the work"],
            ["Context", "What guidance mattered"],
            ["Exception", "What changed in the field"],
            ["Result", "Whether completion was verified"],
          ].map(([title, body], index) => (
            <article key={title}><span>{String(index + 1).padStart(2, "0")}</span><strong>{title}</strong><p>{body}</p></article>
          ))}
        </div>
      </section>

      <section className="expansion-slide story-system" data-expansion-slide="3" aria-labelledby="system-title">
        <ChapterMeta number="03" label="The operating system" />
        <div className="story-system-copy">
          <p className="story-kicker" data-animate="reveal">A guest reports: the air conditioner is not cooling</p>
          <h2 id="system-title" data-animate="reveal">It does more than<br /><em>generate a reply.</em></h2>
          <p data-animate="reveal">SuperhostOS reads property context, identifies the likely problem, finds qualified available capability, provides the procedure, tracks the work, and verifies completion.</p>
          <ArrowSequence items={operatingLoop} ariaLabel="SuperhostOS operating loop" />
        </div>
        <div className="story-terminal" data-animate="reveal">
          <p className="terminal-label">SUPERHOSTOS / LIVE OPERATING TRACE</p>
          <Terminal lines={incidentLines} cursorVisible />
          <p className="terminal-foot">The worker brings presence, judgment, and experience. SuperhostOS brings context, coordination, procedure, and memory.</p>
        </div>
      </section>

      <section className="expansion-slide story-authority" data-expansion-slide="4" aria-labelledby="authority-title">
        <ChapterMeta number="04" label="Bounded authority" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">The agent proposes. Policy decides.</p>
          <h2 id="authority-title" data-animate="reveal">An operator that never<br /><em>outruns its authority.</em></h2>
          <p data-animate="reveal">Every tool proposal passes through policy. Safe reads can proceed; operational actions pause for approval; unsupported authority ends the run honestly.</p>
        </div>
        <div className="policy-flow" data-animate="reveal" aria-label="SuperhostOS authority flow">
          {[
            ["01", "Claim the run", "A user starts a scoped session."],
            ["02", "Assemble context", "Property, reservation, incident."],
            ["03", "Model proposes", "Allowlisted tool + rationale."],
            ["04", "Policy evaluates", "Proceed, pause, or refuse."],
            ["05", "Return evidence", "Completion or exception."],
          ].map(([number, title, body]) => <article key={number}><span>{number}</span><strong>{title}</strong><p>{body}</p></article>)}
        </div>
        <aside className="authority-boundary" data-animate="reveal">
          <strong>PROPOSAL IS NOT RESULT</strong>
          <p>SuperhostOS cannot directly pay, order, sign, certify, or file. When it lacks authority, it says so and stops.</p>
        </aside>
      </section>

      <section className="expansion-slide story-capability" data-expansion-slide="5" aria-labelledby="capability-title">
        <ChapterMeta number="05" label="Human Capability Index" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">A job title is a poor description of a person</p>
          <h2 id="capability-title" data-animate="reveal">Record what people<br /><em>can actually do.</em></h2>
          <p data-animate="reveal">The Index is not a public score or leaderboard. It is a structured, evolving record of demonstrated capability: observed, assisted, independently verified, and repeated over time.</p>
        </div>
        <div className="capability-sheet" data-animate="reveal">
          <header><span>CAPABILITY RECORD / PRIVATE</span><span>EVIDENCE, NOT CLAIMS</span></header>
          {[
            ["Qualification", "Current + task-specific"],
            ["Reliability", "Repeated verified outcomes"],
            ["Recency", "Capability changes with time"],
            ["Complexity", "Conditions and property context"],
            ["Support", "Independent / guided / supervised"],
          ].map(([key, value]) => <div key={key}><strong>{key}</strong><span>{value}</span></div>)}
        </div>
        <DocumentaryPhoto src={crewPhoto} alt="A painted auto-rickshaw on an Indian street" caption="PEOPLE IN CONTEXT / CAPABILITY IN MOTION" />
      </section>

      <section className="expansion-slide story-match" data-expansion-slide="6" aria-labelledby="match-title">
        <ChapterMeta number="06" label="Better matching" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">The goal is not to find any worker</p>
          <h2 id="match-title" data-animate="reveal">Find the right<br /><em>available capability.</em></h2>
          <p data-animate="reveal">Match the work while recognizing when someone needs guidance, supervision, or escalation.</p>
        </div>
        <div className="match-board" data-animate="reveal">
          <div className="match-brief"><span>WORK / AC DIAGNOSTIC</span><strong>Unit 204</strong><p>High urgency · guest occupied · split system</p></div>
          <div className="match-arrow" aria-hidden="true">→</div>
          <div className="match-capability"><span>CAPABILITY / VERIFIED</span><strong>Available now</strong><p>Guided procedure · recent evidence · correct zone</p></div>
          <div className="match-outcome"><span>SUPPORT LEVEL</span><strong>Guide + verify</strong><p>Escalate if cooling delta remains below standard.</p></div>
        </div>
        <blockquote data-animate="reveal">It is not a hospitality chatbot. It is an operating system for getting real work done.</blockquote>
      </section>

      <section className="expansion-slide story-companies" data-expansion-slide="7" aria-labelledby="companies-title">
        <ChapterMeta number="07" label="The company system" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">Operations → intelligence ↔ capability → operations</p>
          <h2 id="companies-title" data-animate="reveal">Three roles.<br /><em>One loop.</em></h2>
          <p data-animate="reveal">The arrows carry specific value. Comfort Curators supplies real operating context. SuperhostOS and Curators Crew exchange approved work, guidance, evidence, and exceptions. Verified outcomes return to the operation.</p>
        </div>
        <CompanyFlywheel />
      </section>

      <section className="expansion-slide story-market" data-expansion-slide="8" aria-labelledby="market-title">
        <ChapterMeta number="08" label="Why hospitality first" />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">Hospitality is not the limit. It is the training environment.</p>
          <h2 id="market-title" data-animate="reveal">A dense laboratory<br /><em>for real work.</em></h2>
          <p data-animate="reveal">Cleaning, maintenance, inspection, inventory, procurement, delivery, scheduling, and guest service coexist inside one operating system—then extend into facilities, logistics, retail, food service, events, and residential support.</p>
        </div>
        <div className="market-stats">
          <article data-animate="reveal"><strong>84.63M</strong><span>tourism-linked jobs in India / 2023–24</span></article>
          <article data-animate="reveal"><strong>23.5M</strong><span>projected Indian gig workforce / 2029–30</span><small>7.7M in 2020</small></article>
          <article data-animate="reveal"><strong>39%</strong><span>of workers' core skills expected to change / 2030</span></article>
        </div>
      </section>

      <section className="expansion-slide story-compounding" data-expansion-slide="9" aria-labelledby="compounding-title">
        <ChapterMeta number="09" label="Start narrow. Compound." />
        <div className="story-heading">
          <p className="story-kicker" data-animate="reveal">30+ existing property relationships / one measurable objective</p>
          <h2 id="compounding-title" data-animate="reveal">Learn to run homes<br /><em>exceptionally well.</em></h2>
          <p data-animate="reveal">Today, Comfort Curators earns revenue by coordinating real hospitality services. Next, SuperhostOS structures verified outcomes. Later, that intelligence supports Curators Crew and adjacent service work.</p>
        </div>
        <ol className="growth-loop">
          {compoundingLoop.map(([title, body], index) => <li key={title} data-animate="reveal"><span>{String(index + 1).padStart(2, "0")}</span><strong>{title}</strong><p>{body}</p></li>)}
        </ol>
        <p className="compounding-proof" data-animate="reveal">More properties → more real tasks → more verified outcomes → better execution and economics → more demand.</p>
      </section>

      <section className="expansion-slide story-close" data-expansion-slide="10" aria-labelledby="close-title">
        <div className="close-statement">
          <p className="story-kicker" data-animate="reveal">The vision is large. The starting point is measurable.</p>
          <h2 id="close-title" data-animate="reveal">Build around what<br /><em>people can do.</em></h2>
          <p data-animate="reveal">We are not starting by rebuilding the labor market. We are starting with one difficult, valuable operating environment and building outward from evidence.</p>
          <strong data-animate="reveal">Comfort Curators / SuperhostOS</strong>
        </div>
        <DocumentaryPhoto src={superhostPhoto} alt="A painted building facade with windows opening onto different rooms" caption="EVERY ROOM IN CONTEXT / MUMBAI" />
        <div className="source-notes" data-animate="reveal">
          <p>SOURCE NOTES / ACCESSED AUGUST 2026</p>
          {sources.map((source, index) => (
            <a href={source.href} target="_blank" rel="noreferrer" key={source.href}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <strong>{source.label}</strong>
              <small>{source.note}</small>
            </a>
          ))}
        </div>
      </section>

      <nav className="stage-progress" aria-label="Expansion story chapters">
        {slideLabels.map((label, index) => (
          <button key={label} type="button" data-active={activeSlide === index} onClick={() => goToSlide(index)} aria-label={`Go to chapter ${index + 1}: ${label}`}>
            <span>{label}</span>
          </button>
        ))}
      </nav>
    </main>
  );
}
