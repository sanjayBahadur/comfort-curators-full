// Package observability provides business-aware observability primitives for
// the Comfort Curators backend: correlation that crosses the API, durable job,
// outbox and model tool-call boundaries, in-process traces, business metrics
// for API, database, outbox, job, calendar, notification, file, AI and
// authorization work, and business-effect alerts for property readiness,
// assignment, incident, stock and approval outcomes.
//
// All content emitted through this package is redacted before it leaves the
// process: sensitive values are never written to logs, traces, metrics or
// alerts. Correlation identity is preserved through redaction so operators can
// still join events across services.
package observability

// RedactedValue is the canonical placeholder for any sensitive value that must
// not leave the process.
const RedactedValue = "[redacted]"
