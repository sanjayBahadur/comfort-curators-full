import { Link } from "react-router-dom";

export default function Home() {
  return (
    <main style={{ padding: 32, fontFamily: "ui-monospace, monospace" }}>
      <p>comfort curators — phase 0</p>
      <Link to="/debug">/debug →</Link>
    </main>
  );
}
