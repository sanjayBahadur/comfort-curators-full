package api

import (
	"time"

	"comfort-curators-backend/internal/billing"
)

// This file defines the financial evidence traceability views of the protected
// owner-finance API slice. FIN-003 requires every invoice line to link to a
// contract rule, ticket, order, or approved manual adjustment, and FIN-010
// requires financial corrections to preserve the original entry. The views
// below expose those links so every owner-visible amount is traceable to its
// source. Money is always integer minor units with an ISO 4217 currency; guest
// purchases and booking payout custody are never represented.

// EvidenceLink is one traceable link between a financial record and its
// supporting evidence.
type EvidenceLink struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ChargeEvidenceLinks returns the traceable evidence links of a charge. A
// charge may link to a contract rule, the operational evidence that proved the
// work, the ticket, the order, and the approval that authorized the charge.
func ChargeEvidenceLinks(c billing.Charge) []EvidenceLink {
	var links []EvidenceLink
	if c.ContractRuleID != "" {
		links = append(links, EvidenceLink{Kind: "contract_rule", ID: c.ContractRuleID})
	}
	if c.EvidenceID != "" {
		links = append(links, EvidenceLink{Kind: "evidence", ID: c.EvidenceID})
	}
	if c.TicketID != "" {
		links = append(links, EvidenceLink{Kind: "ticket", ID: c.TicketID})
	}
	if c.OrderID != "" {
		links = append(links, EvidenceLink{Kind: "order", ID: c.OrderID})
	}
	if c.ApprovalID != "" {
		links = append(links, EvidenceLink{Kind: "approval", ID: c.ApprovalID})
	}
	return links
}

// InvoiceLineEvidenceLinks returns the traceable evidence links of an invoice
// line. Every line links to a contract rule, ticket, order, or approved manual
// adjustment as FIN-003 requires.
func InvoiceLineEvidenceLinks(l billing.InvoiceLine) []EvidenceLink {
	var links []EvidenceLink
	if l.ContractRuleID != "" {
		links = append(links, EvidenceLink{Kind: "contract_rule", ID: l.ContractRuleID})
	}
	if l.TicketID != "" {
		links = append(links, EvidenceLink{Kind: "ticket", ID: l.TicketID})
	}
	if l.OrderID != "" {
		links = append(links, EvidenceLink{Kind: "order", ID: l.OrderID})
	}
	if l.AdjustmentID != "" {
		links = append(links, EvidenceLink{Kind: "approved_adjustment", ID: l.AdjustmentID})
	}
	return links
}

// SubledgerTrace returns the source reference linking an operational subledger
// entry back to the financial record that produced it.
func SubledgerTrace(e billing.OperationalSubledgerEntry) EvidenceLink {
	return EvidenceLink{Kind: e.ReferenceType, ID: e.ReferenceID}
}

// CreditTrace returns the original entry that a credit preserves. Financial
// corrections never delete the original entry; the trace makes the correction
// link back to it (FIN-010).
func CreditTrace(c billing.Credit) EvidenceLink {
	return EvidenceLink{Kind: c.OriginalEntryType, ID: c.OriginalEntryID}
}

// ChargeTraceabilityView renders a charge with its evidence chain for the
// owner-finance slice. The view carries the integer minor-unit amount, the
// charge class, status and resource version, plus the traceable evidence
// links. It never contains worker HR material.
func ChargeTraceabilityView(c billing.Charge) map[string]any {
	return map[string]any{
		"id":                 c.ID,
		"tenant_id":          c.TenantID,
		"property_id":        c.PropertyID,
		"charge_type":        c.ChargeType,
		"amount_minor_units": c.AmountMinorUnits,
		"currency":           c.Currency,
		"status":             c.Status,
		"version":            c.Version,
		"evidence_links":     ChargeEvidenceLinks(c),
		"created_at":         c.CreatedAt.Format(time.RFC3339),
	}
}

// InvoiceTraceabilityView renders an invoice with its line evidence chain. The
// view carries the integer minor-unit total, the invoice status, the resource
// version and each line's evidence links. It never contains worker HR material.
func InvoiceTraceabilityView(inv billing.Invoice, lines []billing.InvoiceLine) map[string]any {
	lineViews := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		lineViews = append(lineViews, map[string]any{
			"id":                 l.ID,
			"charge_type":        l.ChargeType,
			"description":        l.Description,
			"amount_minor_units": l.AmountMinorUnits,
			"currency":           l.Currency,
			"evidence_links":     InvoiceLineEvidenceLinks(l),
		})
	}
	return map[string]any{
		"id":                inv.ID,
		"tenant_id":         inv.TenantID,
		"property_id":       inv.PropertyID,
		"total_minor_units": inv.TotalMinorUnits,
		"currency":          inv.Currency,
		"status":            inv.Status,
		"version":           inv.Version,
		"lines":             lineViews,
	}
}
