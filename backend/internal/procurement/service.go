package procurement

import (
	"context"
	"encoding/json"
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

type CatalogItemReader struct {
	ID               string
	SKU              string
	Name             string
	Category         string
	Supplier         string
	UnitCostCurrency string
	Status           string
}

type StockLevel struct {
	CatalogItemID string
	Balance       int64
}

type InventoryReader interface {
	GetStockLevels(ctx context.Context, tenantID, locationID string, catalogItemIDs []string) ([]StockLevel, error)
}

type CatalogReader interface {
	GetCatalogItem(ctx context.Context, tenantID, catalogItemID string) (*CatalogItemReader, error)
}

func (s *Service) CreateSupplier(ctx context.Context, tenantID string, params CreateSupplierParams, actorID string) (*Supplier, error) {
	if params.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidSupplier)
	}

	supplier := &Supplier{
		TenantID:    tenantID,
		Name:        params.Name,
		ContactInfo: params.ContactInfo,
		Status:      SupplierStatusPendingApproval,
		CreatedBy:   actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertSupplier(ctx, tx, supplier); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.supplier.created",
			ResourceType: "supplier",
			ResourceID:   supplier.ID,
			NewState:     marshalJSON(supplier),
		})
	}); err != nil {
		return nil, err
	}

	return supplier, nil
}

func (s *Service) GetSupplier(ctx context.Context, tenantID, supplierID string) (*Supplier, error) {
	return s.store.GetSupplier(ctx, tenantID, supplierID)
}

func (s *Service) ListSuppliers(ctx context.Context, tenantID string) ([]Supplier, error) {
	return s.store.ListSuppliers(ctx, tenantID)
}

func (s *Service) ApproveSupplier(ctx context.Context, tenantID, supplierID, actorID string) (*Supplier, error) {
	if actorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidSupplier)
	}

	supplier, err := s.store.GetSupplier(ctx, tenantID, supplierID)
	if err != nil {
		return nil, err
	}
	if supplier.Status != SupplierStatusPendingApproval {
		return nil, fmt.Errorf("%w: supplier status is %q", ErrSupplierAlreadyApproved, supplier.Status)
	}

	now := time.Now()
	var approved *Supplier
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		approved, err = s.store.ApproveSupplier(ctx, tx, tenantID, supplierID, actorID, now)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.supplier.approved",
			ResourceType: "supplier",
			ResourceID:   supplierID,
			NewState:     marshalJSON(approved),
		})
	})
	if err != nil {
		return nil, err
	}

	return approved, nil
}

func (s *Service) RejectSupplier(ctx context.Context, tenantID, supplierID, actorID string) (*Supplier, error) {
	supplier, err := s.store.GetSupplier(ctx, tenantID, supplierID)
	if err != nil {
		return nil, err
	}
	if supplier.Status != SupplierStatusPendingApproval {
		return nil, fmt.Errorf("%w: supplier status is %q", ErrSupplierAlreadyRejected, supplier.Status)
	}

	var rejected *Supplier
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		rejected, err = s.store.RejectSupplier(ctx, tx, tenantID, supplierID)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.supplier.rejected",
			ResourceType: "supplier",
			ResourceID:   supplierID,
			NewState:     marshalJSON(rejected),
		})
	})
	if err != nil {
		return nil, err
	}

	return rejected, nil
}

func (s *Service) CreateSupplierItem(ctx context.Context, tenantID, supplierID string, params CreateSupplierItemParams, actorID string) (*SupplierItem, error) {
	if params.CatalogItemID == "" {
		return nil, fmt.Errorf("%w: catalog_item_id is required", ErrInvalidSupplierItem)
	}
	if !ValidCurrency(params.UnitCostCurrency) {
		return nil, fmt.Errorf("%w: invalid currency %q", ErrInvalidSupplierItem, params.UnitCostCurrency)
	}
	if params.UnitCostMinorUnits < 0 {
		return nil, fmt.Errorf("%w: unit cost must not be negative", ErrInvalidSupplierItem)
	}

	supplier, err := s.store.GetSupplier(ctx, tenantID, supplierID)
	if err != nil {
		return nil, err
	}
	if supplier.Status != SupplierStatusActive {
		return nil, fmt.Errorf("%w: supplier %q is not active (status: %s)", ErrInvalidSupplierItem, supplierID, supplier.Status)
	}

	si := &SupplierItem{
		TenantID:             tenantID,
		SupplierID:           supplierID,
		CatalogItemID:        params.CatalogItemID,
		SupplierSKU:          params.SupplierSKU,
		UnitCostMinorUnits:   params.UnitCostMinorUnits,
		UnitCostCurrency:     params.UnitCostCurrency,
		LeadTimeDays:         params.LeadTimeDays,
		MinimumOrderQuantity: params.MinimumOrderQuantity,
		IsPreferred:          params.IsPreferred,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertSupplierItem(ctx, tx, si); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.supplier_item.created",
			ResourceType: "supplier_item",
			ResourceID:   si.ID,
			NewState:     marshalJSON(si),
		})
	}); err != nil {
		return nil, err
	}

	return si, nil
}

func (s *Service) GetSupplierItem(ctx context.Context, tenantID, itemID string) (*SupplierItem, error) {
	return s.store.GetSupplierItem(ctx, tenantID, itemID)
}

func (s *Service) ListSupplierItems(ctx context.Context, tenantID, supplierID string) ([]SupplierItem, error) {
	return s.store.ListSupplierItems(ctx, tenantID, supplierID)
}

func (s *Service) CreateRequisition(ctx context.Context, tenantID string, params CreateRequisitionParams, actorID string, isAIActor bool) (*Requisition, error) {
	if len(params.Items) == 0 {
		return nil, fmt.Errorf("%w: at least one item is required", ErrInvalidRequisition)
	}

	if isAIActor {
		return nil, fmt.Errorf("%w: AI actor cannot create a requisition", ErrAICannotApprove)
	}

	currency := ""

	req := &Requisition{
		TenantID:       tenantID,
		PropertyID:     params.PropertyID,
		Status:         RequisitionStatusDraft,
		CreatedBy:      actorID,
		Notes:          params.Notes,
		NewSupplierIDs: []string{},
	}

	var totalCost int64
	var newSupplierMap = make(map[string]bool)

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertRequisition(ctx, tx, req); err != nil {
			return err
		}

		for i, item := range params.Items {
			if item.Quantity <= 0 {
				return fmt.Errorf("%w: item[%d] quantity must be positive", ErrInvalidRequisition, i)
			}

			si, err := s.store.GetSupplierItem(ctx, tenantID, item.SupplierItemID)
			if err != nil {
				return fmt.Errorf("%w: item[%d] supplier_item %q: %w", ErrInvalidRequisition, i, item.SupplierItemID, err)
			}

			supplier, err := s.store.GetSupplier(ctx, tenantID, si.SupplierID)
			if err != nil {
				return err
			}

			if supplier.IsNew() {
				newSupplierMap[supplier.ID] = true
			}

			if currency == "" {
				currency = si.UnitCostCurrency
			} else if currency != si.UnitCostCurrency {
				return fmt.Errorf("%w: mismatched currencies within requisition (%s vs %s)", ErrInvalidRequisition, currency, si.UnitCostCurrency)
			}

			lineTotal := si.UnitCostMinorUnits * item.Quantity
			totalCost += lineTotal

			ri := &RequisitionItem{
				TenantID:            tenantID,
				RequisitionID:       req.ID,
				CatalogItemID:       si.CatalogItemID,
				SupplierItemID:      si.ID,
				Quantity:            item.Quantity,
				UnitCostMinorUnits:  si.UnitCostMinorUnits,
				UnitCostCurrency:    si.UnitCostCurrency,
				LineTotalMinorUnits: lineTotal,
			}
			if err := s.store.InsertRequisitionItem(ctx, tx, ri); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	newSupplierIDs := make([]string, 0, len(newSupplierMap))
	for sid := range newSupplierMap {
		newSupplierIDs = append(newSupplierIDs, sid)
	}

	updated, err := s.store.GetRequisition(ctx, tenantID, req.ID)
	if err != nil {
		return nil, err
	}
	updated.TotalCostMinorUnits = totalCost
	updated.Currency = currency
	updated.NewSupplierIDs = newSupplierIDs

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		updated, err = s.store.UpdateRequisitionTotal(ctx, tx, tenantID, req.ID, totalCost, currency)
		if err != nil {
			return err
		}
		nIDSJSON := marshalJSONInternal(newSupplierIDs)
		if nIDSJSON == nil {
			nIDSJSON = []byte("[]")
		}
		_, execErr := tx.Exec(ctx, `UPDATE requisitions SET new_supplier_ids = $3 WHERE id = $1 AND tenant_id = $2`,
			req.ID, tenantID, nIDSJSON)
		if execErr != nil {
			return execErr
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.requisition.created",
			ResourceType: "requisition",
			ResourceID:   req.ID,
			NewState:     marshalJSON(updated),
		})
	})
	if err != nil {
		return nil, err
	}

	items, _ := s.store.ListRequisitionItems(ctx, tenantID, req.ID)
	updated.Items = items

	return updated, nil
}

func (s *Service) GetRequisition(ctx context.Context, tenantID, requisitionID string) (*Requisition, error) {
	req, err := s.store.GetRequisition(ctx, tenantID, requisitionID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListRequisitionItems(ctx, tenantID, req.ID)
	if err != nil {
		return nil, err
	}
	req.Items = items
	return req, nil
}

func (s *Service) ListRequisitions(ctx context.Context, tenantID, propertyID string) ([]Requisition, error) {
	reqs, err := s.store.ListRequisitions(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	for i := range reqs {
		items, err := s.store.ListRequisitionItems(ctx, tenantID, reqs[i].ID)
		if err != nil {
			return nil, err
		}
		reqs[i].Items = items
	}
	return reqs, nil
}

func (s *Service) ApproveRequisition(ctx context.Context, tenantID, requisitionID string, params ApproveRequisitionParams) (*Requisition, error) {
	if params.ActorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidRequisition)
	}

	req, err := s.store.GetRequisition(ctx, tenantID, requisitionID)
	if err != nil {
		return nil, err
	}

	if req.Status != RequisitionStatusPendingApproval {
		return nil, fmt.Errorf("%w: requisition status is %q", ErrRequisitionNotPending, req.Status)
	}

	if params.ActorID == req.CreatedBy {
		return nil, ErrSelfApprovalDenied
	}

	if params.IsAIActor {
		return nil, ErrAICannotApprove
	}

	if len(req.NewSupplierIDs) > 0 {
		return nil, fmt.Errorf("%w: requisition contains %d new supplier(s) that require human approval", ErrNewSupplierRequiresHuman, len(req.NewSupplierIDs))
	}

	approval := &RequisitionApproval{
		TenantID:      tenantID,
		RequisitionID: requisitionID,
		ActorID:       params.ActorID,
		Decision:      ApprovalDecisionApproved,
		Reason:        params.Reason,
		IsAIActor:     params.IsAIActor,
	}

	var approved *Requisition
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertApproval(ctx, tx, approval); err != nil {
			return err
		}
		approved, err = s.store.UpdateRequisitionStatus(ctx, tx, tenantID, requisitionID, RequisitionStatusApproved, params.ActorID, "")
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "procurement.requisition.approved",
			ResourceType: "requisition",
			ResourceID:   requisitionID,
			NewState:     marshalJSON(approved),
		})
	})
	if err != nil {
		return nil, err
	}

	items, _ := s.store.ListRequisitionItems(ctx, tenantID, approved.ID)
	approved.Items = items

	return approved, nil
}

func (s *Service) RejectRequisition(ctx context.Context, tenantID, requisitionID string, params RejectRequisitionParams) (*Requisition, error) {
	if params.ActorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidRequisition)
	}

	req, err := s.store.GetRequisition(ctx, tenantID, requisitionID)
	if err != nil {
		return nil, err
	}

	if req.Status != RequisitionStatusPendingApproval {
		return nil, fmt.Errorf("%w: requisition status is %q", ErrRequisitionNotPending, req.Status)
	}

	if params.ActorID == req.CreatedBy {
		return nil, ErrSelfApprovalDenied
	}

	if params.IsAIActor {
		return nil, ErrAICannotApprove
	}

	approval := &RequisitionApproval{
		TenantID:      tenantID,
		RequisitionID: requisitionID,
		ActorID:       params.ActorID,
		Decision:      ApprovalDecisionRejected,
		Reason:        params.Reason,
		IsAIActor:     params.IsAIActor,
	}

	var rejected *Requisition
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertApproval(ctx, tx, approval); err != nil {
			return err
		}
		rejected, err = s.store.UpdateRequisitionStatus(ctx, tx, tenantID, requisitionID, RequisitionStatusRejected, "", params.ActorID)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "procurement.requisition.rejected",
			ResourceType: "requisition",
			ResourceID:   requisitionID,
			NewState:     marshalJSON(rejected),
		})
	})
	if err != nil {
		return nil, err
	}

	return rejected, nil
}

func (s *Service) GetRequisitionApprovals(ctx context.Context, tenantID, requisitionID string) ([]RequisitionApproval, error) {
	return s.store.ListApprovals(ctx, tenantID, requisitionID)
}

func (s *Service) CreatePurchaseOrder(ctx context.Context, tenantID, requisitionID string, params CreatePurchaseOrderParams, actorID string) (*PurchaseOrder, error) {
	req, err := s.store.GetRequisition(ctx, tenantID, requisitionID)
	if err != nil {
		return nil, err
	}

	if req.Status != RequisitionStatusApproved {
		return nil, fmt.Errorf("%w: requisition must be approved (current: %s)", ErrInvalidPurchaseOrder, req.Status)
	}

	supplier, err := s.store.GetSupplier(ctx, tenantID, params.SupplierID)
	if err != nil {
		return nil, err
	}
	if supplier.Status != SupplierStatusActive {
		return nil, fmt.Errorf("%w: supplier must be active (current: %s)", ErrInvalidPurchaseOrder, supplier.Status)
	}

	po := &PurchaseOrder{
		TenantID:         tenantID,
		RequisitionID:    requisitionID,
		SupplierID:       params.SupplierID,
		Status:           PurchaseOrderStatusDraft,
		OrderedBy:        params.OrderedBy,
		Currency:         req.Currency,
		OrderDate:        time.Now(),
		ExpectedDelivery: params.ExpectedDelivery,
	}

	var total int64
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertPurchaseOrder(ctx, tx, po); err != nil {
			return err
		}

		items, err := s.store.ListRequisitionItems(ctx, tenantID, requisitionID)
		if err != nil {
			return err
		}

		for _, ri := range items {
			poi := &PurchaseOrderItem{
				TenantID:            tenantID,
				PurchaseOrderID:     po.ID,
				RequisitionItemID:   ri.ID,
				CatalogItemID:       ri.CatalogItemID,
				Quantity:            ri.Quantity,
				UnitCostMinorUnits:  ri.UnitCostMinorUnits,
				Currency:            ri.UnitCostCurrency,
				LineTotalMinorUnits: ri.LineTotalMinorUnits,
			}
			if err := s.store.InsertPurchaseOrderItem(ctx, tx, poi); err != nil {
				return err
			}
			total += ri.LineTotalMinorUnits
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	updatedPO, err := s.store.GetPurchaseOrder(ctx, tenantID, po.ID)
	if err != nil {
		return nil, err
	}
	items, _ := s.store.ListPurchaseOrderItems(ctx, tenantID, updatedPO.ID)
	updatedPO.Items = items
	updatedPO.TotalMinorUnits = total

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE purchase_orders SET total_minor_units = $3 WHERE id = $1 AND tenant_id = $2`,
			po.ID, tenantID, total)
		if execErr != nil {
			return execErr
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.purchase_order.created",
			ResourceType: "purchase_order",
			ResourceID:   po.ID,
			NewState:     marshalJSON(updatedPO),
		})
	})
	if err != nil {
		return nil, err
	}

	return updatedPO, nil
}

func (s *Service) IssuePurchaseOrder(ctx context.Context, tenantID, poID, actorID string) (*PurchaseOrder, error) {
	po, err := s.store.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return nil, err
	}
	if po.Status != PurchaseOrderStatusDraft {
		return nil, fmt.Errorf("%w: purchase order must be in draft status (current: %s)", ErrInvalidPurchaseOrder, po.Status)
	}

	var issued *PurchaseOrder
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		issued, err = s.store.UpdatePurchaseOrderStatus(ctx, tx, tenantID, poID, PurchaseOrderStatusIssued)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE requisitions SET status = $3, updated_at = NOW() WHERE id = $1 AND tenant_id = $2`,
			po.RequisitionID, tenantID, RequisitionStatusOrdered)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.purchase_order.issued",
			ResourceType: "purchase_order",
			ResourceID:   poID,
			NewState:     marshalJSON(issued),
		})
	})
	if err != nil {
		return nil, err
	}

	items, _ := s.store.ListPurchaseOrderItems(ctx, tenantID, issued.ID)
	issued.Items = items

	return issued, nil
}

func (s *Service) GetPurchaseOrder(ctx context.Context, tenantID, poID string) (*PurchaseOrder, error) {
	po, err := s.store.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListPurchaseOrderItems(ctx, tenantID, po.ID)
	if err != nil {
		return nil, err
	}
	po.Items = items
	return po, nil
}

func (s *Service) ListPurchaseOrders(ctx context.Context, tenantID, requisitionID string) ([]PurchaseOrder, error) {
	return s.store.ListPurchaseOrders(ctx, tenantID, requisitionID)
}

func (s *Service) ReceiveGoods(ctx context.Context, tenantID, poID string, params CreateGoodsReceiptParams, actorID string) (*GoodsReceipt, error) {
	po, err := s.store.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return nil, err
	}

	if po.Status == PurchaseOrderStatusCancelled {
		return nil, fmt.Errorf("%w: cannot receive against a cancelled purchase order", ErrInvalidGoodsReceipt)
	}
	if po.Status == PurchaseOrderStatusDraft {
		return nil, fmt.Errorf("%w: cannot receive against a draft purchase order", ErrInvalidGoodsReceipt)
	}

	if !ValidCondition(params.Condition) {
		return nil, fmt.Errorf("%w: invalid condition %q", ErrInvalidGoodsReceipt, params.Condition)
	}

	if len(params.Items) == 0 {
		return nil, fmt.Errorf("%w: at least one receipt item is required", ErrInvalidGoodsReceipt)
	}

	poItems, err := s.store.ListPurchaseOrderItems(ctx, tenantID, poID)
	if err != nil {
		return nil, err
	}

	poItemMap := make(map[string]*PurchaseOrderItem)
	for i := range poItems {
		poItemMap[poItems[i].ID] = &poItems[i]
	}

	for _, ri := range params.Items {
		poi, ok := poItemMap[ri.PurchaseOrderItemID]
		if !ok {
			return nil, fmt.Errorf("%w: purchase order item %q not found in order %s", ErrInvalidGoodsReceipt, ri.PurchaseOrderItemID, poID)
		}
		if ri.QuantityReceived <= 0 {
			return nil, fmt.Errorf("%w: quantity_received must be positive for item %s", ErrInvalidGoodsReceipt, ri.PurchaseOrderItemID)
		}
		_ = poi
	}

	gr := &GoodsReceipt{
		TenantID:        tenantID,
		PurchaseOrderID: poID,
		ReceivedBy:      params.ReceivedBy,
		Status:          ReceiptStatusDraft,
		Condition:       params.Condition,
		ConditionNotes:  params.ConditionNotes,
		EvidenceRef:     params.EvidenceRef,
		ReceivedAt:      time.Now(),
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertGoodsReceipt(ctx, tx, gr); err != nil {
			return err
		}

		for _, ri := range params.Items {
			poi := poItemMap[ri.PurchaseOrderItemID]
			gri := &GoodsReceiptItem{
				TenantID:            tenantID,
				GoodsReceiptID:      gr.ID,
				PurchaseOrderItemID: ri.PurchaseOrderItemID,
				CatalogItemID:       poi.CatalogItemID,
				QuantityOrdered:     poi.Quantity,
				QuantityReceived:    ri.QuantityReceived,
			}
			if err := s.store.InsertGoodsReceiptItem(ctx, tx, gri); err != nil {
				return err
			}
		}

		if params.Condition == ConditionGood {
			if params.EvidenceRef == "" {
				return fmt.Errorf("%w: evidence_ref is required when accepting goods (condition: good)", ErrInvalidGoodsReceipt)
			}
			gr.Status = ReceiptStatusReceived
			if _, err := s.store.UpdatePurchaseOrderStatus(ctx, tx, tenantID, poID, PurchaseOrderStatusReceived); err != nil {
				return err
			}
		} else if params.Condition == ConditionDamaged || params.Condition == ConditionShort || params.Condition == ConditionWrongItem {
			gr.Status = ReceiptStatusQuarantined
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.goods_receipt.created",
			ResourceType: "goods_receipt",
			ResourceID:   gr.ID,
			NewState:     marshalJSON(gr),
		})
	})
	if err != nil {
		return nil, err
	}

	created, err := s.store.GetGoodsReceipt(ctx, tenantID, gr.ID)
	if err != nil {
		return nil, err
	}
	griItems, _ := s.store.ListGoodsReceiptItems(ctx, tenantID, created.ID)
	created.Items = griItems

	return created, nil
}

func (s *Service) GetGoodsReceipt(ctx context.Context, tenantID, receiptID string) (*GoodsReceipt, error) {
	gr, err := s.store.GetGoodsReceipt(ctx, tenantID, receiptID)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListGoodsReceiptItems(ctx, tenantID, gr.ID)
	if err != nil {
		return nil, err
	}
	gr.Items = items
	return gr, nil
}

func (s *Service) ListGoodsReceipts(ctx context.Context, tenantID, poID string) ([]GoodsReceipt, error) {
	return s.store.ListGoodsReceipts(ctx, tenantID, poID)
}

func (s *Service) CreateRebate(ctx context.Context, tenantID, supplierID, poID string, params CreateRebateParams, actorID string) (*SupplierRebate, error) {
	if params.Description == "" {
		return nil, fmt.Errorf("%w: description is required", ErrInvalidRebate)
	}
	if params.AmountMinorUnits < 0 {
		return nil, fmt.Errorf("%w: amount must not be negative", ErrInvalidRebate)
	}
	if !ValidCurrency(params.Currency) {
		return nil, fmt.Errorf("%w: invalid currency %q", ErrInvalidRebate, params.Currency)
	}

	if supplierID != "" {
		if _, err := s.store.GetSupplier(ctx, tenantID, supplierID); err != nil {
			return nil, fmt.Errorf("%w: supplier %q: %w", ErrInvalidRebate, supplierID, err)
		}
	}

	if poID != "" {
		if _, err := s.store.GetPurchaseOrder(ctx, tenantID, poID); err != nil {
			return nil, fmt.Errorf("%w: purchase order %q: %w", ErrInvalidRebate, poID, err)
		}
	}

	rebate := &SupplierRebate{
		TenantID:         tenantID,
		SupplierID:       supplierID,
		PurchaseOrderID:  poID,
		Description:      params.Description,
		AmountMinorUnits: params.AmountMinorUnits,
		Currency:         params.Currency,
		Status:           RebateStatusOffered,
		OfferedAt:        time.Now(),
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertRebate(ctx, tx, rebate); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.rebate.created",
			ResourceType: "supplier_rebate",
			ResourceID:   rebate.ID,
			NewState:     marshalJSON(rebate),
		})
	}); err != nil {
		return nil, err
	}

	return rebate, nil
}

func (s *Service) GetRebate(ctx context.Context, tenantID, rebateID string) (*SupplierRebate, error) {
	return s.store.GetRebate(ctx, tenantID, rebateID)
}

func (s *Service) ListRebates(ctx context.Context, tenantID, supplierID string) ([]SupplierRebate, error) {
	return s.store.ListRebates(ctx, tenantID, supplierID)
}

func (s *Service) SettleRebate(ctx context.Context, tenantID, rebateID, actorID string) (*SupplierRebate, error) {
	rebate, err := s.store.GetRebate(ctx, tenantID, rebateID)
	if err != nil {
		return nil, err
	}
	if rebate.Status != RebateStatusAccepted {
		return nil, fmt.Errorf("%w: rebate must be accepted before settling (current: %s)", ErrInvalidRebate, rebate.Status)
	}

	var settled *SupplierRebate
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		settled, err = s.store.UpdateRebateStatus(ctx, tx, tenantID, rebateID, RebateStatusSettled)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "procurement.rebate.settled",
			ResourceType: "supplier_rebate",
			ResourceID:   rebateID,
			NewState:     marshalJSON(settled),
		})
	})
	if err != nil {
		return nil, err
	}

	return settled, nil
}

func (s *Service) CalculateReorderBasis(ctx context.Context, tenantID, locationID string, stockLevels []StockLevel, supplierItems []SupplierItem, leadTimeDays int, safetyStockDays int) []ReorderBasis {
	var basis []ReorderBasis

	stockMap := make(map[string]int64)
	for _, sl := range stockLevels {
		stockMap[sl.CatalogItemID] = sl.Balance
	}

	supplierItemMap := make(map[string]SupplierItem)
	for _, si := range supplierItems {
		supplierItemMap[si.CatalogItemID] = si
	}

	for _, si := range supplierItems {
		currentStock := stockMap[si.CatalogItemID]

		avgDailyUsage := 0.0
		var reorderPoint int64
		if si.LeadTimeDays > 0 || leadTimeDays > 0 {
			effectiveLead := si.LeadTimeDays
			if effectiveLead == 0 {
				effectiveLead = leadTimeDays
			}
			reorderPoint = int64(float64(effectiveLead+safetyStockDays) * avgDailyUsage)

			if avgDailyUsage > 0 {
				reorderPoint = int64(float64(effectiveLead+safetyStockDays) * avgDailyUsage)
			} else {
				reorderPoint = si.MinimumOrderQuantity
			}
		} else {
			reorderPoint = si.MinimumOrderQuantity
		}

		reorderQuantity := si.MinimumOrderQuantity
		if currentStock < reorderPoint && reorderPoint > 0 {
			reorderQuantity = reorderPoint - currentStock + si.MinimumOrderQuantity
		}

		basis = append(basis, ReorderBasis{
			CatalogItemID:       si.CatalogItemID,
			CurrentStock:        currentStock,
			AverageDailyUsage:   avgDailyUsage,
			LeadTimeDays:        si.LeadTimeDays,
			SafetyStockDays:     safetyStockDays,
			ReorderPoint:        reorderPoint,
			ReorderQuantity:     reorderQuantity,
			SupplierItemID:      si.ID,
			UnitCostMinorUnits:  si.UnitCostMinorUnits,
			UnitCostCurrency:    si.UnitCostCurrency,
			LineTotalMinorUnits: si.UnitCostMinorUnits * reorderQuantity,
		})
	}

	return basis
}

func marshalJSONInternal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	if data == nil {
		return []byte("[]")
	}
	return data
}
