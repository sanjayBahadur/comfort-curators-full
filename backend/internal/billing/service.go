package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func (s *Service) appendAuditTx(ctx context.Context, q querier, event audit.AuditEvent) error {
	if s.auditStore == nil {
		return nil
	}
	if event.ID == "" {
		event.ID = newID("aud")
	}
	return s.auditStore.AppendTx(ctx, q, event)
}

func ComputeIdempotencyHash(key string, body []byte) string {
	h := sha256.Sum256(append([]byte(key), body...))
	return hex.EncodeToString(h[:])
}

// ============================================================
// Charges
// ============================================================

func (s *Service) CreateCharge(ctx context.Context, tenantID string, params CreateChargeParams, actorID string) (*Charge, error) {
	if params.PropertyID == "" {
		return nil, fmt.Errorf("%w: property_id is required", ErrInvalidCharge)
	}
	if !ValidChargeType(params.ChargeType) {
		return nil, fmt.Errorf("%w: invalid charge_type %q", ErrInvalidCharge, params.ChargeType)
	}
	if params.AmountMinorUnits < 0 {
		return nil, ErrNegativeAmount
	}
	if !ValidCurrency(params.Currency) {
		return nil, fmt.Errorf("%w: invalid currency %q", ErrInvalidCharge, params.Currency)
	}
	if params.IdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidCharge)
	}

	existing, err := s.store.GetChargeByIdempotencyKey(ctx, s.store.pool, tenantID, params.IdempotencyKey)
	if err != nil && err != ErrChargeNotFound {
		return nil, err
	}
	if existing != nil {
		if existing.AmountMinorUnits != params.AmountMinorUnits ||
			existing.Currency != params.Currency ||
			existing.ChargeType != params.ChargeType {
			return nil, ErrDuplicateCharge
		}
		return existing, nil
	}

	charge := &Charge{
		TenantID:         tenantID,
		PropertyID:       params.PropertyID,
		ChargeType:       params.ChargeType,
		AmountMinorUnits: params.AmountMinorUnits,
		Currency:         params.Currency,
		Reason:           params.Reason,
		Data:             params.Data,
		ContractRuleID:   params.ContractRuleID,
		EvidenceID:       params.EvidenceID,
		TicketID:         params.TicketID,
		OrderID:          params.OrderID,
		ApprovalID:       params.ApprovalID,
		IdempotencyKey:   params.IdempotencyKey,
		Status:           ChargeStatusPending,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCharge(ctx, tx, charge); err != nil {
			return err
		}
		subledgerEntry := &OperationalSubledgerEntry{
			TenantID:         tenantID,
			PropertyID:       params.PropertyID,
			EntryType:        SubledgerEntryTypeCharge,
			AmountMinorUnits: params.AmountMinorUnits,
			Currency:         params.Currency,
			ReferenceType:    "charge",
			ReferenceID:      charge.ID,
			Description:      fmt.Sprintf("charge: %s", params.ChargeType),
		}
		if err := s.store.InsertSubledgerEntry(ctx, tx, subledgerEntry); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "billing.charge.created",
			ResourceType: "charge",
			ResourceID:   charge.ID,
			NewState:     marshalJSON(charge),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return charge, nil
}

func (s *Service) GetCharge(ctx context.Context, tenantID, chargeID string) (*Charge, error) {
	return s.store.GetCharge(ctx, tenantID, chargeID)
}

func (s *Service) ListCharges(ctx context.Context, tenantID, propertyID string) ([]Charge, error) {
	return s.store.ListCharges(ctx, tenantID, propertyID)
}

// ApplyCharge posts a pending charge so it is counted as real revenue in
// contribution and monthly reports. Every charge is created "pending"
// (CreateCharge) so it can be reviewed before it affects any report; this is
// the one path that moves it to "applied". Applying an already-applied or
// corrected charge is a no-op that returns the charge unchanged rather than
// erroring, so a retried request is safe.
func (s *Service) ApplyCharge(ctx context.Context, tenantID, chargeID, actorID string) (*Charge, error) {
	existing, err := s.store.GetCharge(ctx, tenantID, chargeID)
	if err != nil {
		return nil, err
	}
	if existing.Status != ChargeStatusPending {
		return existing, nil
	}

	charge, err := s.store.UpdateChargeStatus(ctx, s.pool, tenantID, chargeID, ChargeStatusApplied)
	if err != nil {
		return nil, err
	}

	s.appendAudit(ctx, audit.AuditEvent{
		EventType:    audit.EventTypeMutation,
		TenantID:     tenantID,
		ActorID:      actorID,
		Action:       "billing.charge.applied",
		ResourceType: "charge",
		ResourceID:   charge.ID,
		NewState:     marshalJSON(charge),
	})

	return charge, nil
}

// ============================================================
// Invoices
// ============================================================

func (s *Service) IssueInvoice(ctx context.Context, tenantID string, params CreateInvoiceParams, actorID string) (*Invoice, error) {
	if params.PropertyID == "" {
		return nil, fmt.Errorf("%w: property_id is required", ErrInvalidInvoice)
	}
	if !ValidCurrency(params.Currency) {
		return nil, fmt.Errorf("%w: invalid currency %q", ErrInvalidInvoice, params.Currency)
	}
	if params.IdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidInvoice)
	}

	existing, err := s.store.GetInvoiceByIdempotencyKey(ctx, s.store.pool, tenantID, params.IdempotencyKey)
	if err != nil && err != ErrInvoiceNotFound {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	var totalMinorUnits int64
	for _, line := range params.Lines {
		if line.AmountMinorUnits < 0 {
			return nil, fmt.Errorf("%w: invoice line amount must not be negative", ErrInvalidInvoice)
		}
		totalMinorUnits += line.AmountMinorUnits
	}

	invoice := &Invoice{
		TenantID:        tenantID,
		PropertyID:      params.PropertyID,
		PeriodStart:     params.PeriodStart,
		PeriodEnd:       params.PeriodEnd,
		TotalMinorUnits: totalMinorUnits,
		Currency:        params.Currency,
		Status:          InvoiceStatusIssued,
		IdempotencyKey:  params.IdempotencyKey,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertInvoice(ctx, tx, invoice); err != nil {
			return err
		}
		for _, line := range params.Lines {
			il := &InvoiceLine{
				InvoiceID:        invoice.ID,
				TenantID:         tenantID,
				ChargeType:       line.ChargeType,
				Description:      line.Description,
				AmountMinorUnits: line.AmountMinorUnits,
				Currency:         params.Currency,
				ContractRuleID:   line.ContractRuleID,
				TicketID:         line.TicketID,
				OrderID:          line.OrderID,
				AdjustmentID:     line.AdjustmentID,
			}
			if err := s.store.InsertInvoiceLine(ctx, tx, il); err != nil {
				return err
			}
		}
		subledgerEntry := &OperationalSubledgerEntry{
			TenantID:         tenantID,
			PropertyID:       params.PropertyID,
			EntryType:        SubledgerEntryTypeCharge,
			AmountMinorUnits: totalMinorUnits,
			Currency:         params.Currency,
			ReferenceType:    "invoice",
			ReferenceID:      invoice.ID,
			Description:      fmt.Sprintf("invoice issued with %d lines", len(params.Lines)),
		}
		if err := s.store.InsertSubledgerEntry(ctx, tx, subledgerEntry); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "billing.invoice.issued",
			ResourceType: "invoice",
			ResourceID:   invoice.ID,
			NewState:     marshalJSON(invoice),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return invoice, nil
}

func (s *Service) GetInvoice(ctx context.Context, tenantID, invoiceID string) (*Invoice, error) {
	return s.store.GetInvoice(ctx, tenantID, invoiceID)
}

func (s *Service) ListInvoices(ctx context.Context, tenantID, propertyID string) ([]Invoice, error) {
	return s.store.ListInvoices(ctx, tenantID, propertyID)
}

func (s *Service) GetInvoiceLines(ctx context.Context, tenantID, invoiceID string) ([]InvoiceLine, error) {
	return s.store.ListInvoiceLines(ctx, tenantID, invoiceID)
}

// ============================================================
// Credits (financial corrections that preserve original entries)
// ============================================================

func (s *Service) IssueCredit(ctx context.Context, tenantID string, params CreateCreditParams, actorID string) (*Credit, error) {
	if params.PropertyID == "" {
		return nil, fmt.Errorf("%w: property_id is required", ErrInvalidCredit)
	}
	if !ValidCreditType(params.CreditType) {
		return nil, fmt.Errorf("%w: invalid credit_type %q", ErrInvalidCredit, params.CreditType)
	}
	if params.AmountMinorUnits < 0 {
		return nil, ErrNegativeAmount
	}
	if !ValidCurrency(params.Currency) {
		return nil, fmt.Errorf("%w: invalid currency %q", ErrInvalidCredit, params.Currency)
	}
	if params.OriginalEntryID == "" {
		return nil, fmt.Errorf("%w: original_entry_id is required", ErrOriginalEntryPreserved)
	}
	if params.IdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidCredit)
	}

	existing, err := s.store.GetCreditByIdempotencyKey(ctx, s.store.pool, tenantID, params.IdempotencyKey)
	if err != nil && err != ErrCreditNotFound {
		return nil, err
	}
	if existing != nil {
		if existing.AmountMinorUnits != params.AmountMinorUnits ||
			existing.Currency != params.Currency ||
			existing.CreditType != params.CreditType {
			return nil, ErrDuplicateCredit
		}
		return existing, nil
	}

	credit := &Credit{
		TenantID:          tenantID,
		PropertyID:        params.PropertyID,
		CreditType:        params.CreditType,
		AmountMinorUnits:  params.AmountMinorUnits,
		Currency:          params.Currency,
		Reason:            params.Reason,
		OriginalEntryID:   params.OriginalEntryID,
		OriginalEntryType: params.OriginalEntryType,
		Data:              params.Data,
		IdempotencyKey:    params.IdempotencyKey,
		Status:            CreditStatusIssued,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCredit(ctx, tx, credit); err != nil {
			return err
		}
		subledgerEntry := &OperationalSubledgerEntry{
			TenantID:         tenantID,
			PropertyID:       params.PropertyID,
			EntryType:        SubledgerEntryTypeCredit,
			AmountMinorUnits: -params.AmountMinorUnits,
			Currency:         params.Currency,
			ReferenceType:    "credit",
			ReferenceID:      credit.ID,
			Description:      fmt.Sprintf("credit %s preserves original entry %s", params.CreditType, params.OriginalEntryID),
		}
		if err := s.store.InsertSubledgerEntry(ctx, tx, subledgerEntry); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "billing.credit.issued",
			ResourceType: "credit",
			ResourceID:   credit.ID,
			NewState:     marshalJSON(credit),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return credit, nil
}

func (s *Service) GetCredit(ctx context.Context, tenantID, creditID string) (*Credit, error) {
	return s.store.GetCredit(ctx, tenantID, creditID)
}

func (s *Service) ListCredits(ctx context.Context, tenantID, propertyID string) ([]Credit, error) {
	return s.store.ListCredits(ctx, tenantID, propertyID)
}

// ============================================================
// Operational Subledger
// ============================================================

func (s *Service) ListSubledgerEntries(ctx context.Context, tenantID, propertyID string) ([]OperationalSubledgerEntry, error) {
	return s.store.ListSubledgerEntries(ctx, tenantID, propertyID)
}

// ============================================================
// Accounting Exports
// ============================================================

func (s *Service) CreateAccountingExport(ctx context.Context, tenantID string, params CreateAccountingExportParams, actorID string) (*AccountingExport, error) {
	if params.Format == "" {
		params.Format = "journal_csv"
	}

	export := &AccountingExport{
		TenantID:    tenantID,
		PeriodStart: params.PeriodStart,
		PeriodEnd:   params.PeriodEnd,
		Format:      params.Format,
		Status:      ExportStatusRequested,
		RequestedBy: actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertAccountingExport(ctx, tx, export); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "billing.accounting_export.created",
			ResourceType: "accounting_export",
			ResourceID:   export.ID,
			NewState:     marshalJSON(export),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return export, nil
}

func (s *Service) GetAccountingExport(ctx context.Context, tenantID, exportID string) (*AccountingExport, error) {
	return s.store.GetAccountingExport(ctx, tenantID, exportID)
}

// ============================================================
// Financial Approvals
// ============================================================

func (s *Service) DecideFinancialApproval(ctx context.Context, tenantID, approvalID string, params CreateFinancialApprovalParams) (*FinancialApproval, error) {
	if params.Decision != FinancialApprovalStatusApproved && params.Decision != FinancialApprovalStatusRejected {
		return nil, fmt.Errorf("%w: invalid decision %q", ErrInvalidFinancialApproval, params.Decision)
	}
	if params.ApproverID == "" {
		return nil, fmt.Errorf("%w: approver_id is required", ErrInvalidFinancialApproval)
	}

	if params.RequestID != "" {
		req, err := s.store.GetMakerCheckerRequest(ctx, tenantID, params.RequestID)
		if err == nil && req.CreatedBy == params.ApproverID {
			return nil, ErrSelfApprovalDenied
		}
	}

	approval := &FinancialApproval{
		TenantID:   tenantID,
		RequestID:  params.RequestID,
		ApproverID: params.ApproverID,
		Decision:   params.Decision,
		Reason:     params.Reason,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertFinancialApproval(ctx, tx, approval); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ApproverID,
			Action:       "billing.financial_approval." + params.Decision,
			ResourceType: "financial_approval",
			ResourceID:   approval.ID,
			NewState:     marshalJSON(approval),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return approval, nil
}

// ============================================================
// Property Contribution Report
// ============================================================

func (s *Service) GetPropertyContributionReport(ctx context.Context, tenantID, propertyID string) (map[string]any, error) {
	charges, err := s.store.ListCharges(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	credits, err := s.store.ListCredits(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	entries, err := s.store.ListSubledgerEntries(ctx, tenantID, propertyID)
	if err != nil {
		return nil, err
	}

	var totalCharges int64
	var totalCredits int64
	currency := "INR"

	for _, c := range charges {
		totalCharges += c.AmountMinorUnits
		if c.Currency != "" {
			currency = c.Currency
		}
	}
	for _, c := range credits {
		totalCredits += c.AmountMinorUnits
	}

	return map[string]any{
		"property_id":               propertyID,
		"total_charges_minor_units": totalCharges,
		"total_credits_minor_units": totalCredits,
		"net_minor_units":           totalCharges - totalCredits,
		"currency":                  currency,
		"charge_count":              len(charges),
		"credit_count":              len(credits),
		"subledger_entry_count":     len(entries),
	}, nil
}

// ============================================================
// Maker-Checker Requests
// ============================================================

func (s *Service) CreateMakerCheckerRequest(ctx context.Context, tenantID string, params CreateMakerCheckerRequestParams, actorID string) (*MakerCheckerRequest, error) {
	if !ValidMakerCheckerRequestType(params.RequestType) {
		return nil, fmt.Errorf("%w: invalid request_type %q", ErrInvalidMakerCheckerRequest, params.RequestType)
	}
	if actorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidMakerCheckerRequest)
	}

	if params.IdempotencyKey != "" {
		existing, err := s.store.GetMakerCheckerRequestByIdempotencyKey(ctx, s.store.pool, tenantID, params.IdempotencyKey)
		if err != nil && err != ErrMakerCheckerRequestNotFound {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	status := RequestStatusDraft
	if params.RequiresVerification {
		status = RequestStatusPendingVerification
	}

	request := &MakerCheckerRequest{
		TenantID:             tenantID,
		RequestType:          params.RequestType,
		PropertyID:           params.PropertyID,
		Status:               status,
		CreatedBy:            actorID,
		Payload:              params.Payload,
		IdempotencyKey:       params.IdempotencyKey,
		RequiresVerification: params.RequiresVerification,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertMakerCheckerRequest(ctx, tx, request); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "billing.maker_checker_request.created",
			ResourceType: "maker_checker_request",
			ResourceID:   request.ID,
			NewState:     marshalJSON(request),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return request, nil
}

func (s *Service) SubmitMakerCheckerRequest(ctx context.Context, tenantID, requestID string, params SubmitMakerCheckerRequestParams) (*MakerCheckerRequest, error) {
	req, err := s.store.GetMakerCheckerRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}

	if req.Status != RequestStatusDraft && req.Status != RequestStatusPendingVerification {
		return nil, fmt.Errorf("%w: current status is %q", ErrInvalidMakerCheckerRequest, req.Status)
	}

	if req.CreatedBy != params.ActorID {
		return nil, fmt.Errorf("%w: only the creator can submit their own request", ErrInvalidMakerCheckerRequest)
	}

	if req.RequiresVerification {
		return nil, ErrBankVerificationRequired
	}

	var submitted *MakerCheckerRequest
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		submitted, err = s.store.SubmitMakerCheckerRequest(ctx, tx, tenantID, requestID, params.ActorID)
		if err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "billing.maker_checker_request.submitted",
			ResourceType: "maker_checker_request",
			ResourceID:   requestID,
			NewState:     marshalJSON(submitted),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return submitted, nil
}

func (s *Service) DecideMakerCheckerRequest(ctx context.Context, tenantID, requestID string, params DecideMakerCheckerRequestParams) (*MakerCheckerRequest, error) {
	if params.Decision != RequestStatusApproved && params.Decision != RequestStatusRejected {
		return nil, fmt.Errorf("%w: invalid decision %q", ErrInvalidMakerCheckerRequest, params.Decision)
	}
	if params.ActorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidMakerCheckerRequest)
	}

	req, err := s.store.GetMakerCheckerRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}

	if req.Status != RequestStatusPendingApproval {
		return nil, fmt.Errorf("%w: current status is %q", ErrRequestNotPendingApproval, req.Status)
	}

	if params.ActorID == req.CreatedBy {
		return nil, ErrSelfApprovalDenied
	}

	if params.IsAIActor {
		return nil, ErrAICannotPost
	}

	var approvedBy, rejectedBy string
	if params.Decision == RequestStatusApproved {
		approvedBy = params.ActorID
	} else {
		rejectedBy = params.ActorID
	}

	var decided *MakerCheckerRequest
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		decided, err = s.store.UpdateMakerCheckerRequestStatus(ctx, tx, tenantID, requestID, params.Decision, approvedBy, rejectedBy)
		if err != nil {
			return err
		}

		approval := &FinancialApproval{
			TenantID:   tenantID,
			RequestID:  requestID,
			ApproverID: params.ActorID,
			Decision:   params.Decision,
			Reason:     params.Reason,
		}
		if err := s.store.InsertFinancialApproval(ctx, tx, approval); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "billing.maker_checker_request." + params.Decision,
			ResourceType: "maker_checker_request",
			ResourceID:   requestID,
			NewState:     marshalJSON(decided),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return decided, nil
}

func (s *Service) GetMakerCheckerRequest(ctx context.Context, tenantID, requestID string) (*MakerCheckerRequest, error) {
	return s.store.GetMakerCheckerRequest(ctx, tenantID, requestID)
}

func (s *Service) ListMakerCheckerRequests(ctx context.Context, tenantID, propertyID string) ([]MakerCheckerRequest, error) {
	return s.store.ListMakerCheckerRequests(ctx, tenantID, propertyID)
}

// ============================================================
// Bank Verification
// ============================================================

func (s *Service) CreateBankVerification(ctx context.Context, tenantID string, params CreateBankVerificationParams) (*BankVerification, error) {
	req, err := s.store.GetMakerCheckerRequest(ctx, tenantID, params.RequestID)
	if err != nil {
		return nil, fmt.Errorf("%w: request not found", ErrInvalidMakerCheckerRequest)
	}

	if req.RequestType != MakerCheckerRequestTypeBankChange {
		return nil, fmt.Errorf("%w: verification only applies to bank_change requests", ErrInvalidMakerCheckerRequest)
	}

	if !req.RequiresVerification {
		return nil, fmt.Errorf("%w: request does not require verification", ErrInvalidMakerCheckerRequest)
	}

	token := newID("bvkt")
	bv := &BankVerification{
		TenantID:          tenantID,
		RequestID:         params.RequestID,
		VerificationToken: token,
		Status:            BankVerificationStatusPending,
		ExpiresAt:         time.Now().UTC().Add(7 * 24 * time.Hour),
	}

	if err := s.store.InsertBankVerification(ctx, s.store.pool, bv); err != nil {
		return nil, err
	}

	return bv, nil
}

func (s *Service) ConfirmBankVerification(ctx context.Context, tenantID, verificationID string, params ConfirmBankVerificationParams) (*BankVerification, error) {
	bv, err := s.store.GetBankVerification(ctx, tenantID, verificationID)
	if err != nil {
		return nil, err
	}

	if bv.Status != BankVerificationStatusPending {
		return nil, fmt.Errorf("%w: verification is %q", ErrBankVerificationExpired, bv.Status)
	}

	if time.Now().UTC().After(bv.ExpiresAt) {
		if _, err := s.store.ExpireBankVerification(ctx, s.store.pool, tenantID, verificationID); err != nil {
			logging.Error(ctx, "failed to expire bank verification", "error", err)
		}
		return nil, ErrBankVerificationExpired
	}

	if bv.VerificationToken != params.Token {
		return nil, fmt.Errorf("%w: invalid verification token", ErrBankVerificationExpired)
	}

	var verified *BankVerification
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		verified, err = s.store.ConfirmBankVerification(ctx, tx, tenantID, verificationID, "external_verifier")
		if err != nil {
			return err
		}

		req, fetchErr := s.store.GetMakerCheckerRequest(ctx, tenantID, bv.RequestID)
		if fetchErr != nil {
			return fetchErr
		}
		if req.Status == RequestStatusPendingVerification {
			if _, updateErr := s.store.UpdateMakerCheckerRequestStatus(ctx, tx, tenantID, bv.RequestID, RequestStatusPendingApproval, "", ""); updateErr != nil {
				return updateErr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return verified, nil
}

func (s *Service) GetBankVerification(ctx context.Context, tenantID, verificationID string) (*BankVerification, error) {
	return s.store.GetBankVerification(ctx, tenantID, verificationID)
}

// ============================================================
// Journal Finalization with AI Guard
// ============================================================

func (s *Service) FinalizeJournalPosting(ctx context.Context, tenantID string, params FinalizeJournalParams) (*AccountingExport, error) {
	if params.ExportID == "" {
		return nil, fmt.Errorf("%w: export_id is required", ErrInvalidAccountingExport)
	}

	if params.IsAIActor {
		return nil, ErrAICannotPost
	}

	export, err := s.store.GetAccountingExport(ctx, tenantID, params.ExportID)
	if err != nil {
		return nil, err
	}

	if export.Status == ExportStatusCompleted {
		return export, nil
	}

	updated := *export
	updated.Status = ExportStatusCompleted
	updated.UpdatedAt = time.Now().UTC()

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "billing.journal.finalized",
			ResourceType: "accounting_export",
			ResourceID:   export.ID,
			NewState:     marshalJSON(updated),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &updated, nil
}

// ============================================================
// Reconciliation Exceptions
// ============================================================

func (s *Service) RecordReconciliationException(ctx context.Context, tenantID string, params CreateReconciliationExceptionParams, actorID string) (*ReconciliationException, error) {
	if params.PropertyID == "" {
		return nil, fmt.Errorf("%w: property_id is required", ErrInvalidReconciliationException)
	}
	if !ValidExceptionType(params.ExceptionType) {
		return nil, fmt.Errorf("%w: invalid exception_type %q", ErrInvalidReconciliationException, params.ExceptionType)
	}
	if actorID == "" {
		return nil, fmt.Errorf("%w: recorded_by is required", ErrInvalidReconciliationException)
	}

	exception := &ReconciliationException{
		TenantID:      tenantID,
		PropertyID:    params.PropertyID,
		EntryID:       params.EntryID,
		EntryType:     params.EntryType,
		ExceptionType: params.ExceptionType,
		Description:   params.Description,
		Status:        ExceptionStatusOpen,
		RecordedBy:    actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertReconciliationException(ctx, tx, exception); err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "billing.reconciliation_exception.recorded",
			ResourceType: "reconciliation_exception",
			ResourceID:   exception.ID,
			NewState:     marshalJSON(exception),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return exception, nil
}

func (s *Service) ResolveReconciliationException(ctx context.Context, tenantID, exceptionID string, params ResolveReconciliationExceptionParams) (*ReconciliationException, error) {
	exception, err := s.store.GetReconciliationException(ctx, tenantID, exceptionID)
	if err != nil {
		return nil, err
	}

	if exception.Status == ExceptionStatusResolved {
		return exception, nil
	}

	if params.ActorID == exception.RecordedBy {
		return nil, ErrSelfApprovalDenied
	}

	var resolved *ReconciliationException
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		resolved, err = s.store.ResolveReconciliationException(ctx, tx, tenantID, exceptionID, params.ActorID)
		if err != nil {
			return err
		}
		if err := s.appendAuditTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      params.ActorID,
			Action:       "billing.reconciliation_exception.resolved",
			ResourceType: "reconciliation_exception",
			ResourceID:   exceptionID,
			NewState:     marshalJSON(resolved),
		}); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return resolved, nil
}

func (s *Service) GetReconciliationException(ctx context.Context, tenantID, exceptionID string) (*ReconciliationException, error) {
	return s.store.GetReconciliationException(ctx, tenantID, exceptionID)
}

func (s *Service) ListReconciliationExceptions(ctx context.Context, tenantID, propertyID string) ([]ReconciliationException, error) {
	return s.store.ListReconciliationExceptions(ctx, tenantID, propertyID)
}

func MustMarshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
