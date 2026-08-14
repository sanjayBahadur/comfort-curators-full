# Route Inventory (generated, 340 routes)

Generated from source. The OpenAPI contract covers 65 of these; this file covers all of them.

Payload shapes: open the handler file listed and find the request struct.


## access (14)

- `GET    /v1/access-grants/{grant_id}` — `internal/access/handler.go`
- `POST   /v1/access-grants/{grant_id}/acknowledge` — `internal/access/handler.go`
- `POST   /v1/access-grants/{grant_id}/disclose` — `internal/access/handler.go`
- `GET    /v1/access-grants/{grant_id}/disclosures` — `internal/access/handler.go`
- `POST   /v1/access-grants/{grant_id}/return` — `internal/access/handler.go`
- `POST   /v1/access-grants/{grant_id}/revoke` — `internal/access/handler.go`
- `DELETE /v1/access-holds/{hold_id}` — `internal/access/handler.go`
- `GET    /v1/properties/{property_id}/access-custody-events` — `internal/access/handler.go`
- `GET    /v1/properties/{property_id}/access-grants` — `internal/access/handler.go`
- `POST   /v1/properties/{property_id}/access-grants` — `internal/access/handler.go`
- `POST   /v1/properties/{property_id}/access-holds` — `internal/access/handler.go`
- `GET    /v1/properties/{property_id}/access-secrets` — `internal/access/handler.go`
- `POST   /v1/properties/{property_id}/access-secrets` — `internal/access/handler.go`
- `POST   /v1/properties/{property_id}/emergency-access` — `internal/access/handler.go`

## api(slices) (33)

- `POST   /v1/accounting-exports` — `internal/api/finance_handler.go`
- `POST   /v1/bank-verifications` — `internal/api/finance_handler.go`
- `POST   /v1/bank-verifications/{verification_id}/confirm` — `internal/api/finance_handler.go`
- `POST   /v1/billing/charges` — `internal/api/finance_handler.go`
- `POST   /v1/billing/credits` — `internal/api/finance_handler.go`
- `POST   /v1/billing/invoices` — `internal/api/finance_handler.go`
- `GET    /v1/document-versions/{version_id}/extractions` — `internal/api/finance_handler.go`
- `POST   /v1/document-versions/{version_id}/extractions` — `internal/api/finance_handler.go`
- `POST   /v1/documents` — `internal/api/finance_handler.go`
- `GET    /v1/documents/{document_id}` — `internal/api/finance_handler.go`
- `GET    /v1/documents/{document_id}/reviews` — `internal/api/finance_handler.go`
- `POST   /v1/documents/{document_id}/reviews` — `internal/api/finance_handler.go`
- `GET    /v1/documents/{document_id}/versions` — `internal/api/finance_handler.go`
- `POST   /v1/documents/{document_id}/versions` — `internal/api/finance_handler.go`
- `POST   /v1/financial-approvals/{approval_id}/decisions` — `internal/api/finance_handler.go`
- `POST   /v1/journal/finalize` — `internal/api/finance_handler.go`
- `POST   /v1/maker-checker/decisions` — `internal/api/finance_handler.go`
- `POST   /v1/maker-checker/requests` — `internal/api/finance_handler.go`
- `POST   /v1/maker-checker/requests/{request_id}/submit` — `internal/api/finance_handler.go`
- `POST   /v1/owners/onboarding-cases` — `internal/api/handlers.go`
- `POST   /v1/properties/{property_id}/access-disclosures` — `internal/api/handlers.go`
- `POST   /v1/properties/{property_id}/contracts` — `internal/api/handlers.go`
- `GET    /v1/properties/{property_id}/documents` — `internal/api/finance_handler.go`
- `POST   /v1/properties/{property_id}/documents/expiry-check` — `internal/api/finance_handler.go`
- `POST   /v1/properties/{property_id}/inspections` — `internal/api/handlers.go`
- `POST   /v1/properties/{property_id}/submission-packets` — `internal/api/finance_handler.go`
- `GET    /v1/reconciliation-exceptions` — `internal/api/finance_handler.go`
- `POST   /v1/reconciliation-exceptions` — `internal/api/finance_handler.go`
- `POST   /v1/reconciliation-exceptions/{exception_id}/resolve` — `internal/api/finance_handler.go`
- `GET    /v1/reports/property-contribution` — `internal/api/finance_handler.go`
- `GET    /v1/submission-packets/{packet_id}` — `internal/api/finance_handler.go`
- `POST   /v1/submission-packets/{packet_id}/confirmations` — `internal/api/finance_handler.go`
- `GET    /v1/submission-packets/{packet_id}/receipt` — `internal/api/finance_handler.go`

## automation (11)

- `POST   /v1/agent-runs` — `internal/automation/handler.go`
- `GET    /v1/agent-runs/{run_id}` — `internal/automation/handler.go`
- `POST   /v1/agent-runs/{run_id}/cancel` — `internal/automation/handler.go`
- `GET    /v1/agent-runs/{run_id}/events` — `internal/automation/handler.go`
- `POST   /v1/agent-runs/{run_id}/retry` — `internal/automation/handler.go`
- `GET    /v1/hermes/deliveries` — `internal/automation/hermes/handler.go`
- `GET    /v1/hermes/deliveries/{delivery_id}` — `internal/automation/hermes/handler.go`
- `POST   /v1/hermes/drafts` — `internal/automation/hermes/handler.go`
- `POST   /v1/hermes/drafts/{draft_id}/deliver` — `internal/automation/hermes/handler.go`
- `POST   /v1/hermes/drafts/{draft_id}/review` — `internal/automation/hermes/handler.go`
- `POST   /v1/jarvis/runs` — `internal/automation/jarvis/handler.go`

## catalog (13)

- `GET    /v1/catalog/items` — `internal/catalog/handler.go`
- `POST   /v1/catalog/items` — `internal/catalog/handler.go`
- `GET    /v1/catalog/items/{item_id}` — `internal/catalog/handler.go`
- `GET    /v1/catalog/items/{item_id}/claims` — `internal/catalog/handler.go`
- `POST   /v1/catalog/items/{item_id}/claims` — `internal/catalog/handler.go`
- `GET    /v1/catalog/templates` — `internal/catalog/handler.go`
- `POST   /v1/catalog/templates` — `internal/catalog/handler.go`
- `GET    /v1/catalog/templates/{template_id}` — `internal/catalog/handler.go`
- `GET    /v1/properties/{property_id}/packages` — `internal/catalog/handler.go`
- `POST   /v1/properties/{property_id}/packages` — `internal/catalog/handler.go`
- `GET    /v1/properties/{property_id}/packages/{version_id}` — `internal/catalog/handler.go`
- `POST   /v1/properties/{property_id}/packages/{version_id}/activate` — `internal/catalog/handler.go`
- `POST   /v1/properties/{property_id}/packages/{version_id}/reject` — `internal/catalog/handler.go`

## communications (20)

- `GET    /v1/communications/deliveries` — `internal/communications/handler.go`
- `POST   /v1/communications/deliveries/{delivery_id}/result` — `internal/communications/handler.go`
- `POST   /v1/communications/drafts` — `internal/communications/handler.go`
- `GET    /v1/communications/drafts/{draft_id}` — `internal/communications/handler.go`
- `POST   /v1/communications/drafts/{draft_id}/deliver` — `internal/communications/handler.go`
- `GET    /v1/communications/drafts/{draft_id}/preview` — `internal/communications/handler.go`
- `POST   /v1/communications/drafts/{draft_id}/review` — `internal/communications/handler.go`
- `GET    /v1/communications/drafts/{draft_id}/reviews` — `internal/communications/handler.go`
- `GET    /v1/communications/preferences` — `internal/communications/handler.go`
- `PUT    /v1/communications/preferences` — `internal/communications/handler.go`
- `GET    /v1/communications/secure-links` — `internal/communications/handler.go`
- `POST   /v1/communications/secure-links` — `internal/communications/handler.go`
- `POST   /v1/communications/secure-links/redeem` — `internal/communications/handler.go`
- `POST   /v1/communications/secure-links/{link_id}/revoke` — `internal/communications/handler.go`
- `GET    /v1/communications/templates` — `internal/communications/handler.go`
- `POST   /v1/communications/templates` — `internal/communications/handler.go`
- `GET    /v1/communications/templates/{template_key}` — `internal/communications/handler.go`
- `GET    /v1/communications/templates/{template_key}/preview` — `internal/communications/handler.go`
- `GET    /v1/communications/templates/{template_key}/resolve` — `internal/communications/handler.go`
- `POST   /v1/communications/templates/{template_key}/versions` — `internal/communications/handler.go`

## compliance (7)

- `POST   /v1/compliance/items` — `internal/compliance/handler.go`
- `GET    /v1/compliance/items/{item_id}` — `internal/compliance/handler.go`
- `POST   /v1/compliance/items/{item_id}/renew` — `internal/compliance/handler.go`
- `GET    /v1/compliance/properties/{property_id}/items` — `internal/compliance/handler.go`
- `GET    /v1/compliance/properties/{property_id}/warnings` — `internal/compliance/handler.go`
- `POST   /v1/compliance/scan-expiry` — `internal/compliance/handler.go`
- `POST   /v1/compliance/warnings/{warning_id}/acknowledge` — `internal/compliance/handler.go`

## consumer (8)

- `POST   /v1/consumer/acceptances` — `internal/consumer/handler.go`
- `GET    /v1/consumer/acceptances/{acceptance_id}` — `internal/consumer/handler.go`
- `GET    /v1/consumer/disclosures` — `internal/consumer/handler.go`
- `POST   /v1/consumer/disclosures` — `internal/consumer/handler.go`
- `GET    /v1/consumer/disclosures/{disclosure_id}` — `internal/consumer/handler.go`
- `GET    /v1/consumer/history-exports` — `internal/consumer/handler.go`
- `POST   /v1/consumer/history-exports` — `internal/consumer/handler.go`
- `GET    /v1/consumer/history-exports/{export_id}` — `internal/consumer/handler.go`

## contracts (8)

- `GET    /v1/contracts/agreements` — `internal/contracts/handler.go`
- `POST   /v1/contracts/agreements` — `internal/contracts/handler.go`
- `GET    /v1/contracts/agreements/{agreement_id}` — `internal/contracts/handler.go`
- `POST   /v1/contracts/agreements/{agreement_id}/accept` — `internal/contracts/handler.go`
- `POST   /v1/contracts/agreements/{agreement_id}/versions` — `internal/contracts/handler.go`
- `PUT    /v1/contracts/agreements/{agreement_id}/versions` — `internal/contracts/handler.go`
- `POST   /v1/contracts/fee-rules` — `internal/contracts/handler.go`
- `POST   /v1/contracts/quotes` — `internal/contracts/handler.go`

## fleet (17)

- `GET    /v1/fleet/assets` — `internal/fleet/handler.go`
- `POST   /v1/fleet/assets` — `internal/fleet/handler.go`
- `GET    /v1/fleet/assets/{asset_id}` — `internal/fleet/handler.go`
- `GET    /v1/fleet/assets/{asset_id}/custody-events` — `internal/fleet/handler.go`
- `POST   /v1/fleet/assets/{asset_id}/custody/handover` — `internal/fleet/handler.go`
- `POST   /v1/fleet/assets/{asset_id}/custody/return` — `internal/fleet/handler.go`
- `GET    /v1/fleet/assets/{asset_id}/dispatch-eligibility` — `internal/fleet/handler.go`
- `POST   /v1/fleet/assets/{asset_id}/incidents` — `internal/fleet/handler.go`
- `GET    /v1/fleet/assets/{asset_id}/incidents/open` — `internal/fleet/handler.go`
- `POST   /v1/fleet/assets/{asset_id}/inspections` — `internal/fleet/handler.go`
- `POST   /v1/fleet/assets/{asset_id}/safety-items` — `internal/fleet/handler.go`
- `GET    /v1/fleet/assets/{asset_id}/safety-items/overdue` — `internal/fleet/handler.go`
- `GET    /v1/fleet/incidents/{incident_id}` — `internal/fleet/handler.go`
- `POST   /v1/fleet/incidents/{incident_id}/review` — `internal/fleet/handler.go`
- `POST   /v1/fleet/safety-items/{record_id}/complete` — `internal/fleet/handler.go`
- `POST   /v1/fleet/workers/{worker_id}/locations` — `internal/fleet/handler.go`
- `GET    /v1/fleet/workers/{worker_id}/tracking-status` — `internal/fleet/handler.go`

## iam (16)

- `POST   /access/check` — `internal/iam/handler.go`
- `POST   /auth/mfa/check` — `internal/iam/handler.go`
- `POST   /auth/mfa/confirm` — `internal/iam/handler.go`
- `POST   /auth/mfa/enroll` — `internal/iam/handler.go`
- `POST   /auth/mfa/verify` — `internal/iam/handler.go`
- `POST   /auth/otp/request` — `internal/iam/handler.go`
- `POST   /auth/otp/verify` — `internal/iam/handler.go`
- `GET    /auth/session` — `internal/iam/handler.go`
- `POST   /auth/session/create` — `internal/iam/testfixtures.go`
- `POST   /auth/session/revoke` — `internal/iam/handler.go`
- `DELETE /support-access-grants/{grant_id}` — `internal/iam/handler.go`
- `POST   /tenants` — `internal/iam/handler.go`
- `GET    /tenants/{tenant_id}` — `internal/iam/handler.go`
- `POST   /tenants/{tenant_id}/memberships` — `internal/iam/handler.go`
- `DELETE /tenants/{tenant_id}/memberships/{user_id}` — `internal/iam/handler.go`
- `POST   /tenants/{tenant_id}/support-access-grants` — `internal/iam/handler.go`

## inventory (11)

- `POST   /v1/inventory/counts` — `internal/inventory/handler.go`
- `GET    /v1/inventory/counts/{count_id}` — `internal/inventory/handler.go`
- `PUT    /v1/inventory/counts/{count_id}/lines` — `internal/inventory/handler.go`
- `POST   /v1/inventory/counts/{count_id}/reconcile` — `internal/inventory/handler.go`
- `POST   /v1/inventory/counts/{count_id}/review` — `internal/inventory/handler.go`
- `GET    /v1/inventory/locations` — `internal/inventory/handler.go`
- `POST   /v1/inventory/locations` — `internal/inventory/handler.go`
- `GET    /v1/inventory/locations/{location_id}` — `internal/inventory/handler.go`
- `GET    /v1/inventory/locations/{location_id}/balances/{catalog_item_id}` — `internal/inventory/handler.go`
- `POST   /v1/inventory/locations/{location_id}/movements` — `internal/inventory/handler.go`
- `GET    /v1/inventory/locations/{location_id}/movements/{catalog_item_id}` — `internal/inventory/handler.go`

## maintenance (21)

- `POST   /v1/maintenance/estimates/{estimate_id}/decide` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/estimates/{estimate_id}/submit` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/requests` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/requests` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/requests/{request_id}` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/requests/{request_id}/approvals` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/requests/{request_id}/approvals` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/requests/{request_id}/estimates` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/requests/{request_id}/estimates` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/requests/{request_id}/triage` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/requests/{request_id}/work-orders` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/vendor/work-orders` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/vendor/work-orders/{work_order_id}` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/warranties` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/warranties/{warranty_id}` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/work-orders` — `internal/maintenance/handler.go`
- `GET    /v1/maintenance/work-orders/{work_order_id}` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/work-orders/{work_order_id}/complete` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/work-orders/{work_order_id}/start` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/work-orders/{work_order_id}/verify` — `internal/maintenance/handler.go`
- `POST   /v1/maintenance/work-orders/{work_order_id}/warranty` — `internal/maintenance/handler.go`

## observability (5)

- `GET    /alerts` — `internal/observability/handler.go`
- `GET    /alerts/unresolved` — `internal/observability/handler.go`
- `GET    /metrics` — `internal/observability/handler.go`
- `GET    /traces` — `internal/observability/handler.go`
- `GET    /traces/{traceID}` — `internal/observability/handler.go`

## onboarding (10)

- `GET    /v1/onboarding/cases` — `internal/onboarding/handler.go`
- `POST   /v1/onboarding/cases` — `internal/onboarding/handler.go`
- `GET    /v1/onboarding/cases/{case_id}` — `internal/onboarding/handler.go`
- `POST   /v1/onboarding/cases/{case_id}/activate` — `internal/onboarding/handler.go`
- `GET    /v1/onboarding/cases/{case_id}/activation-holds` — `internal/onboarding/handler.go`
- `PUT    /v1/onboarding/cases/{case_id}/contacts` — `internal/onboarding/handler.go`
- `POST   /v1/onboarding/cases/{case_id}/evidence` — `internal/onboarding/handler.go`
- `POST   /v1/onboarding/cases/{case_id}/inspections` — `internal/onboarding/handler.go`
- `GET    /v1/onboarding/cases/{case_id}/progress` — `internal/onboarding/handler.go`
- `PUT    /v1/onboarding/cases/{case_id}/sections/{section}` — `internal/onboarding/handler.go`

## operations (37)

- `GET    /v1/alerts/incident` — `internal/operations/handler.go`
- `GET    /v1/dispatch/assignments/{assignment_id}` — `internal/operations/dispatch_handler.go`
- `POST   /v1/dispatch/assignments/{assignment_id}/accept` — `internal/operations/dispatch_handler.go`
- `POST   /v1/dispatch/assignments/{assignment_id}/decline` — `internal/operations/dispatch_handler.go`
- `GET    /v1/dispatch/workers/{worker_id}/assignments` — `internal/operations/dispatch_handler.go`
- `GET    /v1/dispatch/workers/{worker_id}/treatment` — `internal/operations/dispatch_handler.go`
- `GET    /v1/evidence/{evidence_id}` — `internal/operations/handler.go`
- `GET    /v1/recovery/{recovery_id}` — `internal/operations/handler.go`
- `POST   /v1/recovery/{recovery_id}/close` — `internal/operations/handler.go`
- `POST   /v1/sync-conflicts/{conflict_id}/resolve` — `internal/operations/handler.go`
- `GET    /v1/tickets` — `internal/operations/handler.go`
- `POST   /v1/tickets` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}/alerts` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/blockers` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/cancel` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}/checklist-items` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/checklist-syncs` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/checklist-syncs/idempotent` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/classify` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/dispatch/assign` — `internal/operations/dispatch_handler.go`
- `GET    /v1/tickets/{ticket_id}/dispatch/assignments` — `internal/operations/dispatch_handler.go`
- `POST   /v1/tickets/{ticket_id}/dispatch/candidates` — `internal/operations/dispatch_handler.go`
- `POST   /v1/tickets/{ticket_id}/dispatch/override` — `internal/operations/dispatch_handler.go`
- `GET    /v1/tickets/{ticket_id}/dispatch/overrides` — `internal/operations/dispatch_handler.go`
- `GET    /v1/tickets/{ticket_id}/evidence` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/evidence` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}/offline-evidence` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/offline-evidence` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/offline-evidence/sync` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}/recovery` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/recovery` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/reopen` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}/state-events` — `internal/operations/handler.go`
- `GET    /v1/tickets/{ticket_id}/sync-conflicts` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/transitions` — `internal/operations/handler.go`
- `POST   /v1/tickets/{ticket_id}/unblock` — `internal/operations/handler.go`

## platform (2)

- `GET    /jobs/dead-letter` — `internal/platform/app/app.go`
- `GET    /security/findings` — `internal/platform/app/app.go`

## privacy (26)

- `POST   /v1/privacy/aadhaar-preferences` — `internal/privacy/handler.go`
- `GET    /v1/privacy/aadhaar-preferences/{actor_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/consents` — `internal/privacy/handler.go`
- `GET    /v1/privacy/consents/{consent_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/consents/{consent_id}/withdraw` — `internal/privacy/handler.go`
- `POST   /v1/privacy/evaluation-exports` — `internal/privacy/handler.go`
- `GET    /v1/privacy/evaluation-exports/{export_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/evaluation-exports/{export_id}/approve` — `internal/privacy/handler.go`
- `POST   /v1/privacy/evaluation-exports/{export_id}/deny` — `internal/privacy/handler.go`
- `POST   /v1/privacy/identity-alternatives` — `internal/privacy/handler.go`
- `GET    /v1/privacy/identity-alternatives/{alt_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/notices` — `internal/privacy/handler.go`
- `GET    /v1/privacy/notices/{notice_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/processors` — `internal/privacy/handler.go`
- `GET    /v1/privacy/processors/{contract_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/processors/{contract_id}/review` — `internal/privacy/handler.go`
- `GET    /v1/privacy/purposes` — `internal/privacy/handler.go`
- `POST   /v1/privacy/purposes` — `internal/privacy/handler.go`
- `GET    /v1/privacy/purposes/{purpose_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/retention-records` — `internal/privacy/handler.go`
- `GET    /v1/privacy/retention-records/{record_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/rights-requests` — `internal/privacy/handler.go`
- `GET    /v1/privacy/rights-requests/{request_id}` — `internal/privacy/handler.go`
- `POST   /v1/privacy/rights-requests/{request_id}/process` — `internal/privacy/handler.go`
- `POST   /v1/privacy/security-log-settings` — `internal/privacy/handler.go`
- `GET    /v1/privacy/security-log-settings/{setting_id}` — `internal/privacy/handler.go`

## procurement (25)

- `GET    /v1/procurement/purchase-orders` — `internal/procurement/handler.go`
- `POST   /v1/procurement/purchase-orders` — `internal/procurement/handler.go`
- `GET    /v1/procurement/purchase-orders/{po_id}` — `internal/procurement/handler.go`
- `POST   /v1/procurement/purchase-orders/{po_id}/issue` — `internal/procurement/handler.go`
- `GET    /v1/procurement/purchase-orders/{po_id}/receipts` — `internal/procurement/handler.go`
- `POST   /v1/procurement/purchase-orders/{po_id}/receipts` — `internal/procurement/handler.go`
- `GET    /v1/procurement/rebates` — `internal/procurement/handler.go`
- `POST   /v1/procurement/rebates` — `internal/procurement/handler.go`
- `GET    /v1/procurement/rebates/{rebate_id}` — `internal/procurement/handler.go`
- `POST   /v1/procurement/rebates/{rebate_id}/settle` — `internal/procurement/handler.go`
- `GET    /v1/procurement/receipts/{receipt_id}` — `internal/procurement/handler.go`
- `GET    /v1/procurement/requisitions` — `internal/procurement/handler.go`
- `POST   /v1/procurement/requisitions` — `internal/procurement/handler.go`
- `GET    /v1/procurement/requisitions/{requisition_id}` — `internal/procurement/handler.go`
- `GET    /v1/procurement/requisitions/{requisition_id}/approvals` — `internal/procurement/handler.go`
- `POST   /v1/procurement/requisitions/{requisition_id}/approve` — `internal/procurement/handler.go`
- `POST   /v1/procurement/requisitions/{requisition_id}/reject` — `internal/procurement/handler.go`
- `GET    /v1/procurement/supplier-items/{item_id}` — `internal/procurement/handler.go`
- `GET    /v1/procurement/suppliers` — `internal/procurement/handler.go`
- `POST   /v1/procurement/suppliers` — `internal/procurement/handler.go`
- `GET    /v1/procurement/suppliers/{supplier_id}` — `internal/procurement/handler.go`
- `POST   /v1/procurement/suppliers/{supplier_id}/approve` — `internal/procurement/handler.go`
- `GET    /v1/procurement/suppliers/{supplier_id}/items` — `internal/procurement/handler.go`
- `POST   /v1/procurement/suppliers/{supplier_id}/items` — `internal/procurement/handler.go`
- `POST   /v1/procurement/suppliers/{supplier_id}/reject` — `internal/procurement/handler.go`

## property (9)

- `GET    /v1/properties` — `internal/property/handler.go`
- `POST   /v1/properties` — `internal/property/handler.go`
- `GET    /v1/properties/{property_id}` — `internal/property/handler.go`
- `POST   /v1/properties/{property_id}/compliance-holds` — `internal/property/handler.go`
- `POST   /v1/properties/{property_id}/compliance-holds/{hold_id}/exception` — `internal/property/handler.go`
- `POST   /v1/properties/{property_id}/compliance-holds/{hold_id}/resolve` — `internal/property/handler.go`
- `PUT    /v1/properties/{property_id}/readiness` — `internal/property/handler.go`
- `GET    /v1/properties/{property_id}/transitions` — `internal/property/handler.go`
- `POST   /v1/properties/{property_id}/transitions` — `internal/property/handler.go`

## recovery (1)

- `GET    /health/recovery` — `internal/recovery/recovery.go`

## reporting (12)

- `GET    /v1/reporting/approval-pipeline` — `internal/reporting/handler.go`
- `GET    /v1/reporting/inventory-summary` — `internal/reporting/handler.go`
- `GET    /v1/reporting/owner-exceptions` — `internal/reporting/handler.go`
- `GET    /v1/reporting/property-contribution` — `internal/reporting/handler.go`
- `GET    /v1/reporting/readiness` — `internal/reporting/handler.go`
- `GET    /v1/reporting/service-level-summary` — `internal/reporting/handler.go`
- `GET    /v1/reporting/snapshots` — `internal/reporting/handler.go`
- `POST   /v1/reporting/snapshots/rebuild` — `internal/reporting/handler.go`
- `POST   /v1/reporting/snapshots/verify` — `internal/reporting/handler.go`
- `GET    /v1/reporting/worker-metrics` — `internal/reporting/handler.go`
- `POST   /v1/reporting/worker-metrics` — `internal/reporting/handler.go`
- `GET    /v1/reporting/worker-metrics/summary` — `internal/reporting/handler.go`

## reservations (14)

- `POST   /v1/calendar-exceptions/{exception_id}/resolve` — `internal/reservations/handler.go`
- `POST   /v1/calendar-feeds/{feed_id}/polls` — `internal/reservations/handler.go`
- `PUT    /v1/calendar-feeds/{feed_id}/status` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/calendar-events` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/calendar-exceptions` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/calendar-feeds` — `internal/reservations/handler.go`
- `POST   /v1/properties/{property_id}/calendar-feeds` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/calendar-feeds/{feed_id}` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/calendar-health` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/reservation-conflicts` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/reservations` — `internal/reservations/handler.go`
- `GET    /v1/properties/{property_id}/turnover-proposals` — `internal/reservations/handler.go`
- `POST   /v1/properties/{property_id}/turnover-proposals/generate` — `internal/reservations/handler.go`
- `POST   /v1/reservation-conflicts/{conflict_id}/resolve` — `internal/reservations/handler.go`

## workforce (20)

- `POST   /v1/grievances` — `internal/workforce/handler.go`
- `POST   /v1/time-entries` — `internal/workforce/handler.go`
- `GET    /v1/workers` — `internal/workforce/handler.go`
- `POST   /v1/workers` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/adverse-actions` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}/availability-windows` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/availability-windows` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/certifications` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}/employment-terms` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/employment-terms` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}/expenses` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/expenses` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}/grievances` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/grievances` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/ratings` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/sos` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}/sos-events` — `internal/workforce/handler.go`
- `GET    /v1/workers/{worker_id}/time-entries` — `internal/workforce/handler.go`
- `POST   /v1/workers/{worker_id}/time-entries` — `internal/workforce/handler.go`
