import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { homeFor } from "../lib/auth/roles";
import { signIn, type Role } from "../lib/auth/session";
import minarHero from "../assets/hero/minar-login-hero.webp";
import "./login.css";

type RoleOption = {
  role: Role;
  label: string;
  contact: string;
  index: string;
  lane: string;
  description: string;
};

const roleOptions: RoleOption[] = [
  {
    role: "owner",
    label: "Owner",
    contact: "owner@demo.test",
    index: "01",
    lane: "Exceptions + approvals",
    description: "See what needs a decision and leave routine operations quiet.",
  },
  {
    role: "staff",
    label: "Staff",
    contact: "staff@demo.test",
    index: "02",
    lane: "Operations + curation",
    description: "Triage new work, coordinate curators, and keep every property on schedule.",
  },
  {
    role: "guest",
    label: "Guest",
    contact: "guest@demo.test",
    index: "03",
    lane: "Stay + local essentials",
    description: "Find arrival details, house guidance, and useful extras for your stay.",
  },
];

export default function Login() {
  const navigate = useNavigate();
  const [busyRole, setBusyRole] = useState<Role | null>(null);

  function replayIntro() {
    sessionStorage.removeItem("cc_intro_seen");
    navigate("/?replay=intro");
  }

  async function chooseRole(option: RoleOption) {
    setBusyRole(option.role);

    try {
      await signIn(option.role, option.contact);
      navigate(homeFor(option.role), { replace: true });
    } catch {
      // signIn() surfaces the API's message globally; this card simply resets.
    } finally {
      setBusyRole(null);
    }
  }

  return (
    <main className="access-page registration-frame">
      <header className="access-meta">
        <span>COMFORT CURATORS / ACCESS</span>
        <span>BUILD 08</span>
      </header>
      <Link to="/expansion" className="access-pitch-button">THE PITCH <b aria-hidden="true">→</b></Link>

      <section className="access-intro" aria-labelledby="login-title">
        <div className="access-headline">
          <h1 id="login-title" className="access-title">
            Choose your
            <br />
            <em>keys.</em>
          </h1>
        </div>
        <div className="access-hero-media">
          <img className="access-hero-image" src={minarHero} alt="" />
          <button
            className="access-hero-key"
            type="button"
            aria-label="Replay the Comfort Curators intro"
            data-cursor-scale="7.6"
            onClick={replayIntro}
          />
        </div>
      </section>

      <section className="access-roles" aria-labelledby="role-title">
        <h2 id="role-title" className="sr-only">Choose a role</h2>
        <div className="access-role-grid">
          {roleOptions.map((option) => {
            const isBusy = busyRole === option.role;

            return (
              <button
                key={option.role}
                className={`access-role-card access-role-card--${option.role}`}
                type="button"
                aria-busy={isBusy}
                disabled={busyRole !== null}
                onClick={() => void chooseRole(option)}
              >
                <span className="access-role-index">{option.index}</span>
                <span className="access-role-label">{option.label}</span>
                <span className="access-role-lane">{option.lane}</span>
                <span className="access-role-description">{option.description}</span>
                <span className="access-role-action">
                  {isBusy ? "MINTING…" : "ENTER →"}
                </span>
              </button>
            );
          })}
        </div>
      </section>
    </main>
  );
}
