package reporting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Source row projections. The reporting module reads tenant-scoped rows from
// the owning modules' tables by SQL and folds them into deterministic read
// models. It never writes to those tables.

// ChargeSourceRow is the slice of the charges table a projection needs.
type ChargeSourceRow struct {
	ID               string
	ChargeType       string
	Status           string
	AmountMinorUnits int64
	Currency         string
}

// CreditSourceRow is the slice of the credits table a projection needs.
type CreditSourceRow struct {
	ID               string
	CreditType       string
	Status           string
	AmountMinorUnits int64
	Currency         string
}

// RecoverySourceRow is the slice of the service_recoveries table a projection
// needs. Rework cost is the avoidable cost preserved by service recovery.
type RecoverySourceRow struct {
	ID              string
	Status          string
	ReworkCostMinor int64
	Currency        string
}

// TicketSourceRow is the slice of the tickets table a projection needs.
type TicketSourceRow struct {
	ID         string
	PropertyID string
	Type       string
	Status     string
	Severity   string
	Reason     string
	CreatedAt  time.Time
}

// ComplianceSourceRow is the slice of the compliance_items table a projection needs.
type ComplianceSourceRow struct {
	ID         string
	PropertyID string
	Severity   string
	Status     string
	ExpiryDate time.Time
}

// OnboardingSourceRow is the slice of the onboarding_cases table a projection needs.
type OnboardingSourceRow struct {
	ID         string
	PropertyID string
	Status     string
}

// StockLocationSourceRow is the slice of the stock_locations table a projection needs.
type StockLocationSourceRow struct {
	ID           string
	PropertyID   string
	LocationType string
}

// InventoryMovementAggRow is an aggregated row from inventory_movements.
type InventoryMovementAggRow struct {
	LocationID   string
	MovementType string
	Quantity     int64
	CreatedAt    time.Time
}

// CountRow is a simple aggregated count row.
type CountRow struct {
	Status string
	Count  int
}

// ApprovalSourceRow is the slice of the maker_checker_requests table a projection needs.
type ApprovalSourceRow struct {
	ID         string
	PropertyID string
	Status     string
}

// DocumentSourceRow is the slice of the documents table a projection needs.
type DocumentSourceRow struct {
	ID         string
	PropertyID string
	Status     string
	ExpiresAt  *time.Time
}

// TimeEntryAggRow is an aggregated row from time_entries.
type TimeEntryAggRow struct {
	WorkerID      string
	WorkMinutes   int64
	TravelMinutes int64
	Overtime      int
}

// ExpenseAggRow is an aggregated row from expenses.
type ExpenseAggRow struct {
	WorkerID        string
	TotalMinorUnits int64
}

// Charge/credit statuses mirrored from the billing module's schema. Only
// applied charges and issued/applied credits are effective for reporting.
const (
	chargeStatusApplied = "applied"
	creditStatusIssued  = "issued"
)

// Contribution charge-type categories mirrored from the billing module.
const (
	chargeTypeManagementFee  = "management_fee"
	chargeTypeTaskService    = "task_service"
	chargeTypePurchasedGoods = "purchased_goods"
	chargeTypeReimbursement  = "reimbursement"
	chargeTypeVendorFee      = "vendor_fee"
	chargeTypeDiscount       = "discount"
	chargeTypeRebate         = "rebate"
	chargeTypeTax            = "tax"
	chargeTypeRefund         = "refund"
	creditTypeRefund         = "refund"
	creditTypeDiscount       = "discount"
	creditTypeReversal       = "reversal"
	creditTypeCreditNote     = "credit_note"
)

// AggregateContribution folds tenant-scoped source rows into a property
// contribution report. It is a pure function: identical inputs always produce
// an identical report, which is what makes the projection rebuildable.
//
// Category mapping (all in integer minor units):
//   - revenue: applied management fee and task service charges;
//   - supply margin: applied purchased-goods charges minus rebates;
//   - vendor cost: applied vendor fees and reimbursements;
//   - refund: applied refund charges plus issued refund credits;
//   - discount: applied discount charges plus issued discount credits;
//   - exception cost: rework cost preserved by open service recoveries;
//   - tax: applied tax charges, shown separately and excluded from the net.
//
// Corrections are never applied in place: a corrected charge or credit keeps
// its original row (its status is no longer applied), so the original entry
// remains preserved and the projection simply stops counting it.
func AggregateContribution(charges []ChargeSourceRow, credits []CreditSourceRow, recoveries []RecoverySourceRow) PropertyContribution {
	var pc PropertyContribution
	pc.Currency = "INR"

	for _, c := range charges {
		if c.Status != chargeStatusApplied {
			continue
		}
		switch c.ChargeType {
		case chargeTypeManagementFee, chargeTypeTaskService:
			pc.RevenueMinorUnits += c.AmountMinorUnits
		case chargeTypePurchasedGoods:
			pc.SupplyMarginMinorUnits += c.AmountMinorUnits
		case chargeTypeRebate:
			pc.SupplyMarginMinorUnits -= c.AmountMinorUnits
		case chargeTypeVendorFee, chargeTypeReimbursement:
			pc.VendorCostMinorUnits += c.AmountMinorUnits
		case chargeTypeRefund:
			pc.RefundMinorUnits += c.AmountMinorUnits
		case chargeTypeDiscount:
			pc.DiscountMinorUnits += c.AmountMinorUnits
		case chargeTypeTax:
			pc.TaxMinorUnits += c.AmountMinorUnits
		}
	}

	for _, cr := range credits {
		if cr.Status != creditStatusIssued {
			continue
		}
		switch cr.CreditType {
		case creditTypeRefund:
			pc.RefundMinorUnits += cr.AmountMinorUnits
		case creditTypeDiscount:
			pc.DiscountMinorUnits += cr.AmountMinorUnits
		case creditTypeReversal, creditTypeCreditNote:
			pc.RevenueMinorUnits -= cr.AmountMinorUnits
		}
	}

	for _, r := range recoveries {
		if r.Status == recoveryStatusOpen {
			pc.ExceptionCostMinorUnits += r.ReworkCostMinor
		}
	}

	pc.NetContributionMinorUnits = pc.RevenueMinorUnits +
		pc.SupplyMarginMinorUnits -
		pc.VendorCostMinorUnits -
		pc.RefundMinorUnits -
		pc.DiscountMinorUnits -
		pc.ExceptionCostMinorUnits

	return pc
}

func encodeContributionHash(charges []ChargeSourceRow, credits []CreditSourceRow, recoveries []RecoverySourceRow) string {
	h := sha256.New()
	for _, c := range charges {
		fmt.Fprintf(h, "charge|%s|%s|%s|%d|%s\n", c.ID, c.ChargeType, c.Status, c.AmountMinorUnits, c.Currency)
	}
	for _, cr := range credits {
		fmt.Fprintf(h, "credit|%s|%s|%s|%d|%s\n", cr.ID, cr.CreditType, cr.Status, cr.AmountMinorUnits, cr.Currency)
	}
	for _, r := range recoveries {
		fmt.Fprintf(h, "recovery|%s|%s|%d|%s\n", r.ID, r.Status, r.ReworkCostMinor, r.Currency)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func encodeTicketHash(t TicketSourceRow) string {
	return fmt.Sprintf("ticket|%s|%s|%s|%s\n", t.ID, t.Type, t.Status, t.Severity)
}

func hashSource(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// buildPropertyContribution recomputes the contribution read model from the
// current source transactions for a tenant, property and optional period. It
// returns the report plus the number and deterministic hash of the source
// rows used, so a snapshot can later be verified against the source.
func (s *ReportingService) buildPropertyContribution(ctx context.Context, tenantID, propertyID string, period *Period) (*PropertyContribution, int64, string, error) {
	var start, end *time.Time
	if period != nil {
		start, end = &period.Start, &period.End
	}

	charges, err := s.store.ListChargesForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	credits, err := s.store.ListCreditsForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	recoveries, err := s.store.ListRecoveriesForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}

	pc := AggregateContribution(charges, credits, recoveries)
	count := int64(len(charges) + len(credits) + len(recoveries))
	hash := encodeContributionHash(charges, credits, recoveries)
	return &pc, count, hash, nil
}

// buildOwnerMonthlyReport recomputes the owner-facing monthly report from the
// current source transactions. It aggregates the contribution read model, a
// short service-level summary (completed tickets, open incidents, open
// recoveries, inventory movements) and the owner-visible exception feed.
func (s *ReportingService) buildOwnerMonthlyReport(ctx context.Context, tenantID, propertyID string, period *Period) (*OwnerMonthlyReport, int64, string, error) {
	var start, end *time.Time
	if period != nil {
		start, end = &period.Start, &period.End
	}

	pc, count, hash, err := s.buildPropertyContribution(ctx, tenantID, propertyID, period)
	if err != nil {
		return nil, 0, "", err
	}

	completed, err := s.store.CountClosedTicketsForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	incidents, incidentRows, err := s.store.ListIncidentTicketsForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	recoveries, err := s.store.CountOpenRecoveriesForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	movements, err := s.store.CountInventoryMovementsForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	exceptions, err := s.store.ListOwnerExceptionRows(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}

	ownerExceptions := make([]OwnerException, 0, len(exceptions))
	for _, ex := range exceptions {
		if ex.OwnerVisible {
			ownerExceptions = append(ownerExceptions, ex)
		}
	}

	rpt := &OwnerMonthlyReport{
		PropertyID:         propertyID,
		PeriodStart:        periodPtr(periodStart(period), period != nil),
		PeriodEnd:          periodPtr(periodEnd(period), period != nil),
		Currency:           pc.Currency,
		Contribution:       *pc,
		CompletedTickets:   completed,
		OpenIncidents:      incidents,
		OpenRecoveries:     recoveries,
		InventoryMovements: movements,
		OwnerExceptions:    ownerExceptions,
	}

	reportCount := count + int64(completed) + int64(len(incidentRows)) + int64(recoveries) + int64(movements) + int64(len(exceptions))
	hashParts := []string{hash}
	for _, t := range incidentRows {
		hashParts = append(hashParts, encodeTicketHash(t))
	}
	hashParts = append(hashParts,
		fmt.Sprintf("closed_tickets|%d\n", completed),
		fmt.Sprintf("open_recoveries|%d\n", recoveries),
		fmt.Sprintf("inventory_movements|%d\n", movements),
	)
	for _, ex := range exceptions {
		hashParts = append(hashParts, fmt.Sprintf("exception|%s|%s|%s|%t\n", ex.Source, ex.SourceID, ex.Status, ex.OwnerVisible))
	}
	reportHash := hashSource(hashParts...)

	return rpt, reportCount, reportHash, nil
}

func periodStart(p *Period) time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.Start
}

func periodEnd(p *Period) time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.End
}

// buildReadiness compiles the property readiness projection.
func (s *ReportingService) buildReadiness(ctx context.Context, tenantID, propertyID string) (*PropertyReadiness, int64, string, error) {
	holds, err := s.store.CountComplianceHolds(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	renewals, err := s.store.CountPendingRenewals(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	onboarding, err := s.store.GetOnboardingStatus(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}

	r := &PropertyReadiness{
		PropertyID:            propertyID,
		ActiveComplianceHolds: holds,
		PendingRenewals:       renewals,
		OnboardingStatus:      onboarding,
		HasActivationBlocker:  holds > 0 || onboarding == "blocked",
	}

	count := int64(holds) + int64(renewals) + 1
	h := hashSource(
		fmt.Sprintf("holds|%d", holds),
		fmt.Sprintf("renewals|%d", renewals),
		fmt.Sprintf("onboarding|%s", onboarding),
	)

	return r, count, h, nil
}

// buildServiceLevelSummary compiles the service level projection.
func (s *ReportingService) buildServiceLevelSummary(ctx context.Context, tenantID, propertyID string, period *Period) (*ServiceLevelSummary, int64, string, error) {
	var start, end *time.Time
	if period != nil {
		start, end = &period.Start, &period.End
	}

	total, err := s.store.CountTicketsByStatus(ctx, tenantID, propertyID, start, end, "")
	if err != nil {
		return nil, 0, "", err
	}
	closed, err := s.store.CountTicketsByStatus(ctx, tenantID, propertyID, start, end, "closed")
	if err != nil {
		return nil, 0, "", err
	}
	open, err := s.store.CountTicketsByStatus(ctx, tenantID, propertyID, start, end, "open")
	if err != nil {
		return nil, 0, "", err
	}
	incidents, err := s.store.CountTicketsByStatus(ctx, tenantID, propertyID, start, end, "in_progress")
	if err != nil {
		return nil, 0, "", err
	}
	// Count open incident tickets specifically.
	openIncidents, _, err := s.store.ListIncidentTicketsForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	cancelled, err := s.store.CountTicketsByStatus(ctx, tenantID, propertyID, start, end, "cancelled")
	if err != nil {
		return nil, 0, "", err
	}
	openRecoveries, err := s.store.CountOpenRecoveriesForReport(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}
	checklists, err := s.store.CountCompletedChecklistItems(ctx, tenantID, propertyID, start, end)
	if err != nil {
		return nil, 0, "", err
	}

	sls := &ServiceLevelSummary{
		PropertyID:          propertyID,
		TotalTickets:        total,
		ClosedTickets:       closed,
		OpenTickets:         open,
		OpenIncidents:       openIncidents + incidents,
		CancelledTickets:    cancelled,
		OpenRecoveries:      openRecoveries,
		CompletedChecklists: checklists,
	}

	count := int64(total + closed + open + openIncidents + cancelled + openRecoveries + checklists)
	h := hashSource(
		fmt.Sprintf("total|%d", total),
		fmt.Sprintf("closed|%d", closed),
		fmt.Sprintf("open|%d", open),
		fmt.Sprintf("incidents|%d", openIncidents),
		fmt.Sprintf("cancelled|%d", cancelled),
		fmt.Sprintf("recoveries|%d", openRecoveries),
		fmt.Sprintf("checklists|%d", checklists),
	)

	return sls, count, h, nil
}

// buildInventorySummary compiles the inventory read model.
func (s *ReportingService) buildInventorySummary(ctx context.Context, tenantID, propertyID string) (*InventorySummary, int64, string, error) {
	locations, err := s.store.CountStockLocations(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	movements, consumed, err := s.store.CountInventoryMovementsDetailed(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	adjustments, err := s.store.CountAdjustments(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	pendingCounts, err := s.store.CountPendingInventoryCounts(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}

	is := &InventorySummary{
		PropertyID:         propertyID,
		StockLocationCount: locations,
		TotalMovements:     movements,
		ConsumedQuantity:   consumed,
		AdjustmentCount:    adjustments,
		PendingCounts:      pendingCounts,
	}

	count := int64(locations + movements + adjustments + pendingCounts)
	h := hashSource(
		fmt.Sprintf("locations|%d", locations),
		fmt.Sprintf("movements|%d", movements),
		fmt.Sprintf("consumed|%d", consumed),
		fmt.Sprintf("adjustments|%d", adjustments),
		fmt.Sprintf("pending|%d", pendingCounts),
	)

	return is, count, h, nil
}

// buildApprovalPipeline compiles the approval pipeline read model.
func (s *ReportingService) buildApprovalPipeline(ctx context.Context, tenantID, propertyID string) (*ApprovalPipeline, int64, string, error) {
	statuses, err := s.store.CountMakerCheckerRequests(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}

	ap := &ApprovalPipeline{
		PropertyID:           propertyID,
		PendingApprovals:     statuses["pending_approval"],
		PendingSubmissions:   statuses["submitted"],
		RejectedSubmissions:  statuses["rejected"],
		PendingVerifications: statuses["pending_verification"],
		DraftRequests:        statuses["draft"],
	}

	count := int64(ap.PendingApprovals + ap.PendingSubmissions + ap.RejectedSubmissions + ap.PendingVerifications + ap.DraftRequests)
	h := hashSource(
		fmt.Sprintf("pending_approval|%d", ap.PendingApprovals),
		fmt.Sprintf("submitted|%d", ap.PendingSubmissions),
		fmt.Sprintf("rejected|%d", ap.RejectedSubmissions),
		fmt.Sprintf("verification|%d", ap.PendingVerifications),
		fmt.Sprintf("draft|%d", ap.DraftRequests),
	)

	return ap, count, h, nil
}

// buildDocumentStatus compiles the document status read model.
func (s *ReportingService) buildDocumentStatus(ctx context.Context, tenantID, propertyID string) (*DocumentStatus, int64, string, error) {
	total, err := s.store.CountDocuments(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	expired, err := s.store.CountExpiredDocuments(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	reviews, err := s.store.CountPendingDocumentReviews(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	packets, err := s.store.CountCompletedSubmissionPackets(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}

	ds := &DocumentStatus{
		PropertyID:       propertyID,
		TotalDocuments:   total,
		ExpiredDocuments: expired,
		PendingReviews:   reviews,
		CompletedPackets: packets,
	}

	count := int64(total + expired + reviews + packets)
	h := hashSource(
		fmt.Sprintf("total|%d", total),
		fmt.Sprintf("expired|%d", expired),
		fmt.Sprintf("reviews|%d", reviews),
		fmt.Sprintf("packets|%d", packets),
	)

	return ds, count, h, nil
}

// buildLaborTravelSummary compiles the workforce labor and travel read model.
func (s *ReportingService) buildLaborTravelSummary(ctx context.Context, tenantID, propertyID string) (*LaborTravelSummary, int64, string, error) {
	workMins, travelMins, overtime, workers, err := s.store.AggregateTimeEntries(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}
	expenses, err := s.store.AggregateExpenses(ctx, tenantID, propertyID)
	if err != nil {
		return nil, 0, "", err
	}

	lts := &LaborTravelSummary{
		PropertyID:      propertyID,
		TotalWorkMins:   workMins,
		TotalTravel:     travelMins,
		OvertimeCount:   overtime,
		DistinctWorkers: workers,
		TotalExpenses:   expenses,
	}

	count := int64(workMins) + int64(travelMins) + int64(overtime) + int64(workers) + int64(expenses)
	h := hashSource(
		fmt.Sprintf("work|%d", workMins),
		fmt.Sprintf("travel|%d", travelMins),
		fmt.Sprintf("overtime|%d", overtime),
		fmt.Sprintf("workers|%d", workers),
		fmt.Sprintf("expenses|%d", expenses),
	)

	return lts, count, h, nil
}
