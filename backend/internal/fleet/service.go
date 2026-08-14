package fleet

import (
	"context"
	"errors"
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
	store      *FleetStore
	auditStore *audit.AuditStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:       pool,
		store:      NewFleetStore(pool),
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

func (s *Service) CreateAsset(ctx context.Context, tenantID string, params CreateAssetParams, actorID string) (*FleetAsset, *FleetBattery, error) {
	if params.Model == "" || params.SerialNumber == "" {
		return nil, nil, fmt.Errorf("%w: model and serial number are required", ErrInvalidAsset)
	}
	if params.RatedMotorPowerWatts <= 0 || params.RatedMotorPowerWatts > RatedMotorPowerWattsLimit {
		return nil, nil, ErrPowerLimitExceeded
	}
	if params.MaximumDesignSpeedKmh <= 0 || params.MaximumDesignSpeedKmh > MaximumDesignSpeedKmhLimit {
		return nil, nil, ErrDesignSpeedLimitExceeded
	}
	if params.DesignSpeedEvidenceRef == "" || params.ComplianceDocumentRef == "" {
		return nil, nil, ErrComplianceEvidenceRequired
	}
	if params.PurchaseDate.IsZero() {
		return nil, nil, fmt.Errorf("%w: purchase date is required", ErrInvalidAsset)
	}
	if params.BatterySerial == "" {
		return nil, nil, fmt.Errorf("%w: battery serial is required", ErrInvalidAsset)
	}

	asset := &FleetAsset{
		TenantID:               tenantID,
		Model:                  params.Model,
		SerialNumber:           params.SerialNumber,
		RatedMotorPowerWatts:   params.RatedMotorPowerWatts,
		MaximumDesignSpeedKmh:  params.MaximumDesignSpeedKmh,
		DesignSpeedEvidenceRef: params.DesignSpeedEvidenceRef,
		ComplianceDocumentRef:  params.ComplianceDocumentRef,
		BatterySerial:          params.BatterySerial,
		Charger:                params.Charger,
		PurchaseDate:           params.PurchaseDate,
		WarrantyExpiresAt:      params.WarrantyExpiresAt,
		WarrantyTerms:          params.WarrantyTerms,
		Status:                 AssetStatusAvailable,
	}

	battery := &FleetBattery{
		TenantID:      tenantID,
		AssetID:       asset.ID,
		BatterySerial: params.BatterySerial,
		HealthStatus:  "ok",
		Status:        "active",
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertAsset(ctx, tx, asset); err != nil {
			return err
		}
		battery.AssetID = asset.ID
		if err := s.store.InsertBattery(ctx, tx, battery); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.asset.created",
			ResourceType: "fleet_asset",
			ResourceID:   asset.ID,
			NewState:     marshalJSON(asset),
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return asset, battery, nil
}

func (s *Service) GetAsset(ctx context.Context, tenantID, assetID string) (*FleetAsset, error) {
	return s.store.GetAsset(ctx, tenantID, assetID)
}

func (s *Service) ListAssets(ctx context.Context, tenantID string) ([]FleetAsset, error) {
	return s.store.ListAssets(ctx, tenantID)
}

func (s *Service) ScheduleSafetyItem(ctx context.Context, tenantID, assetID string, params SafetyItemParams, actorID string) (*FleetMaintenanceRecord, error) {
	if !ValidSafetyKind(params.Kind) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSafetyKind, params.Kind)
	}
	if params.DueAt.IsZero() {
		return nil, ErrSafetyItemDueRequired
	}
	if params.Title == "" {
		params.Title = params.Kind
	}

	if _, err := s.store.GetAsset(ctx, tenantID, assetID); err != nil {
		return nil, err
	}

	record := &FleetMaintenanceRecord{
		TenantID:    tenantID,
		AssetID:     assetID,
		Kind:        params.Kind,
		Title:       params.Title,
		Description: params.Description,
		DueAt:       &params.DueAt,
		Status:      ItemStatusOpen,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertMaintenanceRecord(ctx, tx, record); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.safety_item.scheduled",
			ResourceType: "fleet_maintenance",
			ResourceID:   record.ID,
			NewState:     marshalJSON(record),
		})
	})
	if err != nil {
		return nil, err
	}

	return record, nil
}

func (s *Service) CompleteSafetyItem(ctx context.Context, tenantID, recordID string, params CompleteSafetyItemParams, actorID string) (*FleetMaintenanceRecord, error) {
	if params.CompletedAt.IsZero() {
		params.CompletedAt = time.Now().UTC()
	}

	existing, err := s.store.GetMaintenanceRecord(ctx, tenantID, recordID)
	if err != nil {
		return nil, err
	}
	if existing.Status == ItemStatusCompleted {
		return nil, ErrSafetyItemAlreadyCompleted
	}

	var record *FleetMaintenanceRecord
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		record, err = s.store.CompleteMaintenanceRecord(ctx, tx, tenantID, recordID, params.CompletedAt, params.PerformedBy, params.Notes)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSafetyItemNotFound
			}
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.safety_item.completed",
			ResourceType: "fleet_maintenance",
			ResourceID:   record.ID,
			NewState:     marshalJSON(record),
		})
	})
	if err != nil {
		return nil, err
	}

	return record, nil
}

func (s *Service) ListOverdueSafetyItems(ctx context.Context, tenantID, assetID string, now time.Time) ([]FleetMaintenanceRecord, error) {
	return s.store.ListOverdueSafetyItems(ctx, tenantID, assetID, now)
}

func (s *Service) RecordInspection(ctx context.Context, tenantID, assetID string, params InspectionParams, actorID string) (*FleetInspection, error) {
	if params.InspectionType == "" {
		params.InspectionType = InspectionTypePreUse
	}
	if params.InspectionType != InspectionTypePreUse {
		return nil, fmt.Errorf("%w: unsupported inspection type %q", ErrInvalidInspection, params.InspectionType)
	}
	if params.Result != InspectionResultPass && params.Result != InspectionResultFail {
		return nil, ErrInspectionResultInvalid
	}
	if params.DamageReported && params.DamageDescription == "" {
		return nil, fmt.Errorf("%w: damage description is required when damage is reported", ErrInvalidInspection)
	}
	if params.WorkerID == "" {
		return nil, fmt.Errorf("%w: worker is required", ErrInvalidInspection)
	}

	if _, err := s.store.GetAsset(ctx, tenantID, assetID); err != nil {
		return nil, err
	}

	inspection := &FleetInspection{
		TenantID:          tenantID,
		AssetID:           assetID,
		WorkerID:          params.WorkerID,
		InspectionType:    params.InspectionType,
		Result:            params.Result,
		DamageReported:    params.DamageReported,
		DamageDescription: params.DamageDescription,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertInspection(ctx, tx, inspection); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.inspection.recorded",
			ResourceType: "fleet_inspection",
			ResourceID:   inspection.ID,
			NewState:     marshalJSON(inspection),
		})
	})
	if err != nil {
		return nil, err
	}

	return inspection, nil
}

func (s *Service) Handover(ctx context.Context, tenantID, assetID string, params CustodyParams, actorID string) (*FleetCustodyEvent, error) {
	if params.ToWorkerID == "" {
		return nil, fmt.Errorf("%w: recipient worker is required", ErrCustodyEventInvalid)
	}
	if params.AcknowledgedBy == "" {
		params.AcknowledgedBy = params.ToWorkerID
	}

	asset, err := s.store.GetAsset(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	if asset.AssignedCustodianID != "" && asset.AssignedCustodianID != params.FromWorkerID {
		return nil, fmt.Errorf("%w: asset is already in custody of %s", ErrCustodyEventInvalid, asset.AssignedCustodianID)
	}
	if asset.IsFrozen() {
		return nil, fmt.Errorf("%w: a frozen asset cannot change custody", ErrCustodyEventInvalid)
	}

	now := time.Now().UTC()
	event := &FleetCustodyEvent{
		TenantID:       tenantID,
		AssetID:        assetID,
		EventType:      CustodyEventTypeHandover,
		FromWorkerID:   params.FromWorkerID,
		ToWorkerID:     params.ToWorkerID,
		Condition:      params.Condition,
		Accessories:    params.Accessories,
		AcknowledgedBy: params.AcknowledgedBy,
		AcknowledgedAt: &now,
		Notes:          params.Notes,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCustodyEvent(ctx, tx, event); err != nil {
			return err
		}
		_, err := s.store.SetCustodian(ctx, tx, tenantID, assetID, params.ToWorkerID)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.custody.handover",
			ResourceType: "fleet_custody_event",
			ResourceID:   event.ID,
			NewState:     marshalJSON(event),
		})
	})
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (s *Service) Return(ctx context.Context, tenantID, assetID string, params CustodyParams, actorID string) (*FleetCustodyEvent, error) {
	asset, err := s.store.GetAsset(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	if asset.AssignedCustodianID == "" {
		return nil, ErrNoActiveCustody
	}
	if params.FromWorkerID != "" && params.FromWorkerID != asset.AssignedCustodianID {
		return nil, fmt.Errorf("%w: %s is not the current custodian", ErrCustodyMismatch, params.FromWorkerID)
	}

	now := time.Now().UTC()
	event := &FleetCustodyEvent{
		TenantID:       tenantID,
		AssetID:        assetID,
		EventType:      CustodyEventTypeReturn,
		FromWorkerID:   asset.AssignedCustodianID,
		ToWorkerID:     params.ToWorkerID,
		Condition:      params.Condition,
		Accessories:    params.Accessories,
		AcknowledgedBy: params.AcknowledgedBy,
		AcknowledgedAt: &now,
		Notes:          params.Notes,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCustodyEvent(ctx, tx, event); err != nil {
			return err
		}
		_, err := s.store.ClearCustodian(ctx, tx, tenantID, assetID)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.custody.return",
			ResourceType: "fleet_custody_event",
			ResourceID:   event.ID,
			NewState:     marshalJSON(event),
		})
	})
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (s *Service) ListCustodyEvents(ctx context.Context, tenantID, assetID string) ([]FleetCustodyEvent, error) {
	return s.store.ListCustodyEvents(ctx, tenantID, assetID)
}

// RecordIncident records a vehicle incident, links the safety ticket raised for
// it, and freezes the asset until the incident is reviewed (VEH-005).
func (s *Service) RecordIncident(ctx context.Context, tenantID, assetID string, params IncidentParams, actorID string) (*FleetIncident, error) {
	if !ValidIncidentSeverity(params.Severity) {
		return nil, fmt.Errorf("%w: %q", ErrIncidentSeverityInvalid, params.Severity)
	}
	if params.Description == "" {
		return nil, fmt.Errorf("%w: incident description is required", ErrInvalidAsset)
	}
	if params.SafetyTicketID == "" {
		return nil, fmt.Errorf("%w: a linked safety ticket is required", ErrInvalidAsset)
	}

	asset, err := s.store.GetAsset(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	previous := marshalJSON(asset)

	incident := &FleetIncident{
		TenantID:       tenantID,
		AssetID:        assetID,
		Kind:           params.Kind,
		Severity:       params.Severity,
		Description:    params.Description,
		ReportedBy:     actorID,
		SafetyTicketID: params.SafetyTicketID,
		Status:         IncidentStatusOpen,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertIncident(ctx, tx, incident); err != nil {
			return err
		}
		frozen, err := s.store.FreezeAsset(ctx, tx, tenantID, assetID)
		if err != nil {
			return err
		}
		_ = frozen
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:     audit.EventTypeMutation,
			TenantID:      tenantID,
			ActorID:       actorID,
			Action:        "fleet.incident.recorded",
			ResourceType:  "fleet_incident",
			ResourceID:    incident.ID,
			PreviousState: previous,
			NewState:      marshalJSON(incident),
		})
	})
	if err != nil {
		return nil, err
	}

	return incident, nil
}

// ReviewIncident marks an incident resolved, records the reviewer's resolution,
// and unfreezes the asset once no open incident remains.
func (s *Service) ReviewIncident(ctx context.Context, tenantID, incidentID string, params ReviewIncidentParams, actorID string) (*FleetIncident, error) {
	if params.Resolution == "" {
		return nil, ErrIncidentRequiresResolution
	}

	var incident *FleetIncident
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		incident, err = s.store.ResolveIncident(ctx, tx, tenantID, incidentID, params.Resolution, actorID, time.Now().UTC())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIncidentNotFound
			}
			return err
		}

		remaining, err := s.store.CountOpenIncidents(ctx, tx, tenantID, incident.AssetID)
		if err != nil {
			return err
		}
		if remaining == 0 {
			if _, err := s.store.UnfreezeAsset(ctx, tx, tenantID, incident.AssetID); err != nil {
				return err
			}
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.incident.reviewed",
			ResourceType: "fleet_incident",
			ResourceID:   incident.ID,
			NewState:     marshalJSON(incident),
		})
	})
	if err != nil {
		return nil, err
	}

	return incident, nil
}

func (s *Service) GetIncident(ctx context.Context, tenantID, incidentID string) (*FleetIncident, error) {
	return s.store.GetIncident(ctx, tenantID, incidentID)
}

func (s *Service) ListOpenIncidents(ctx context.Context, tenantID, assetID string) ([]FleetIncident, error) {
	return s.store.ListOpenIncidents(ctx, tenantID, assetID)
}

// DispatchEligibility evaluates every hard dispatch hold for an asset: frozen
// status, unreviewed incident, overdue safety item, and a failed pre-use
// inspection with unreported-cleared damage. Any hold blocks dispatch.
func (s *Service) DispatchEligibility(ctx context.Context, tenantID, assetID string, now time.Time) (*DispatchBlock, error) {
	asset, err := s.store.GetAsset(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}

	overdue, err := s.store.ListOverdueSafetyItems(ctx, tenantID, assetID, now)
	if err != nil {
		return nil, err
	}

	incidents, err := s.store.ListOpenIncidents(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}

	var latestInspection *FleetInspection
	if latest, err := s.store.LatestInspection(ctx, tenantID, assetID); err == nil {
		latestInspection = latest
	} else if !errors.Is(err, ErrAssetNotFound) {
		return nil, err
	}

	return EvaluateDispatch(now, asset, overdue, incidents, latestInspection), nil
}

// EvaluateDispatch is the deterministic dispatch policy. It is pure so the
// hard constraints are unit-testable without a database. Only open safety
// items whose due date has passed block dispatch.
func EvaluateDispatch(now time.Time, asset *FleetAsset, overdue []FleetMaintenanceRecord, openIncidents []FleetIncident, latestInspection *FleetInspection) *DispatchBlock {
	block := &DispatchBlock{Allowed: true}

	if asset.IsFrozen() {
		block.AddReason(DispatchBlockFrozen, "asset is frozen and must not be dispatched")
	}

	for i := range openIncidents {
		inc := &openIncidents[i]
		block.AddReason(DispatchBlockIncidentPending,
			fmt.Sprintf("incident %s (%s) pending review: %s", inc.ID, inc.Severity, inc.Description))
	}

	for i := range overdue {
		item := &overdue[i]
		if !IsSafetyItemOverdue(now, item) {
			continue
		}
		due := "no due date"
		if item.DueAt != nil {
			due = item.DueAt.Format(time.RFC3339)
		}
		block.AddReason(DispatchBlockSafetyOverdue,
			fmt.Sprintf("%s safety item %q is overdue since %s", item.Kind, item.Title, due))
	}

	if latestInspection != nil && !latestInspection.IsPassingPreUse() {
		msg := "the latest pre-use inspection did not pass"
		if latestInspection.DamageReported {
			msg = "the latest pre-use inspection reported damage: " + latestInspection.DamageDescription
		}
		block.AddReason(DispatchBlockInspectionFailed, msg)
	}

	return block
}

// GetTrackingStatus reports whether location tracking is currently enabled for
// a worker. Tracking is enabled only while the worker holds an active asset
// custody and is automatically disabled the moment the asset is returned
// (VEH-009).
func (s *Service) GetTrackingStatus(ctx context.Context, tenantID, workerID string) (*TrackingStatus, error) {
	asset, err := s.store.GetActiveCustodyAsset(ctx, tenantID, workerID)
	if errors.Is(err, ErrAssetNotFound) {
		return &TrackingStatus{Tracking: false}, nil
	}
	if err != nil {
		return nil, err
	}

	latest, err := s.store.LatestCustodyEvent(ctx, tenantID, asset.ID)
	if err != nil {
		return nil, err
	}

	return &TrackingStatus{
		Tracking: true,
		AssetID:  asset.ID,
		Since:    latest.CreatedAt,
	}, nil
}

// CollectLocation records a worker location sample only while the worker is on
// duty with an active asset custody (an accepted active route or task). While
// off duty, collection is refused and nothing is stored, so off-duty location
// is never collected.
func (s *Service) CollectLocation(ctx context.Context, tenantID, workerID string, params LocationParams, actorID string) (*FleetTrackingEvent, error) {
	asset, err := s.store.GetAssetByCustodian(ctx, tenantID, workerID, params.AssetID)
	if errors.Is(err, ErrAssetNotFound) {
		s.appendAudit(ctx, audit.AuditEvent{
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.tracking.rejected_off_duty",
			ResourceType: "fleet_tracking_event",
			ResourceID:   params.AssetID,
			NewState:     marshalJSON(params),
		})
		return nil, ErrOffDutyTrackingDisabled
	}
	if err != nil {
		return nil, err
	}

	latest, err := s.store.LatestCustodyEvent(ctx, tenantID, asset.ID)
	if err != nil {
		return nil, err
	}

	capturedAt := params.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}

	event := &FleetTrackingEvent{
		TenantID:       tenantID,
		AssetID:        asset.ID,
		WorkerID:       workerID,
		CustodyEventID: latest.ID,
		Latitude:       params.Latitude,
		Longitude:      params.Longitude,
		CapturedAt:     capturedAt,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertTrackingEvent(ctx, tx, event); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "fleet.tracking.recorded",
			ResourceType: "fleet_tracking_event",
			ResourceID:   event.ID,
			NewState:     marshalJSON(event),
		})
	}); err != nil {
		return nil, err
	}

	return event, nil
}

func (s *Service) CountTrackingEvents(ctx context.Context, tenantID, workerID string) (int, error) {
	return s.store.CountTrackingEvents(ctx, tenantID, workerID)
}
