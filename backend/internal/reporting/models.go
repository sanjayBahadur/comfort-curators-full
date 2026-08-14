package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSnapshotNotFound       = errors.New("report snapshot not found")
	ErrInvalidSnapshot        = errors.New("invalid report snapshot")
	ErrUnknownProjection      = errors.New("unknown reporting projection")
	ErrInvalidPeriod          = errors.New("invalid reporting period")
	ErrInvalidMetric          = errors.New("invalid worker metric observation")
	ErrMetricRankingDenied    = errors.New("worker metrics are never ranked automatically")
	ErrMetricDisciplineDenied = errors.New("worker metrics can never become discipline")
)

// Projection kinds. Every kind is a rebuildable read model computed from
// source transactions. The projection is derived data only: it is rebuilt
// from the source and never becomes transaction authority.
const (
	// ProjectionPropertyContribution is the FIN-011 property contribution
	// report (revenue, supply margin, vendor cost, refund, exception cost,
	// discount and tax).
	ProjectionPropertyContribution = "property_contribution"
	// ProjectionOwnerMonthlyReport is the owner-facing monthly report that
	// aggregates the contribution read model, owner-visible exceptions and a
	// short service-level summary for the period.
	ProjectionOwnerMonthlyReport = "owner_monthly_report"
)

// Period is an optional, half-open time window [Start, End) applied to the
// source transactions of a projection. A nil Period means all time.
type Period struct {
	Start time.Time
	End   time.Time
}

// Validate reports whether the period is well formed.
func (p *Period) Validate() error {
	if p == nil {
		return nil
	}
	if p.End.Before(p.Start) {
		return fmt.Errorf("%w: end %s precedes start %s", ErrInvalidPeriod, p.End.Format(time.RFC3339), p.Start.Format(time.RFC3339))
	}
	return nil
}

func periodPtr(t time.Time, ok bool) *time.Time {
	if !ok {
		return nil
	}
	return &t
}

// ReportSnapshot is a stored read-model projection. It carries the number of
// source rows it was built from and a deterministic hash of those rows, so a
// rebuild can be verified against the source transactions at any time.
type ReportSnapshot struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	PropertyID  string          `json:"property_id"`
	Kind        string          `json:"kind"`
	PeriodStart *time.Time      `json:"period_start,omitempty"`
	PeriodEnd   *time.Time      `json:"period_end,omitempty"`
	SourceCount int64           `json:"source_count"`
	SourceHash  string          `json:"source_hash"`
	Data        json.RawMessage `json:"data"`
	BuiltAt     time.Time       `json:"built_at"`
	Version     int64           `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
}

// SnapshotVerification is the result of comparing a stored snapshot with a
// fresh rebuild of the same projection from its source transactions.
type SnapshotVerification struct {
	SnapshotID     string `json:"snapshot_id"`
	Kind           string `json:"kind"`
	Match          bool   `json:"match"`
	ExpectedHash   string `json:"expected_hash"`
	ActualHash     string `json:"actual_hash"`
	MismatchReason string `json:"mismatch_reason,omitempty"`
}

// PropertyContribution is the property-level contribution read model
// (FIN-011). All amounts are integer minor units in a single ISO 4217
// currency. It is computed only from tenant-scoped source transactions, so
// the same source rows always produce the same report.
type PropertyContribution struct {
	RevenueMinorUnits         int64  `json:"revenue_minor_units"`
	SupplyMarginMinorUnits    int64  `json:"supply_margin_minor_units"`
	VendorCostMinorUnits      int64  `json:"vendor_cost_minor_units"`
	RefundMinorUnits          int64  `json:"refund_minor_units"`
	ExceptionCostMinorUnits   int64  `json:"exception_cost_minor_units"`
	DiscountMinorUnits        int64  `json:"discount_minor_units"`
	TaxMinorUnits             int64  `json:"tax_minor_units"`
	NetContributionMinorUnits int64  `json:"net_contribution_minor_units"`
	Currency                  string `json:"currency"`
}

// OwnerMonthlyReport is the owner-facing monthly report read model. It
// combines the property contribution with a service-level summary and the
// owner-visible exception feed. Internal operational noise is never included.
type OwnerMonthlyReport struct {
	PropertyID         string               `json:"property_id"`
	PeriodStart        *time.Time           `json:"period_start,omitempty"`
	PeriodEnd          *time.Time           `json:"period_end,omitempty"`
	Currency           string               `json:"currency"`
	Contribution       PropertyContribution `json:"contribution"`
	CompletedTickets   int                  `json:"completed_tickets"`
	OpenIncidents      int                  `json:"open_incidents"`
	OpenRecoveries     int                  `json:"open_recoveries"`
	InventoryMovements int                  `json:"inventory_movements"`
	OwnerExceptions    []OwnerException     `json:"owner_exceptions"`
}

// OwnerException is an exception the owner is entitled to see. Routine
// internal operational records (turnover, restock, stock counts, internal
// review, alert queues, scheduling) are filtered out before this type is
// produced, so the owner exception feed never carries internal noise.
type OwnerException struct {
	Source       string    `json:"source"`
	SourceID     string    `json:"source_id"`
	PropertyID   string    `json:"property_id"`
	Label        string    `json:"label"`
	Summary      string    `json:"summary"`
	Severity     string    `json:"severity,omitempty"`
	Status       string    `json:"status"`
	OccurredAt   time.Time `json:"occurred_at"`
	OwnerVisible bool      `json:"owner_visible"`
}

// Exception sources surfaced on the owner exception feed.
const (
	ExceptionSourceIncident        = "incident"
	ExceptionSourceServiceRecovery = "service_recovery"
	ExceptionSourceFinancial       = "financial"
)

// Values mirrored from the source modules' schemas. The reporting module
// classifies source rows read by SQL, so these strings must match the values
// written by the owning modules.
const (
	ticketStatusClosed    = "closed"
	ticketStatusCancelled = "cancelled"
	recoveryStatusOpen    = "open"
	exceptionStatusOpen   = "open"
	incidentTicketType    = "incident"
)

// ExceptionClass is the visibility decision for a source record on the owner
// exception feed.
type ExceptionClass struct {
	OwnerVisible bool
	Label        string
}

// ClassifyTicketException decides whether a ticket is owner-visible. Only
// active incident tickets surface as owner exceptions; routine operational
// work (turnover, restock, counts, internal review, onboarding checks,
// routine maintenance, specialist vendor requests) is internal noise.
func ClassifyTicketException(ticketType, status string) ExceptionClass {
	if ticketType == incidentTicketType && status != ticketStatusClosed && status != ticketStatusCancelled {
		return ExceptionClass{OwnerVisible: true, Label: ExceptionSourceIncident}
	}
	return ExceptionClass{OwnerVisible: false}
}

// ClassifyRecoveryException decides whether a service recovery is
// owner-visible. Only open recoveries (an unresolved guest-impacting failure)
// are promoted.
func ClassifyRecoveryException(status string) ExceptionClass {
	if status == recoveryStatusOpen {
		return ExceptionClass{OwnerVisible: true, Label: ExceptionSourceServiceRecovery}
	}
	return ExceptionClass{OwnerVisible: false}
}

// ClassifyFinancialException decides whether a financial exception is
// owner-visible. Only open reconciliation exceptions are promoted.
func ClassifyFinancialException(status string) ExceptionClass {
	if status == exceptionStatusOpen {
		return ExceptionClass{OwnerVisible: true, Label: ExceptionSourceFinancial}
	}
	return ExceptionClass{OwnerVisible: false}
}

// MetricObservation is an append-only worker development metric (for example
// turnover time or resolution time). It is explicitly non-ranking and
// non-disciplinary: it never carries an automatic rank and can never be bound
// to a discipline decision.
type MetricObservation struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	PropertyID  string     `json:"property_id"`
	WorkerID    string     `json:"worker_id"`
	MetricKind  string     `json:"metric_kind"`
	Value       int64      `json:"value"`
	Unit        string     `json:"unit"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty"`
	SourceRef   string     `json:"source_ref"`
	RecordedBy  string     `json:"recorded_by"`
	RecordedAt  time.Time  `json:"recorded_at"`
}

// HasRank reports whether the observation carries an automatic rank. Worker
// metrics are never ranked, so this is always false.
func (m MetricObservation) HasRank() bool { return false }

// CanDiscipline reports whether the observation is bound to a discipline
// decision. Worker metrics never become discipline, so this is always false.
func (m MetricObservation) CanDiscipline() bool { return false }

// Metric kinds supported as development feedback. These are operational job
// execution facts, not performance scores.
const (
	MetricKindTurnoverTimeMinutes     = "turnover_time_minutes"
	MetricKindResolutionTimeMinutes   = "resolution_time_minutes"
	MetricKindReworkCount             = "rework_count"
	MetricKindGuestImpactingIncidents = "guest_impacting_incidents"
)

var validMetricKinds = map[string]bool{
	MetricKindTurnoverTimeMinutes:     true,
	MetricKindResolutionTimeMinutes:   true,
	MetricKindReworkCount:             true,
	MetricKindGuestImpactingIncidents: true,
}

// ValidMetricKind reports whether k is a supported worker metric kind.
func ValidMetricKind(k string) bool {
	return validMetricKinds[k]
}

// GuardMetricsNonDisciplinary is applied to every worker metric read path. It
// fails closed if any observation ever carries an automatic rank or a
// discipline binding, so a worker metric can never silently become ranking or
// discipline downstream.
func GuardMetricsNonDisciplinary(observations []MetricObservation) error {
	for _, o := range observations {
		if o.HasRank() {
			return fmt.Errorf("%w: observation %s carries an automatic rank", ErrMetricRankingDenied, o.ID)
		}
		if o.CanDiscipline() {
			return fmt.Errorf("%w: observation %s is bound to a discipline decision", ErrMetricDisciplineDenied, o.ID)
		}
	}
	return nil
}

// Additional projection kinds covering the parent description's read models.
const (
	ProjectionReadiness           = "property_readiness"
	ProjectionServiceLevelSummary = "service_level_summary"
	ProjectionInventorySummary    = "inventory_summary"
	ProjectionApprovalPipeline    = "approval_pipeline"
	ProjectionDocumentStatus      = "document_status"
	ProjectionLaborTravelSummary  = "labor_travel_summary"
)

// PropertyReadiness is the property readiness read model. It shows active
// compliance holds, onboarding status, and any blocking conditions.
type PropertyReadiness struct {
	PropertyID            string `json:"property_id"`
	ActiveComplianceHolds int    `json:"active_compliance_holds"`
	PendingRenewals       int    `json:"pending_renewals"`
	OnboardingStatus      string `json:"onboarding_status"`
	HasActivationBlocker  bool   `json:"has_activation_blocker"`
}

// ServiceLevelSummary aggregates ticket-level service metrics for a property
// without ranking individual workers. It is a rebuildable read model.
type ServiceLevelSummary struct {
	PropertyID          string `json:"property_id"`
	TotalTickets        int    `json:"total_tickets"`
	ClosedTickets       int    `json:"closed_tickets"`
	OpenTickets         int    `json:"open_tickets"`
	OpenIncidents       int    `json:"open_incidents"`
	CancelledTickets    int    `json:"cancelled_tickets"`
	OpenRecoveries      int    `json:"open_recoveries"`
	CompletedChecklists int    `json:"completed_checklists"`
}

// InventorySummary is the property inventory read model. It aggregates
// movement and count data from the append-only inventory ledger.
type InventorySummary struct {
	PropertyID         string `json:"property_id"`
	StockLocationCount int    `json:"stock_location_count"`
	TotalMovements     int    `json:"total_movements"`
	ConsumedQuantity   int64  `json:"consumed_quantity"`
	AdjustmentCount    int    `json:"adjustment_count"`
	PendingCounts      int    `json:"pending_counts"`
}

// ApprovalPipeline is the maker-checker approval pipeline read model. It
// shows pending approvals, rejected submissions, and pending verifications
// for a property.
type ApprovalPipeline struct {
	PropertyID           string `json:"property_id"`
	PendingApprovals     int    `json:"pending_approvals"`
	PendingSubmissions   int    `json:"pending_submissions"`
	RejectedSubmissions  int    `json:"rejected_submissions"`
	PendingVerifications int    `json:"pending_verifications"`
	DraftRequests        int    `json:"draft_requests"`
}

// DocumentStatus is the document completion read model.
type DocumentStatus struct {
	PropertyID       string `json:"property_id"`
	TotalDocuments   int    `json:"total_documents"`
	ExpiredDocuments int    `json:"expired_documents"`
	PendingReviews   int    `json:"pending_reviews"`
	CompletedPackets int    `json:"completed_packets"`
}

// LaborTravelSummary aggregates workforce labor and travel data.
type LaborTravelSummary struct {
	PropertyID      string `json:"property_id"`
	TotalWorkMins   int64  `json:"total_work_minutes"`
	TotalTravel     int64  `json:"total_travel_minutes"`
	OvertimeCount   int    `json:"overtime_count"`
	DistinctWorkers int    `json:"distinct_workers"`
	TotalExpenses   int64  `json:"total_expenses_minor_units"`
}

// MetricSummary is an aggregate of worker metrics without any rank position.
// Worker metrics are development feedback and never become a leaderboard.
type MetricSummary struct {
	WorkerID   string `json:"worker_id"`
	MetricKind string `json:"metric_kind"`
	Count      int    `json:"count"`
	Sum        int64  `json:"sum"`
	Average    int64  `json:"average"`
	Minimum    int64  `json:"minimum"`
	Maximum    int64  `json:"maximum"`
}
