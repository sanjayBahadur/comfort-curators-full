# Comfort Curators frontend

Vite + React application for the Comfort Curators owner, operations, and
curator surfaces. Product requirements and phase acceptance tests live one
directory above this app.

## Local development

The Go backend must be running on `127.0.0.1:8080` with
`CC_BUILD_TAGS=acceptance`. Then:

```bash
npm install
npm run dev
```

Open `http://localhost:3000/debug`. Browser API calls must use `/api/*`; Vite
proxies them to the backend to avoid its CORS-blocked preflight behavior.

## Checks

```bash
npm run lint
npm run build
```

See `../POLICY.md` before changing the app. Build exactly one phase from
`../PHASES.md`, write its log, and stop for manual acceptance.
