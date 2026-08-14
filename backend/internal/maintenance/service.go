package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool       *pgxpool.Pool
	store      *Store
	auditStore *audit.AuditStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:       pool,
		store:      NewStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *Service) WithAudit(a *audit.AuditStore) *Service {
	s.auditStore = a
	return s
}

func (s *Service) appendAudit(ctx context.Context, event audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, event); err != nil {
		logging.Error(ctx, "failed to append audit event", "error", err)
	}
}

// IsValidSHA256Hash reports whether h is a well-formed sha256 hex digest used
// as the immutable identity of completion evidence content.
func IsValidSHA256Hash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ComputeEvidenceHash returns the stable sha256 hex digest of evidence bytes.
func ComputeEvidenceHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func (s *Service) CreateRequest(ctx context.Context, tenantID string, params CreateRequestParams, actorID string) (*MaintenanceRequest, error) {
	if params.PropertyID == "" {
		return nil, fmt.Errorf("%w: property_id is required", ErrInvalidRequest)
	}
	if params.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidRequest)
	}
	if params.Category != "" && !ValidCategory(params.Category) {
		return nil, fmt.Errorf("%w: invalid category %q", ErrInvalidRequest, params.Category)
	}
	if params.Priority == "" {
		params.Priority = PriorityNormal
	}
	if !ValidPriority(params.Priority) {
		return nil, fmt.Errorf("%w: invalid priority %q", ErrInvalidRequest, params.Priority)
	}
	if params.RiskLevel == "" {
		params.RiskLevel = RiskLevelStandard
	}
	if !ValidRiskLevel(params.RiskLevel) {
		return nil, ErrInvalidRiskLevel
	}

	req := &MaintenanceRequest{
		TenantID:   tenantID,
		PropertyID: params.PropertyID,
		Title:      params.Title,
		Category:   params.Category,
		Priority:   params.Priority,
		RiskLevel:  params.RiskLevel,
		Status:     RequestStatusReported,
		ReportedBy: actorID,
		Notes:      params.Notes,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertRequest(ctx, tx, req); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.request.reported",
			ResourceType: "maintenance_request",
			ResourceID:   req.ID,
			NewState:     marshalJSON(req),
		})
	}); err != nil {
		return nil, err
	}

	return req, nil
}

func (s *Service) GetRequest(ctx context.Context, tenantID, requestID string) (*MaintenanceRequest, error) {
	return s.store.GetRequest(ctx, tenantID, requestID)
}

func (s *Service) ListRequests(ctx context.Context, tenantID, propertyID string) ([]MaintenanceRequest, error) {
	return s.store.ListRequests(ctx, tenantID, propertyID)
}

func (s *Service) TriageRequest(ctx context.Context, tenantID, requestID string, params TriageRequestParams, actorID string) (*MaintenanceRequest, error) {
	if params.Category == "" || !ValidCategory(params.Category) {
		return nil, fmt.Errorf("%w: valid category is required", ErrInvalidRequest)
	}
	if params.Priority == "" || !ValidPriority(params.Priority) {
		return nil, fmt.Errorf("%w: valid priority is required", ErrInvalidRequest)
	}
	if !ValidRiskLevel(params.RiskLevel) {
		return nil, ErrInvalidRiskLevel
	}

	req, err := s.store.GetRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if req.Status != RequestStatusReported {
		return nil, fmt.Errorf("%w: request status is %q", ErrRequestNotTriaged, req.Status)
	}

	var triaged *MaintenanceRequest
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		triaged, err = s.store.TriageRequest(ctx, tx, tenantID, requestID, params, actorID, time.Now())
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.request.triaged",
			ResourceType: "maintenance_request",
			ResourceID:   requestID,
			NewState:     marshalJSON(triaged),
		})
	})
	if err != nil {
		return nil, err
	}

	return triaged, nil
}

func (s *Service) CreateEstimate(ctx context.Context, tenantID, requestID string, params CreateEstimateParams, actorID string) (*MaintenanceEstimate, error) {
	if params.AmountMinorUnits < 0 {
		return nil, fmt.Errorf("%w: amount must not be negative", ErrInvalidEstimate)
	}
	if !ValidCurrency(params.Currency) {
		return nil, fmt.Errorf("%w: invalid currency %q", ErrInvalidEstimate, params.Currency)
	}
	if params.Scope == "" {
		return nil, fmt.Errorf("%w: scope is required", ErrInvalidEstimate)
	}

	req, err := s.store.GetRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if req.Status != RequestStatusTriaged {
		return nil, fmt.Errorf("%w: request status is %q", ErrRequestNotTriaged, req.Status)
	}

	est := &MaintenanceEstimate{
		TenantID:         tenantID,
		RequestID:        requestID,
		PropertyID:       req.PropertyID,
		PreparedBy:       actorID,
		AmountMinorUnits: params.AmountMinorUnits,
		Currency:         params.Currency,
		Scope:            params.Scope,
		Status:           EstimateStatusDraft,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertEstimate(ctx, tx, est); err != nil {
			return err
		}
		if err := s.store.SetRequestEstimate(ctx, tx, tenantID, requestID, est.ID); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.estimate.created",
			ResourceType: "maintenance_estimate",
			ResourceID:   est.ID,
			NewState:     marshalJSON(est),
		})
	})
	if err != nil {
		return nil, err
	}

	return est, nil
}

func (s *Service) GetEstimate(ctx context.Context, tenantID, estimateID string) (*MaintenanceEstimate, error) {
	return s.store.GetEstimate(ctx, tenantID, estimateID)
}

func (s *Service) ListEstimates(ctx context.Context, tenantID, requestID string) ([]MaintenanceEstimate, error) {
	return s.store.ListEstimates(ctx, tenantID, requestID)
}

// SubmitEstimate preserves the estimate: once submitted it can no longer be
// edited; only an approval decision may follow.
func (s *Service) SubmitEstimate(ctx context.Context, tenantID, estimateID, actorID string) (*MaintenanceEstimate, error) {
	est, err := s.store.GetEstimate(ctx, tenantID, estimateID)
	if err != nil {
		return nil, err
	}
	if est.Status != EstimateStatusDraft {
		return nil, fmt.Errorf("%w: estimate status is %q", ErrEstimateImmutable, est.Status)
	}

	var submitted *MaintenanceEstimate
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		submitted, err = s.store.SubmitEstimate(ctx, tx, tenantID, estimateID, time.Now())
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.estimate.submitted",
			ResourceType: "maintenance_estimate",
			ResourceID:   estimateID,
			NewState:     marshalJSON(submitted),
		})
	})
	if err != nil {
		return nil, err
	}

	return submitted, nil
}

func (s *Service) DecideEstimate(ctx context.Context, tenantID, estimateID string, params DecideEstimateParams) (*MaintenanceEstimate, error) {
	if params.ActorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidApproval)
	}
	if params.IsAIActor {
		return nil, ErrAICannotApprove
	}
	if !ValidApprovalDecision(params.Decision) {
		return nil, fmt.Errorf("%w: invalid decision %q", ErrInvalidApproval, params.Decision)
	}

	est, err := s.store.GetEstimate(ctx, tenantID, estimateID)
	if err != nil {
		return nil, err
	}
	if est.Status != EstimateStatusPendingApproval {
		return nil, fmt.Errorf("%w: estimate status is %q", ErrEstimateNotPending, est.Status)
	}
	if params.ActorID == est.PreparedBy {
		return nil, ErrSelfApprovalDenied
	}

	approval := &MaintenanceApproval{
		TenantID:   tenantID,
		RequestID:  est.RequestID,
		EstimateID: estimateID,
		ActorID:    params.ActorID,
		Decision:   params.Decision,
		Reason:     params.Reason,
		IsAIActor:  params.IsAIActor,
	}

	var decided *MaintenanceEstimate
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertApproval(ctx, tx, approval); err != nil {
			return err
		}
		decided, err = s.store.DecideEstimate(ctx, tx, tenantID, estimateID, params)
		if err != nil {
			return err
		}
		status := RequestStatusEstimateApproved
		if params.Decision == ApprovalDecisionRejected {
			status = RequestStatusEstimateRejected
		}
		_, err = s.store.UpdateRequestStatus(ctx, tx, tenantID, est.RequestID, status, estimateID)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "maintenance.estimate." + params.Decision,
			ResourceType: "maintenance_estimate",
			ResourceID:   estimateID,
			NewState:     marshalJSON(decided),
		})
	})
	if err != nil {
		return nil, err
	}

	return decided, nil
}

func (s *Service) GetApprovals(ctx context.Context, tenantID, requestID string) ([]MaintenanceApproval, error) {
	return s.store.ListApprovals(ctx, tenantID, requestID)
}

// AssignVendorWorkOrder routes specialist work to a qualified vendor. The
// bounded scope on the work order is the only scope the vendor may see.
// Assignment preserves the estimate; the "unapproved estimate cannot start"
// control is enforced when the work actually starts.
func (s *Service) AssignVendorWorkOrder(ctx context.Context, tenantID, requestID string, params AssignVendorWorkOrderParams, actorID string) (*VendorWorkOrder, error) {
	if params.VendorID == "" {
		return nil, fmt.Errorf("%w: vendor_id is required", ErrInvalidWorkOrder)
	}
	if params.Scope == "" {
		return nil, fmt.Errorf("%w: assigned scope is required", ErrInvalidWorkOrder)
	}

	req, err := s.store.GetRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if req.Status != RequestStatusTriaged && req.Status != RequestStatusEstimateApproved {
		return nil, fmt.Errorf("%w: request status is %q", ErrRequestNotTriaged, req.Status)
	}
	if req.EstimateID == "" {
		return nil, fmt.Errorf("%w: request has no estimate", ErrRequestNotTriaged)
	}

	est, err := s.store.GetEstimate(ctx, tenantID, req.EstimateID)
	if err != nil {
		return nil, err
	}
	if est.Status == EstimateStatusRejected {
		return nil, fmt.Errorf("%w: estimate status is %q", ErrEstimateNotApproved, est.Status)
	}

	wo := &VendorWorkOrder{
		TenantID:   tenantID,
		RequestID:  requestID,
		EstimateID: req.EstimateID,
		PropertyID: req.PropertyID,
		VendorID:   params.VendorID,
		Scope:      params.Scope,
		RiskLevel:  req.RiskLevel,
		Status:     WorkOrderStatusAssigned,
		AssignedBy: actorID,
		AssignedAt: time.Now(),
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertWorkOrder(ctx, tx, wo); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.work_order.assigned",
			ResourceType: "vendor_work_order",
			ResourceID:   wo.ID,
			NewState:     marshalJSON(wo),
		})
	}); err != nil {
		return nil, err
	}

	return wo, nil
}

// StartWorkOrder enforces the "unapproved estimate cannot start" control. The
// work order may only start when its linked estimate is approved.
func (s *Service) StartWorkOrder(ctx context.Context, tenantID, workOrderID, actorID string) (*VendorWorkOrder, error) {
	wo, err := s.store.GetWorkOrder(ctx, tenantID, workOrderID)
	if err != nil {
		return nil, err
	}
	if wo.Status != WorkOrderStatusAssigned {
		return nil, fmt.Errorf("%w: work order status is %q", ErrWorkOrderNotAssigned, wo.Status)
	}

	est, err := s.store.GetEstimate(ctx, tenantID, wo.EstimateID)
	if err != nil {
		return nil, err
	}
	if err := ValidateStartReady(est.Status); err != nil {
		return nil, err
	}

	var started *VendorWorkOrder
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		started, err = s.store.StartWorkOrder(ctx, tx, tenantID, workOrderID, time.Now())
		if err != nil {
			return err
		}
		_, err = s.store.UpdateRequestStatus(ctx, tx, tenantID, wo.RequestID, RequestStatusInProgress, "")
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.work_order.started",
			ResourceType: "vendor_work_order",
			ResourceID:   workOrderID,
			NewState:     marshalJSON(started),
		})
	})
	if err != nil {
		return nil, err
	}

	return started, nil
}

// CompleteWorkOrder requires the assigned vendor to submit completion evidence
// before the work can be considered done.
func (s *Service) CompleteWorkOrder(ctx context.Context, tenantID, workOrderID string, params CompleteWorkOrderParams) (*VendorWorkOrder, error) {
	if params.CompletedBy == "" {
		return nil, fmt.Errorf("%w: completed_by is required", ErrInvalidWorkOrder)
	}
	if params.CompletionEvidenceRef == "" {
		return nil, fmt.Errorf("%w: completion evidence is required", ErrCompletionEvidenceRequired)
	}
	if !IsValidSHA256Hash(params.CompletionEvidenceRef) {
		return nil, fmt.Errorf("%w: completion evidence must be a sha256 content hash", ErrInvalidWorkOrder)
	}

	wo, err := s.store.GetWorkOrder(ctx, tenantID, workOrderID)
	if err != nil {
		return nil, err
	}
	if wo.Status != WorkOrderStatusInProgress {
		return nil, fmt.Errorf("%w: work order status is %q", ErrWorkOrderNotInProgress, wo.Status)
	}
	if params.CompletedBy != wo.VendorID {
		return nil, fmt.Errorf("%w: only the assigned vendor may complete their own work", ErrInvalidWorkOrder)
	}

	var completed *VendorWorkOrder
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		completed, err = s.store.CompleteWorkOrder(ctx, tx, tenantID, workOrderID, params, time.Now())
		if err != nil {
			return err
		}
		_, err = s.store.UpdateRequestStatus(ctx, tx, tenantID, wo.RequestID, RequestStatusCompleted, "")
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.CompletedBy,
			Action:       "maintenance.work_order.completed",
			ResourceType: "vendor_work_order",
			ResourceID:   workOrderID,
			NewState:     marshalJSON(completed),
		})
	})
	if err != nil {
		return nil, err
	}

	return completed, nil
}

// VerifyWorkOrder enforces the "high-risk actor cannot self-verify" control.
// High-risk work requires an independent verifier who neither performed the
// work nor is the assigned vendor.
func (s *Service) VerifyWorkOrder(ctx context.Context, tenantID, workOrderID, verifierID string) (*VendorWorkOrder, error) {
	if verifierID == "" {
		return nil, fmt.Errorf("%w: verifier_id is required", ErrInvalidWorkOrder)
	}

	wo, err := s.store.GetWorkOrder(ctx, tenantID, workOrderID)
	if err != nil {
		return nil, err
	}
	if wo.Status != WorkOrderStatusCompleted {
		return nil, fmt.Errorf("%w: work order status is %q", ErrWorkOrderNotCompleted, wo.Status)
	}
	if err := ValidateVerifier(wo.RiskLevel, verifierID, wo.CompletedBy, wo.VendorID); err != nil {
		return nil, err
	}

	var verified *VendorWorkOrder
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		verified, err = s.store.VerifyWorkOrder(ctx, tx, tenantID, workOrderID, verifierID, time.Now())
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      verifierID,
			Action:       "maintenance.work_order.verified",
			ResourceType: "vendor_work_order",
			ResourceID:   workOrderID,
			NewState:     marshalJSON(verified),
		})
	})
	if err != nil {
		return nil, err
	}

	return verified, nil
}

type RecordWarrantyParams struct {
	Provider  string
	Coverage  string
	ExpiresAt *time.Time
}

// RecordWarranty retains warranty history for verified work. Warranty records
// are append-only and never hard-deleted.
func (s *Service) RecordWarranty(ctx context.Context, tenantID, workOrderID string, params RecordWarrantyParams, actorID string) (*WarrantyRecord, error) {
	if params.Provider == "" {
		return nil, fmt.Errorf("%w: provider is required", ErrInvalidWarranty)
	}
	if params.Coverage == "" {
		return nil, fmt.Errorf("%w: coverage is required", ErrInvalidWarranty)
	}

	wo, err := s.store.GetWorkOrder(ctx, tenantID, workOrderID)
	if err != nil {
		return nil, err
	}
	if wo.Status != WorkOrderStatusVerified {
		return nil, fmt.Errorf("%w: warranty can only be recorded for verified work (status: %s)", ErrInvalidWarranty, wo.Status)
	}

	record := &WarrantyRecord{
		TenantID:    tenantID,
		WorkOrderID: workOrderID,
		PropertyID:  wo.PropertyID,
		VendorID:    wo.VendorID,
		Provider:    params.Provider,
		Coverage:    params.Coverage,
		ExpiresAt:   params.ExpiresAt,
		Status:      WarrantyStatusActive,
		RecordedBy:  actorID,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertWarranty(ctx, tx, record); err != nil {
			return err
		}
		if _, err := s.store.UpdateWorkOrderStatus(ctx, tx, tenantID, workOrderID, WorkOrderStatusClosed); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "maintenance.warranty.recorded",
			ResourceType: "warranty_record",
			ResourceID:   record.ID,
			NewState:     marshalJSON(record),
		})
	})
	if err != nil {
		return nil, err
	}

	return record, nil
}

func (s *Service) GetWorkOrder(ctx context.Context, tenantID, workOrderID string) (*VendorWorkOrder, error) {
	return s.store.GetWorkOrder(ctx, tenantID, workOrderID)
}

func (s *Service) ListWorkOrders(ctx context.Context, tenantID, requestID string) ([]VendorWorkOrder, error) {
	return s.store.ListWorkOrders(ctx, tenantID, requestID)
}

// GetVendorWorkOrder returns a work order only when it is assigned to the
// given vendor. A vendor never sees another vendor's scope.
func (s *Service) GetVendorWorkOrder(ctx context.Context, tenantID, vendorID, workOrderID string) (*VendorWorkOrder, error) {
	return s.store.GetVendorWorkOrder(ctx, tenantID, vendorID, workOrderID)
}

// ListVendorWorkOrders returns only the work orders assigned to the given
// vendor, scoped to that vendor's assigned work.
func (s *Service) ListVendorWorkOrders(ctx context.Context, tenantID, vendorID string) ([]VendorWorkOrder, error) {
	return s.store.ListVendorWorkOrders(ctx, tenantID, vendorID)
}

func (s *Service) GetWarranty(ctx context.Context, tenantID, warrantyID string) (*WarrantyRecord, error) {
	return s.store.GetWarranty(ctx, tenantID, warrantyID)
}

func (s *Service) ListWarranties(ctx context.Context, tenantID, propertyID string) ([]WarrantyRecord, error) {
	return s.store.ListWarranties(ctx, tenantID, propertyID)
}
