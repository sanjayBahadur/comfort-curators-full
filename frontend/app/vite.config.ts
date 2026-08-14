import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const apiProxy = {
  "/api": {
    target: "http://127.0.0.1:8080",
    changeOrigin: true,
    rewrite: (path: string) => path.replace(/^\/api/, ""),
  },
};

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // MUST stay 3000 — the backend polls http://host.docker.internal:3000/demo.ics
    // to ingest reservations. See SETUP.md §6.
    port: 3000,
    strictPort: true,
    // REQUIRED: Vite binds to localhost by default, so the backend CONTAINER
    // cannot reach http://host.docker.internal:3000/demo.ics to ingest
    // reservations. Without this, Phase 2's calendar poll fails silently and
    // the whole reservation -> turnover -> ticket chain has no data. SETUP.md §6.
    host: true,
    // Vite rejects unknown Host headers with 403 (DNS-rebinding protection).
    // The backend polls us as "host.docker.internal", so it must be allowed.
    allowedHosts: ["host.docker.internal", "localhost"],
    // The Go backend sends no CORS headers and its OPTIONS preflight 401s.
    // Same-origin proxy means no preflight is ever issued. ARCHITECTURE.md §1.
    proxy: apiProxy,
  },
  preview: {
    port: 3000,
    strictPort: true,
    host: true,
    allowedHosts: ["host.docker.internal", "localhost"],
    proxy: apiProxy,
  },
});
