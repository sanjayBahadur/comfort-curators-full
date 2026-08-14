package inventory

import (
	"context"
	"errors"
	"fmt"

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

func (s *Service) CreateLocation(ctx context.Context, tenantID string, params CreateLocationParams, actorID string) (*StockLocation, error) {
	if params.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidLocation)
	}
	if !ValidLocationType(params.LocationType) {
		return nil, fmt.Errorf("%w: invalid location type %q", ErrInvalidLocation, params.LocationType)
	}

	loc := &StockLocation{
		TenantID:     tenantID,
		PropertyID:   params.PropertyID,
		Name:         params.Name,
		LocationType: params.LocationType,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.insertLocation(ctx, tx, loc); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "inventory.location.created",
			ResourceType: "stock_location",
			ResourceID:   loc.ID,
			NewState:     marshalJSON(loc),
		})
	}); err != nil {
		return nil, err
	}

	return loc, nil
}

func (s *Service) GetLocation(ctx context.Context, tenantID, locationID string) (*StockLocation, error) {
	return s.store.GetLocation(ctx, tenantID, locationID)
}

func (s *Service) ListLocations(ctx context.Context, tenantID string) ([]StockLocation, error) {
	return s.store.ListLocations(ctx, tenantID)
}

// RecordMovement atomically records a stock movement in the append-only ledger.
// It locks the stock location, computes the current balance from the ledger,
// validates against negative-stock rules, and inserts the movement.
// Concurrent movements are serialized by the FOR UPDATE lock on the location row.
func (s *Service) RecordMovement(ctx context.Context, tenantID, locationID string, params RecordMovementParams, actorID string) (*InventoryMovement, error) {
	if params.CatalogItemID == "" {
		return nil, fmt.Errorf("%w: catalog_item_id is required", ErrInvalidMovement)
	}
	if !ValidMovementType(params.MovementType) {
		return nil, fmt.Errorf("%w: invalid movement type %q", ErrInvalidMovement, params.MovementType)
	}
	if params.Quantity == 0 {
		return nil, fmt.Errorf("%w: quantity must not be zero", ErrInvalidMovement)
	}

	var movement *InventoryMovement
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := s.store.LockLocation(ctx, tx, tenantID, locationID); err != nil {
			if errors.Is(err, ErrLocationNotFound) {
				return err
			}
			return fmt.Errorf("%w: location not found", ErrInvalidMovement)
		}

		balance, err := s.store.GetBalance(ctx, tx, tenantID, locationID, params.CatalogItemID)
		if err != nil {
			return err
		}

		negative, explained := IsNegativeStock(balance, params.Quantity, params.MovementType)
		if negative && !explained {
			return fmt.Errorf("%w: current balance %d, proposed change %d would result in negative stock",
				ErrNegativeStock, balance, params.Quantity)
		}

		if params.MovementType == MovementTypeIssue || params.MovementType == MovementTypeConsumption {
			effective, err := s.store.GetEffectiveBalance(ctx, tx, tenantID, locationID, params.CatalogItemID)
			if err != nil {
				return err
			}
			if effective+params.Quantity < 0 {
				return fmt.Errorf("%w: effective balance %d excludes expired stock, cannot issue/consume %d",
					ErrExpiredStockCannotIssue, effective, -params.Quantity)
			}
		}

		movement = &InventoryMovement{
			TenantID:      tenantID,
			LocationID:    locationID,
			CatalogItemID: params.CatalogItemID,
			MovementType:  params.MovementType,
			Quantity:      params.Quantity,
			ReferenceType: params.ReferenceType,
			ReferenceID:   params.ReferenceID,
			Reason:        params.Reason,
			ActorID:       actorID,
			ExpiresAt:     params.ExpiresAt,
		}

		if err := s.store.InsertMovement(ctx, tx, movement); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "inventory.movement.recorded",
			ResourceType: "inventory_movement",
			ResourceID:   movement.ID,
			NewState:     marshalJSON(movement),
		})
	})
	if err != nil {
		return nil, err
	}

	return movement, nil
}

// GetBalance rebuilds the stock balance from the append-only movement ledger
// for a single location and catalog item. The balance is always computed,
// never stored.
func (s *Service) GetBalance(ctx context.Context, tenantID, locationID, catalogItemID string) (int64, []InventoryMovement, error) {
	movements, err := s.store.ListMovements(ctx, tenantID, locationID, catalogItemID)
	if err != nil {
		return 0, nil, err
	}
	balance := ComputeBalance(movements)
	return balance, movements, nil
}

// ListMovements returns the full movement history for a location and item.
func (s *Service) ListMovements(ctx context.Context, tenantID, locationID, catalogItemID string) ([]InventoryMovement, error) {
	return s.store.ListMovements(ctx, tenantID, locationID, catalogItemID)
}

func (s *Service) CreateCount(ctx context.Context, tenantID string, params CreateCountParams, actorID string) (*InventoryCount, error) {
	if params.LocationID == "" {
		return nil, fmt.Errorf("%w: location_id is required", ErrInvalidCount)
	}

	if _, err := s.store.GetLocation(ctx, tenantID, params.LocationID); err != nil {
		return nil, err
	}

	count := &InventoryCount{
		TenantID:   tenantID,
		LocationID: params.LocationID,
		Status:     CountStatusDraft,
		CountedBy:  params.CountedBy,
	}

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCount(ctx, tx, count); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "inventory.count.created",
			ResourceType: "inventory_count",
			ResourceID:   count.ID,
			NewState:     marshalJSON(count),
		})
	})
	if err != nil {
		return nil, err
	}

	return count, nil
}

func (s *Service) GetCount(ctx context.Context, tenantID, countID string) (*InventoryCount, []InventoryCountLine, error) {
	count, err := s.store.GetCount(ctx, tenantID, countID)
	if err != nil {
		return nil, nil, err
	}
	lines, err := s.store.ListCountLines(ctx, tenantID, countID)
	if err != nil {
		return nil, nil, err
	}
	return count, lines, nil
}

func (s *Service) UpdateCountLine(ctx context.Context, tenantID, countID string, params UpdateCountLineParams, actorID string) (*InventoryCountLine, error) {
	count, err := s.store.GetCount(ctx, tenantID, countID)
	if err != nil {
		return nil, err
	}
	if count.Status == CountStatusReviewed || count.Status == CountStatusReconciled {
		return nil, ErrCountAlreadyReviewed
	}

	balance, err := s.store.GetBalance(ctx, s.pool, tenantID, count.LocationID, params.CatalogItemID)
	if err != nil {
		return nil, err
	}

	variance := params.CountedQuantity - balance
	line := &InventoryCountLine{
		TenantID:         tenantID,
		CountID:          countID,
		CatalogItemID:    params.CatalogItemID,
		ExpectedQuantity: balance,
		CountedQuantity:  params.CountedQuantity,
		Variance:         variance,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if count.Status == CountStatusDraft {
			_, err := s.store.UpdateCountStatus(ctx, tx, tenantID, countID, CountStatusInProgress, "")
			if err != nil {
				return err
			}
		}
		if err := s.store.UpsertCountLine(ctx, tx, line); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "inventory.count.line_updated",
			ResourceType: "inventory_count_line",
			ResourceID:   line.ID,
			NewState:     marshalJSON(line),
		})
	})
	if err != nil {
		return nil, err
	}

	return line, nil
}

func (s *Service) ReviewCount(ctx context.Context, tenantID, countID string, params ReviewCountParams, actorID string) (*InventoryCount, error) {
	if params.ReviewedBy == "" {
		return nil, fmt.Errorf("%w: reviewed_by is required", ErrInvalidCount)
	}

	var count *InventoryCount
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		existing, err := s.store.GetCount(ctx, tenantID, countID)
		if err != nil {
			return err
		}
		if existing.Status == CountStatusReviewed || existing.Status == CountStatusReconciled {
			return ErrCountAlreadyReviewed
		}

		count, err = s.store.UpdateCountStatus(ctx, tx, tenantID, countID, CountStatusReviewed, params.ReviewedBy)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "inventory.count.reviewed",
			ResourceType: "inventory_count",
			ResourceID:   count.ID,
			NewState:     marshalJSON(count),
		})
	})
	if err != nil {
		return nil, err
	}

	return count, nil
}

// ReconcileCount posts an attributable adjustment for every line variance
// and records the reconciliation. Each adjustment is an explicit ledger
// entry with the counter, reviewer, reason and source count. The original
// ledger entries are never modified.
func (s *Service) ReconcileCount(ctx context.Context, tenantID, countID string, params ReconcileCountParams, actorID string) (*InventoryCount, error) {
	if params.ReviewedBy == "" {
		return nil, fmt.Errorf("%w: reviewed_by is required", ErrInvalidCount)
	}
	if params.Reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidCount)
	}

	var count *InventoryCount
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		existing, err := s.store.GetCount(ctx, tenantID, countID)
		if err != nil {
			return err
		}
		if existing.Status == CountStatusReconciled {
			return ErrCountAlreadyReviewed
		}

		lines, err := s.store.ListCountLines(ctx, tenantID, countID)
		if err != nil {
			return err
		}

		for _, line := range lines {
			if line.Variance == 0 {
				continue
			}

			adjustment := &InventoryMovement{
				TenantID:      tenantID,
				LocationID:    existing.LocationID,
				CatalogItemID: line.CatalogItemID,
				MovementType:  MovementTypeAdjustment,
				Quantity:      line.Variance,
				ReferenceType: "inventory_count",
				ReferenceID:   countID,
				Reason:        fmt.Sprintf("%s: %s (counted_by=%s, reviewed_by=%s)", params.Reason, params.ReviewedBy, existing.CountedBy, params.ReviewedBy),
				ActorID:       actorID,
			}

			if err := s.store.InsertMovement(ctx, tx, adjustment); err != nil {
				return err
			}
		}

		count, err = s.store.UpdateCountStatus(ctx, tx, tenantID, countID, CountStatusReconciled, params.ReviewedBy)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "inventory.count.reconciled",
			ResourceType: "inventory_count",
			ResourceID:   count.ID,
			NewState:     marshalJSON(count),
		})
	})
	if err != nil {
		return nil, err
	}

	return count, nil
}
