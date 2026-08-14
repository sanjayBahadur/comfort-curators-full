package procurement

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"comfort-curators-backend/internal/iam"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/procurement/suppliers", h.handleCreateSupplier)
	mux.HandleFunc("GET /v1/procurement/suppliers", h.handleListSuppliers)
	mux.HandleFunc("GET /v1/procurement/suppliers/{supplier_id}", h.handleGetSupplier)
	mux.HandleFunc("POST /v1/procurement/suppliers/{supplier_id}/approve", h.handleApproveSupplier)
	mux.HandleFunc("POST /v1/procurement/suppliers/{supplier_id}/reject", h.handleRejectSupplier)

	mux.HandleFunc("POST /v1/procurement/suppliers/{supplier_id}/items", h.handleCreateSupplierItem)
	mux.HandleFunc("GET /v1/procurement/suppliers/{supplier_id}/items", h.handleListSupplierItems)
	mux.HandleFunc("GET /v1/procurement/supplier-items/{item_id}", h.handleGetSupplierItem)

	mux.HandleFunc("POST /v1/procurement/requisitions", h.handleCreateRequisition)
	mux.HandleFunc("GET /v1/procurement/requisitions", h.handleListRequisitions)
	mux.HandleFunc("GET /v1/procurement/requisitions/{requisition_id}", h.handleGetRequisition)
	mux.HandleFunc("POST /v1/procurement/requisitions/{requisition_id}/approve", h.handleApproveRequisition)
	mux.HandleFunc("POST /v1/procurement/requisitions/{requisition_id}/reject", h.handleRejectRequisition)
	mux.HandleFunc("GET /v1/procurement/requisitions/{requisition_id}/approvals", h.handleGetRequisitionApprovals)

	mux.HandleFunc("POST /v1/procurement/purchase-orders", h.handleCreatePurchaseOrder)
	mux.HandleFunc("GET /v1/procurement/purchase-orders", h.handleListPurchaseOrders)
	mux.HandleFunc("GET /v1/procurement/purchase-orders/{po_id}", h.handleGetPurchaseOrder)
	mux.HandleFunc("POST /v1/procurement/purchase-orders/{po_id}/issue", h.handleIssuePurchaseOrder)

	mux.HandleFunc("POST /v1/procurement/purchase-orders/{po_id}/receipts", h.handleReceiveGoods)
	mux.HandleFunc("GET /v1/procurement/receipts/{receipt_id}", h.handleGetGoodsReceipt)
	mux.HandleFunc("GET /v1/procurement/purchase-orders/{po_id}/receipts", h.handleListGoodsReceipts)

	mux.HandleFunc("POST /v1/procurement/rebates", h.handleCreateRebate)
	mux.HandleFunc("GET /v1/procurement/rebates", h.handleListRebates)
	mux.HandleFunc("GET /v1/procurement/rebates/{rebate_id}", h.handleGetRebate)
	mux.HandleFunc("POST /v1/procurement/rebates/{rebate_id}/settle", h.handleSettleRebate)
}

type procurementResource struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Data    any    `json:"data"`
}

type procurementError struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

func subjectFromRequest(r *http.Request) (tenantID, actorID string, err error) {
	subject, ok := iam.SubjectFromContext(r.Context())
	if !ok || subject.TenantID == "" {
		return "", "", errors.New("unauthenticated")
	}
	return subject.TenantID, subject.ActorID, nil
}

func apiError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := r.Header.Get("X-Correlation-ID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(procurementError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	})
}

func apiResource(w http.ResponseWriter, status int, id string, version int64, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(procurementResource{
		ID:      id,
		Version: version,
		Data:    data,
	})
}

func apiCollection(w http.ResponseWriter, items []procurementResource) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

func (h *Handler) handleCreateSupplier(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		Name        string `json:"name"`
		ContactInfo string `json:"contact_info"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	supplier, err := h.svc.CreateSupplier(r.Context(), tenantID, CreateSupplierParams{
		Name:        req.Name,
		ContactInfo: req.ContactInfo,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidSupplier) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, supplier.ID, supplier.Version, supplierView(supplier))
}

func (h *Handler) handleListSuppliers(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	suppliers, err := h.svc.ListSuppliers(r.Context(), tenantID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]procurementResource, 0, len(suppliers))
	for i := range suppliers {
		items = append(items, procurementResource{
			ID:      suppliers[i].ID,
			Version: suppliers[i].Version,
			Data:    supplierView(&suppliers[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetSupplier(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	supplierID := r.PathValue("supplier_id")
	supplier, err := h.svc.GetSupplier(r.Context(), tenantID, supplierID)
	if err != nil {
		if errors.Is(err, ErrSupplierNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "supplier not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, supplier.ID, supplier.Version, supplierView(supplier))
}

func (h *Handler) handleApproveSupplier(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	supplierID := r.PathValue("supplier_id")
	supplier, err := h.svc.ApproveSupplier(r.Context(), tenantID, supplierID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrSupplierNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrSupplierAlreadyApproved) || errors.Is(err, ErrSupplierAlreadyRejected) {
			status = http.StatusConflict
			code = "CONFLICT"
		} else if errors.Is(err, ErrInvalidSupplier) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, supplier.ID, supplier.Version, supplierView(supplier))
}

func (h *Handler) handleRejectSupplier(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	supplierID := r.PathValue("supplier_id")
	supplier, err := h.svc.RejectSupplier(r.Context(), tenantID, supplierID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrSupplierNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrSupplierAlreadyApproved) || errors.Is(err, ErrSupplierAlreadyRejected) {
			status = http.StatusConflict
			code = "CONFLICT"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, supplier.ID, supplier.Version, supplierView(supplier))
}

func (h *Handler) handleCreateSupplierItem(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	supplierID := r.PathValue("supplier_id")

	var req struct {
		CatalogItemID        string `json:"catalog_item_id"`
		SupplierSKU          string `json:"supplier_sku"`
		UnitCostMinorUnits   int64  `json:"unit_cost_minor_units"`
		UnitCostCurrency     string `json:"unit_cost_currency"`
		LeadTimeDays         int    `json:"lead_time_days"`
		MinimumOrderQuantity int64  `json:"minimum_order_quantity"`
		IsPreferred          bool   `json:"is_preferred"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	si, err := h.svc.CreateSupplierItem(r.Context(), tenantID, supplierID, CreateSupplierItemParams{
		CatalogItemID:        req.CatalogItemID,
		SupplierSKU:          req.SupplierSKU,
		UnitCostMinorUnits:   req.UnitCostMinorUnits,
		UnitCostCurrency:     req.UnitCostCurrency,
		LeadTimeDays:         req.LeadTimeDays,
		MinimumOrderQuantity: req.MinimumOrderQuantity,
		IsPreferred:          req.IsPreferred,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidSupplierItem) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrSupplierNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, si.ID, si.Version, supplierItemView(si))
}

func (h *Handler) handleListSupplierItems(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	supplierID := r.PathValue("supplier_id")
	items, err := h.svc.ListSupplierItems(r.Context(), tenantID, supplierID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	out := make([]procurementResource, 0, len(items))
	for i := range items {
		out = append(out, procurementResource{
			ID:      items[i].ID,
			Version: items[i].Version,
			Data:    supplierItemView(&items[i]),
		})
	}
	apiCollection(w, out)
}

func (h *Handler) handleGetSupplierItem(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	itemID := r.PathValue("item_id")
	si, err := h.svc.GetSupplierItem(r.Context(), tenantID, itemID)
	if err != nil {
		if errors.Is(err, ErrSupplierItemNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "supplier item not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, si.ID, si.Version, supplierItemView(si))
}

func (h *Handler) handleCreateRequisition(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		PropertyID string                 `json:"property_id"`
		Notes      string                 `json:"notes"`
		Items      []RequisitionItemInput `json:"items"`
		IsAIActor  bool                   `json:"is_ai_actor"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	requisition, err := h.svc.CreateRequisition(r.Context(), tenantID, CreateRequisitionParams{
		PropertyID: req.PropertyID,
		Notes:      req.Notes,
		Items:      req.Items,
	}, actorID, req.IsAIActor)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidRequisition) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrAICannotApprove) {
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, requisition.ID, requisition.Version, requisitionView(requisition))
}

func (h *Handler) handleListRequisitions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	propertyID := r.URL.Query().Get("property_id")
	requisitions, err := h.svc.ListRequisitions(r.Context(), tenantID, propertyID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]procurementResource, 0, len(requisitions))
	for i := range requisitions {
		items = append(items, procurementResource{
			ID:      requisitions[i].ID,
			Version: requisitions[i].Version,
			Data:    requisitionView(&requisitions[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetRequisition(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requisitionID := r.PathValue("requisition_id")
	requisition, err := h.svc.GetRequisition(r.Context(), tenantID, requisitionID)
	if err != nil {
		if errors.Is(err, ErrRequisitionNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "requisition not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, requisition.ID, requisition.Version, requisitionView(requisition))
}

func (h *Handler) handleApproveRequisition(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requisitionID := r.PathValue("requisition_id")

	var req struct {
		IsAIActor bool   `json:"is_ai_actor"`
		Reason    string `json:"reason"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)

	requisition, err := h.svc.ApproveRequisition(r.Context(), tenantID, requisitionID, ApproveRequisitionParams{
		ActorID:   actorID,
		IsAIActor: req.IsAIActor,
		Reason:    req.Reason,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrRequisitionNotFound) || errors.Is(err, ErrRequisitionNotPending) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrSelfApprovalDenied) || errors.Is(err, ErrAICannotApprove) {
			status = http.StatusForbidden
			code = "FORBIDDEN"
		} else if errors.Is(err, ErrNewSupplierRequiresHuman) {
			status = http.StatusUnprocessableEntity
			code = "NEW_SUPPLIER_APPROVAL_REQUIRED"
		} else if errors.Is(err, ErrInvalidRequisition) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, requisition.ID, requisition.Version, requisitionView(requisition))
}

func (h *Handler) handleRejectRequisition(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requisitionID := r.PathValue("requisition_id")

	var req struct {
		IsAIActor bool   `json:"is_ai_actor"`
		Reason    string `json:"reason"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)

	requisition, err := h.svc.RejectRequisition(r.Context(), tenantID, requisitionID, RejectRequisitionParams{
		ActorID:   actorID,
		IsAIActor: req.IsAIActor,
		Reason:    req.Reason,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrRequisitionNotFound) || errors.Is(err, ErrRequisitionNotPending) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrSelfApprovalDenied) || errors.Is(err, ErrAICannotApprove) {
			status = http.StatusForbidden
			code = "FORBIDDEN"
		} else if errors.Is(err, ErrInvalidRequisition) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, requisition.ID, requisition.Version, requisitionView(requisition))
}

func (h *Handler) handleGetRequisitionApprovals(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requisitionID := r.PathValue("requisition_id")
	approvals, err := h.svc.GetRequisitionApprovals(r.Context(), tenantID, requisitionID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]procurementResource, 0, len(approvals))
	for i := range approvals {
		items = append(items, procurementResource{
			ID:      approvals[i].ID,
			Version: 1,
			Data:    approvalView(&approvals[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleCreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		RequisitionID    string `json:"requisition_id"`
		SupplierID       string `json:"supplier_id"`
		OrderedBy        string `json:"ordered_by"`
		ExpectedDelivery string `json:"expected_delivery"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	var delivery *time.Time
	if req.ExpectedDelivery != "" {
		t, err := time.Parse(time.RFC3339, req.ExpectedDelivery)
		if err != nil {
			apiError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid expected_delivery, use RFC3339")
			return
		}
		delivery = &t
	}

	po, err := h.svc.CreatePurchaseOrder(r.Context(), tenantID, req.RequisitionID, CreatePurchaseOrderParams{
		SupplierID:       req.SupplierID,
		OrderedBy:        req.OrderedBy,
		ExpectedDelivery: delivery,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidPurchaseOrder) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrRequisitionNotFound) || errors.Is(err, ErrSupplierNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, po.ID, po.Version, purchaseOrderView(po))
}

func (h *Handler) handleListPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	requisitionID := r.URL.Query().Get("requisition_id")
	pos, err := h.svc.ListPurchaseOrders(r.Context(), tenantID, requisitionID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]procurementResource, 0, len(pos))
	for i := range pos {
		items = append(items, procurementResource{
			ID:      pos[i].ID,
			Version: pos[i].Version,
			Data:    purchaseOrderView(&pos[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	poID := r.PathValue("po_id")
	po, err := h.svc.GetPurchaseOrder(r.Context(), tenantID, poID)
	if err != nil {
		if errors.Is(err, ErrPurchaseOrderNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "purchase order not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, po.ID, po.Version, purchaseOrderView(po))
}

func (h *Handler) handleIssuePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	poID := r.PathValue("po_id")
	po, err := h.svc.IssuePurchaseOrder(r.Context(), tenantID, poID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrPurchaseOrderNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrInvalidPurchaseOrder) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, po.ID, po.Version, purchaseOrderView(po))
}

func (h *Handler) handleReceiveGoods(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	poID := r.PathValue("po_id")

	var req struct {
		ReceivedBy     string                  `json:"received_by"`
		Condition      string                  `json:"condition"`
		ConditionNotes string                  `json:"condition_notes"`
		EvidenceRef    string                  `json:"evidence_ref"`
		Items          []GoodsReceiptItemInput `json:"items"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	gr, err := h.svc.ReceiveGoods(r.Context(), tenantID, poID, CreateGoodsReceiptParams{
		ReceivedBy:     req.ReceivedBy,
		Condition:      req.Condition,
		ConditionNotes: req.ConditionNotes,
		EvidenceRef:    req.EvidenceRef,
		Items:          req.Items,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidGoodsReceipt) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrPurchaseOrderNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, gr.ID, gr.Version, goodsReceiptView(gr))
}

func (h *Handler) handleGetGoodsReceipt(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	receiptID := r.PathValue("receipt_id")
	gr, err := h.svc.GetGoodsReceipt(r.Context(), tenantID, receiptID)
	if err != nil {
		if errors.Is(err, ErrGoodsReceiptNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "goods receipt not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, gr.ID, gr.Version, goodsReceiptView(gr))
}

func (h *Handler) handleListGoodsReceipts(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	poID := r.PathValue("po_id")
	receipts, err := h.svc.ListGoodsReceipts(r.Context(), tenantID, poID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]procurementResource, 0, len(receipts))
	for i := range receipts {
		items = append(items, procurementResource{
			ID:      receipts[i].ID,
			Version: receipts[i].Version,
			Data:    goodsReceiptView(&receipts[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleCreateRebate(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req struct {
		SupplierID       string `json:"supplier_id"`
		PurchaseOrderID  string `json:"purchase_order_id"`
		Description      string `json:"description"`
		AmountMinorUnits int64  `json:"amount_minor_units"`
		Currency         string `json:"currency"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	rebate, err := h.svc.CreateRebate(r.Context(), tenantID, req.SupplierID, req.PurchaseOrderID, CreateRebateParams{
		Description:      req.Description,
		AmountMinorUnits: req.AmountMinorUnits,
		Currency:         req.Currency,
	}, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrInvalidRebate) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		} else if errors.Is(err, ErrSupplierNotFound) || errors.Is(err, ErrPurchaseOrderNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusCreated, rebate.ID, rebate.Version, rebateView(rebate))
}

func (h *Handler) handleListRebates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	supplierID := r.URL.Query().Get("supplier_id")
	rebates, err := h.svc.ListRebates(r.Context(), tenantID, supplierID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]procurementResource, 0, len(rebates))
	for i := range rebates {
		items = append(items, procurementResource{
			ID:      rebates[i].ID,
			Version: rebates[i].Version,
			Data:    rebateView(&rebates[i]),
		})
	}
	apiCollection(w, items)
}

func (h *Handler) handleGetRebate(w http.ResponseWriter, r *http.Request) {
	tenantID, _, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	rebateID := r.PathValue("rebate_id")
	rebate, err := h.svc.GetRebate(r.Context(), tenantID, rebateID)
	if err != nil {
		if errors.Is(err, ErrRebateNotFound) {
			apiError(w, r, http.StatusNotFound, "NOT_FOUND", "supplier rebate not found")
			return
		}
		apiError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	apiResource(w, http.StatusOK, rebate.ID, rebate.Version, rebateView(rebate))
}

func (h *Handler) handleSettleRebate(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, err := subjectFromRequest(r)
	if err != nil {
		apiError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	rebateID := r.PathValue("rebate_id")
	rebate, err := h.svc.SettleRebate(r.Context(), tenantID, rebateID, actorID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if errors.Is(err, ErrRebateNotFound) {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		} else if errors.Is(err, ErrInvalidRebate) {
			status = http.StatusUnprocessableEntity
			code = "VALIDATION_ERROR"
		}
		apiError(w, r, status, code, err.Error())
		return
	}

	apiResource(w, http.StatusOK, rebate.ID, rebate.Version, rebateView(rebate))
}

func supplierView(s *Supplier) map[string]any {
	v := map[string]any{
		"id":           s.ID,
		"tenant_id":    s.TenantID,
		"name":         s.Name,
		"contact_info": s.ContactInfo,
		"status":       s.Status,
		"created_by":   s.CreatedBy,
		"approved_by":  s.ApprovedBy,
		"version":      s.Version,
		"created_at":   s.CreatedAt.Format(time.RFC3339),
		"updated_at":   s.UpdatedAt.Format(time.RFC3339),
	}
	if s.ApprovedAt != nil {
		v["approved_at"] = s.ApprovedAt.Format(time.RFC3339)
	}
	return v
}

func supplierItemView(si *SupplierItem) map[string]any {
	return map[string]any{
		"id":                     si.ID,
		"tenant_id":              si.TenantID,
		"supplier_id":            si.SupplierID,
		"catalog_item_id":        si.CatalogItemID,
		"supplier_sku":           si.SupplierSKU,
		"unit_cost_minor_units":  si.UnitCostMinorUnits,
		"unit_cost_currency":     si.UnitCostCurrency,
		"lead_time_days":         si.LeadTimeDays,
		"minimum_order_quantity": si.MinimumOrderQuantity,
		"is_preferred":           si.IsPreferred,
		"version":                si.Version,
		"created_at":             si.CreatedAt.Format(time.RFC3339),
		"updated_at":             si.UpdatedAt.Format(time.RFC3339),
	}
}

func requisitionView(r *Requisition) map[string]any {
	itemViews := make([]map[string]any, 0, len(r.Items))
	for i := range r.Items {
		itemViews = append(itemViews, requisitionItemView(&r.Items[i]))
	}

	v := map[string]any{
		"id":                     r.ID,
		"tenant_id":              r.TenantID,
		"property_id":            r.PropertyID,
		"status":                 r.Status,
		"created_by":             r.CreatedBy,
		"approved_by":            r.ApprovedBy,
		"rejected_by":            r.RejectedBy,
		"total_cost_minor_units": r.TotalCostMinorUnits,
		"currency":               r.Currency,
		"notes":                  r.Notes,
		"new_supplier_ids":       r.NewSupplierIDs,
		"version":                r.Version,
		"created_at":             r.CreatedAt.Format(time.RFC3339),
		"updated_at":             r.UpdatedAt.Format(time.RFC3339),
		"items":                  itemViews,
	}
	return v
}

func requisitionItemView(ri *RequisitionItem) map[string]any {
	return map[string]any{
		"id":                     ri.ID,
		"tenant_id":              ri.TenantID,
		"requisition_id":         ri.RequisitionID,
		"catalog_item_id":        ri.CatalogItemID,
		"supplier_item_id":       ri.SupplierItemID,
		"quantity":               ri.Quantity,
		"unit_cost_minor_units":  ri.UnitCostMinorUnits,
		"unit_cost_currency":     ri.UnitCostCurrency,
		"line_total_minor_units": ri.LineTotalMinorUnits,
		"created_at":             ri.CreatedAt.Format(time.RFC3339),
	}
}

func approvalView(a *RequisitionApproval) map[string]any {
	return map[string]any{
		"id":             a.ID,
		"tenant_id":      a.TenantID,
		"requisition_id": a.RequisitionID,
		"actor_id":       a.ActorID,
		"decision":       a.Decision,
		"reason":         a.Reason,
		"is_ai_actor":    a.IsAIActor,
		"created_at":     a.CreatedAt.Format(time.RFC3339),
	}
}

func purchaseOrderView(po *PurchaseOrder) map[string]any {
	itemViews := make([]map[string]any, 0, len(po.Items))
	for i := range po.Items {
		itemViews = append(itemViews, purchaseOrderItemView(&po.Items[i]))
	}

	v := map[string]any{
		"id":                po.ID,
		"tenant_id":         po.TenantID,
		"requisition_id":    po.RequisitionID,
		"supplier_id":       po.SupplierID,
		"status":            po.Status,
		"ordered_by":        po.OrderedBy,
		"total_minor_units": po.TotalMinorUnits,
		"currency":          po.Currency,
		"order_date":        po.OrderDate.Format(time.RFC3339),
		"version":           po.Version,
		"created_at":        po.CreatedAt.Format(time.RFC3339),
		"updated_at":        po.UpdatedAt.Format(time.RFC3339),
		"items":             itemViews,
	}
	if po.ExpectedDelivery != nil {
		v["expected_delivery"] = po.ExpectedDelivery.Format(time.RFC3339)
	}
	return v
}

func purchaseOrderItemView(poi *PurchaseOrderItem) map[string]any {
	return map[string]any{
		"id":                     poi.ID,
		"tenant_id":              poi.TenantID,
		"purchase_order_id":      poi.PurchaseOrderID,
		"requisition_item_id":    poi.RequisitionItemID,
		"catalog_item_id":        poi.CatalogItemID,
		"quantity":               poi.Quantity,
		"unit_cost_minor_units":  poi.UnitCostMinorUnits,
		"currency":               poi.Currency,
		"line_total_minor_units": poi.LineTotalMinorUnits,
		"created_at":             poi.CreatedAt.Format(time.RFC3339),
	}
}

func goodsReceiptView(gr *GoodsReceipt) map[string]any {
	itemViews := make([]map[string]any, 0, len(gr.Items))
	for i := range gr.Items {
		itemViews = append(itemViews, goodsReceiptItemView(&gr.Items[i]))
	}

	return map[string]any{
		"id":                gr.ID,
		"tenant_id":         gr.TenantID,
		"purchase_order_id": gr.PurchaseOrderID,
		"received_by":       gr.ReceivedBy,
		"status":            gr.Status,
		"condition":         gr.Condition,
		"condition_notes":   gr.ConditionNotes,
		"evidence_ref":      gr.EvidenceRef,
		"received_at":       gr.ReceivedAt.Format(time.RFC3339),
		"version":           gr.Version,
		"created_at":        gr.CreatedAt.Format(time.RFC3339),
		"updated_at":        gr.UpdatedAt.Format(time.RFC3339),
		"items":             itemViews,
	}
}

func goodsReceiptItemView(gri *GoodsReceiptItem) map[string]any {
	return map[string]any{
		"id":                     gri.ID,
		"tenant_id":              gri.TenantID,
		"goods_receipt_id":       gri.GoodsReceiptID,
		"purchase_order_item_id": gri.PurchaseOrderItemID,
		"catalog_item_id":        gri.CatalogItemID,
		"quantity_ordered":       gri.QuantityOrdered,
		"quantity_received":      gri.QuantityReceived,
		"created_at":             gri.CreatedAt.Format(time.RFC3339),
	}
}

func rebateView(r *SupplierRebate) map[string]any {
	v := map[string]any{
		"id":                 r.ID,
		"tenant_id":          r.TenantID,
		"supplier_id":        r.SupplierID,
		"purchase_order_id":  r.PurchaseOrderID,
		"description":        r.Description,
		"amount_minor_units": r.AmountMinorUnits,
		"currency":           r.Currency,
		"status":             r.Status,
		"offered_at":         r.OfferedAt.Format(time.RFC3339),
		"version":            r.Version,
		"created_at":         r.CreatedAt.Format(time.RFC3339),
		"updated_at":         r.UpdatedAt.Format(time.RFC3339),
	}
	if r.SettledAt != nil {
		v["settled_at"] = r.SettledAt.Format(time.RFC3339)
	}
	return v
}
