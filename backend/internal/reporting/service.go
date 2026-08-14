package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ReportingService struct {
	pool       *pgxpool.Pool
	store      *ReportingStore
	auditStore *audit.AuditStore
}

func NewReportingService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *ReportingService {
	return &ReportingService{
		pool:       pool,
		store:      NewReportingStore(pool),
		auditStore: auditStore,
	}
}

// RebuildParams selects a projection to rebuild.
type RebuildParams struct {
	Kind       string
	PropertyID string
	Period     *Period
}

// RebuildSnapshot recomputes a projection from its source transactions and
// stores the resulting snapshot. Rebuilding is idempotent: the same source
// rows always yield the same snapshot, and any later source row changes the
// snapshot on the next rebuild.
func (s *ReportingService) RebuildSnapshot(ctx context.Context, tenantID string, params RebuildParams) (*ReportSnapshot, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	if err := params.Period.Validate(); err != nil {
		return nil, err
	}

	var (
		data        []byte
		sourceCount int64
		sourceHash  string
	)
	switch params.Kind {
	case ProjectionPropertyContribution:
		pc, count, hash, err := s.buildPropertyContribution(ctx, tenantID, params.PropertyID, params.Period)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(pc), count, hash
	case ProjectionOwnerMonthlyReport:
		rpt, count, hash, err := s.buildOwnerMonthlyReport(ctx, tenantID, params.PropertyID, params.Period)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(rpt), count, hash
	case ProjectionReadiness:
		r, count, hash, err := s.buildReadiness(ctx, tenantID, params.PropertyID)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(r), count, hash
	case ProjectionServiceLevelSummary:
		sls, count, hash, err := s.buildServiceLevelSummary(ctx, tenantID, params.PropertyID, params.Period)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(sls), count, hash
	case ProjectionInventorySummary:
		is, count, hash, err := s.buildInventorySummary(ctx, tenantID, params.PropertyID)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(is), count, hash
	case ProjectionApprovalPipeline:
		ap, count, hash, err := s.buildApprovalPipeline(ctx, tenantID, params.PropertyID)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(ap), count, hash
	case ProjectionDocumentStatus:
		ds, count, hash, err := s.buildDocumentStatus(ctx, tenantID, params.PropertyID)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(ds), count, hash
	case ProjectionLaborTravelSummary:
		lts, count, hash, err := s.buildLaborTravelSummary(ctx, tenantID, params.PropertyID)
		if err != nil {
			return nil, err
		}
		data, sourceCount, sourceHash = mustMarshal(lts), count, hash
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProjection, params.Kind)
	}

	snap := &ReportSnapshot{
		TenantID:    tenantID,
		PropertyID:  params.PropertyID,
		Kind:        params.Kind,
		PeriodStart: periodPtr(periodStart(params.Period), params.Period != nil),
		PeriodEnd:   periodPtr(periodEnd(params.Period), params.Period != nil),
		SourceCount: sourceCount,
		SourceHash:  sourceHash,
		Data:        data,
		BuiltAt:     time.Now().UTC(),
	}
	if err := s.store.UpsertSnapshot(ctx, snap); err != nil {
		return nil, err
	}

	s.appendAudit(ctx, audit.AuditEvent{
		EventType:    audit.EventTypeSystem,
		TenantID:     tenantID,
		ActorID:      "system",
		Action:       "reporting.snapshot.rebuilt",
		ResourceType: "report_snapshot",
		ResourceID:   snap.ID,
		Metadata:     mustJSON(map[string]any{"kind": snap.Kind, "property_id": snap.PropertyID}),
	})

	return snap, nil
}

// VerifySnapshot compares a stored snapshot with a fresh rebuild of the same
// projection from the current source transactions. The snapshot matches the
// source when its source hash, source count and payload all equal the fresh
// rebuild; otherwise the projection is stale and must be rebuilt.
func (s *ReportingService) VerifySnapshot(ctx context.Context, tenantID, snapshotID string) (*SnapshotVerification, error) {
	snap, err := s.store.GetSnapshot(ctx, tenantID, snapshotID)
	if err != nil {
		return nil, err
	}

	var period *Period
	if snap.PeriodStart != nil || snap.PeriodEnd != nil {
		period = &Period{}
		if snap.PeriodStart != nil {
			period.Start = *snap.PeriodStart
		}
		if snap.PeriodEnd != nil {
			period.End = *snap.PeriodEnd
		}
	}

	var (
		fresh       []byte
		sourceCount int64
		sourceHash  string
	)
	switch snap.Kind {
	case ProjectionPropertyContribution:
		pc, count, hash, err := s.buildPropertyContribution(ctx, tenantID, snap.PropertyID, period)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(pc), count, hash
	case ProjectionOwnerMonthlyReport:
		rpt, count, hash, err := s.buildOwnerMonthlyReport(ctx, tenantID, snap.PropertyID, period)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(rpt), count, hash
	case ProjectionReadiness:
		r, count, hash, err := s.buildReadiness(ctx, tenantID, snap.PropertyID)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(r), count, hash
	case ProjectionServiceLevelSummary:
		sls, count, hash, err := s.buildServiceLevelSummary(ctx, tenantID, snap.PropertyID, period)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(sls), count, hash
	case ProjectionInventorySummary:
		is, count, hash, err := s.buildInventorySummary(ctx, tenantID, snap.PropertyID)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(is), count, hash
	case ProjectionApprovalPipeline:
		ap, count, hash, err := s.buildApprovalPipeline(ctx, tenantID, snap.PropertyID)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(ap), count, hash
	case ProjectionDocumentStatus:
		ds, count, hash, err := s.buildDocumentStatus(ctx, tenantID, snap.PropertyID)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(ds), count, hash
	case ProjectionLaborTravelSummary:
		lts, count, hash, err := s.buildLaborTravelSummary(ctx, tenantID, snap.PropertyID)
		if err != nil {
			return nil, err
		}
		fresh, sourceCount, sourceHash = mustMarshal(lts), count, hash
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProjection, snap.Kind)
	}

	verification := &SnapshotVerification{
		SnapshotID:   snap.ID,
		Kind:         snap.Kind,
		ExpectedHash: snap.SourceHash,
		ActualHash:   sourceHash,
	}

	if sourceCount != snap.SourceCount || sourceHash != snap.SourceHash {
		verification.Match = false
		verification.MismatchReason = fmt.Sprintf(
			"projection is stale: snapshot was built from %d source rows (hash %s), source now has %d rows (hash %s)",
			snap.SourceCount, shortHash(snap.SourceHash), sourceCount, shortHash(sourceHash))
		return verification, nil
	}

	if !jsonEqual(snap.Data, fresh) {
		verification.Match = false
		verification.MismatchReason = "snapshot payload differs from a fresh rebuild of the same source rows"
		return verification, nil
	}

	verification.Match = true
	return verification, nil
}

// PropertyContribution computes the live property contribution read model
// directly from the source transactions without storing a snapshot.
func (s *ReportingService) PropertyContribution(ctx context.Context, tenantID, propertyID string, period *Period) (*PropertyContribution, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	if err := period.Validate(); err != nil {
		return nil, err
	}
	pc, _, _, err := s.buildPropertyContribution(ctx, tenantID, propertyID, period)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

// GetSnapshot returns a stored snapshot within the tenant's scope.
func (s *ReportingService) GetSnapshot(ctx context.Context, tenantID, snapshotID string) (*ReportSnapshot, error) {
	return s.store.GetSnapshot(ctx, tenantID, snapshotID)
}

// ListSnapshots returns all stored snapshots for a tenant.
func (s *ReportingService) ListSnapshots(ctx context.Context, tenantID string) ([]ReportSnapshot, error) {
	snapshots, err := s.store.ListSnapshots(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if snapshots == nil {
		snapshots = []ReportSnapshot{}
	}
	return snapshots, nil
}

// ListOwnerExceptions returns the owner exception feed: only records that are
// owner-visible. Routine internal operational records (turnover, restock,
// stock counts, internal review, alert queues, scheduling) are classified as
// internal noise and never appear.
func (s *ReportingService) ListOwnerExceptions(ctx context.Context, tenantID, propertyID string) ([]OwnerException, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	rows, err := s.store.ListOwnerExceptionRows(ctx, tenantID, propertyID, nil, nil)
	if err != nil {
		return nil, err
	}
	feed := make([]OwnerException, 0, len(rows))
	for _, r := range rows {
		if r.OwnerVisible {
			feed = append(feed, r)
		}
	}
	return feed, nil
}

// MetricObservationParams is the input for recording a worker metric.
type MetricObservationParams struct {
	WorkerID    string
	PropertyID  string
	MetricKind  string
	Value       int64
	Unit        string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	SourceRef   string
}

// RecordWorkerMetric appends a worker development metric. Worker metrics are
// operational facts with provenance; they are never ranked and never become
// discipline (the recording path has no ranking or discipline concept).
func (s *ReportingService) RecordWorkerMetric(ctx context.Context, tenantID string, params MetricObservationParams, actorID string) (*MetricObservation, error) {
	if tenantID == "" || params.WorkerID == "" || params.SourceRef == "" {
		return nil, fmt.Errorf("%w: tenant_id, worker_id and source_ref are required", ErrInvalidMetric)
	}
	if !ValidMetricKind(params.MetricKind) {
		return nil, fmt.Errorf("%w: unknown metric kind %q", ErrInvalidMetric, params.MetricKind)
	}
	if params.PeriodStart != nil && params.PeriodEnd != nil && params.PeriodEnd.Before(*params.PeriodStart) {
		return nil, ErrInvalidPeriod
	}

	obs := &MetricObservation{
		TenantID:    tenantID,
		PropertyID:  params.PropertyID,
		WorkerID:    params.WorkerID,
		MetricKind:  params.MetricKind,
		Value:       params.Value,
		Unit:        params.Unit,
		PeriodStart: params.PeriodStart,
		PeriodEnd:   params.PeriodEnd,
		SourceRef:   params.SourceRef,
		RecordedBy:  actorID,
		RecordedAt:  time.Now().UTC(),
	}
	if err := s.store.InsertMetricObservation(ctx, obs); err != nil {
		return nil, err
	}

	s.appendAudit(ctx, audit.AuditEvent{
		EventType:    audit.EventTypeMutation,
		TenantID:     tenantID,
		ActorID:      actorID,
		Action:       "reporting.worker_metric.recorded",
		ResourceType: "worker_metric_observation",
		ResourceID:   obs.ID,
		Metadata:     mustJSON(map[string]any{"worker_id": obs.WorkerID, "metric_kind": obs.MetricKind}),
	})

	return obs, nil
}

// ListWorkerMetrics returns worker metrics as a chronological, non-ranked,
// non-disciplinary list for development review. The guardrail fails closed if
// any stored observation ever carried a rank or a discipline binding.
func (s *ReportingService) ListWorkerMetrics(ctx context.Context, tenantID, propertyID, workerID, metricKind string) ([]MetricObservation, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidMetric)
	}
	observations, err := s.store.ListMetricObservations(ctx, tenantID, propertyID, workerID, metricKind)
	if err != nil {
		return nil, err
	}
	if err := GuardMetricsNonDisciplinary(observations); err != nil {
		return nil, err
	}
	if observations == nil {
		observations = []MetricObservation{}
	}
	return observations, nil
}

// WorkerMetricSummary aggregates worker metrics without producing any rank
// position. It exists so development review can see the distribution of a
// metric without turning the metric into a leaderboard.
func (s *ReportingService) WorkerMetricSummary(ctx context.Context, tenantID, propertyID, workerID, metricKind string) (*MetricSummary, error) {
	if tenantID == "" || workerID == "" || metricKind == "" {
		return nil, fmt.Errorf("%w: tenant_id, worker_id and metric_kind are required", ErrInvalidMetric)
	}
	observations, err := s.store.ListMetricObservations(ctx, tenantID, propertyID, workerID, metricKind)
	if err != nil {
		return nil, err
	}
	if err := GuardMetricsNonDisciplinary(observations); err != nil {
		return nil, err
	}

	summary := &MetricSummary{
		WorkerID:   workerID,
		MetricKind: metricKind,
	}
	if len(observations) == 0 {
		return summary, nil
	}

	values := make([]int64, 0, len(observations))
	for _, o := range observations {
		values = append(values, o.Value)
		summary.Sum += o.Value
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	summary.Count = len(values)
	summary.Minimum = values[0]
	summary.Maximum = values[len(values)-1]
	summary.Average = summary.Sum / int64(len(values))

	return summary, nil
}

// GetReadiness returns the property readiness read model.
func (s *ReportingService) GetReadiness(ctx context.Context, tenantID, propertyID string) (*PropertyReadiness, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	r, _, _, err := s.buildReadiness(ctx, tenantID, propertyID)
	return r, err
}

// GetServiceLevelSummary returns the service level read model.
func (s *ReportingService) GetServiceLevelSummary(ctx context.Context, tenantID, propertyID string, period *Period) (*ServiceLevelSummary, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	sls, _, _, err := s.buildServiceLevelSummary(ctx, tenantID, propertyID, period)
	return sls, err
}

// GetInventorySummary returns the inventory read model.
func (s *ReportingService) GetInventorySummary(ctx context.Context, tenantID, propertyID string) (*InventorySummary, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	is, _, _, err := s.buildInventorySummary(ctx, tenantID, propertyID)
	return is, err
}

// GetApprovalPipeline returns the approval pipeline read model.
func (s *ReportingService) GetApprovalPipeline(ctx context.Context, tenantID, propertyID string) (*ApprovalPipeline, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	ap, _, _, err := s.buildApprovalPipeline(ctx, tenantID, propertyID)
	return ap, err
}

// GetDocumentStatus returns the document status read model.
func (s *ReportingService) GetDocumentStatus(ctx context.Context, tenantID, propertyID string) (*DocumentStatus, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	ds, _, _, err := s.buildDocumentStatus(ctx, tenantID, propertyID)
	return ds, err
}

// GetLaborTravelSummary returns the labor and travel read model.
func (s *ReportingService) GetLaborTravelSummary(ctx context.Context, tenantID, propertyID string) (*LaborTravelSummary, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidSnapshot)
	}
	lts, _, _, err := s.buildLaborTravelSummary(ctx, tenantID, propertyID)
	return lts, err
}

func (s *ReportingService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if evt.ID == "" {
		evt.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, evt); err != nil {
		return
	}
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func mustJSON(v any) json.RawMessage {
	return mustMarshal(v)
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func jsonEqual(a, b []byte) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	ab, err1 := json.Marshal(av)
	bb, err2 := json.Marshal(bv)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}
