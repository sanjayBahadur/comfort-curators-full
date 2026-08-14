package billing

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestMapError(t *testing.T) {
	cases := []struct {
		err       error
		wantCode  string
		wantState int
	}{
		{ErrChargeNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrInvoiceNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrCreditNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrSubledgerEntryNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrAccountingExportNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrFinancialApprovalNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrInvoiceAlreadyIssued, "INVOICE_ALREADY_ISSUED", http.StatusConflict},
		{ErrDuplicateCharge, "DUPLICATE", http.StatusConflict},
		{ErrDuplicateCredit, "DUPLICATE", http.StatusConflict},
		{ErrOriginalEntryPreserved, "VALIDATION_ERROR", http.StatusUnprocessableEntity},
		{ErrFloatNotAllowed, "VALIDATION_ERROR", http.StatusUnprocessableEntity},
		{ErrInvalidCharge, "VALIDATION_ERROR", http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		status, code := mapError(c.err)
		if code != c.wantCode || status != c.wantState {
			t.Fatalf("mapError(%v) = (%d, %s), want (%d, %s)", c.err, status, code, c.wantState, c.wantCode)
		}
	}
}

func TestChargeView(t *testing.T) {
	now := time.Now().UTC()
	c := &Charge{
		ID:               "chg-1",
		TenantID:         "tenant-1",
		PropertyID:       "prop-1",
		ChargeType:       ChargeTypeManagementFee,
		AmountMinorUnits: 25000,
		Currency:         "INR",
		Reason:           "Monthly management fee",
		ContractRuleID:   "rule-1",
		Status:           ChargeStatusPending,
		IdempotencyKey:   "ik-1",
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	v := chargeView(c)
	if v["id"] != "chg-1" {
		t.Fatalf("expected id chg-1, got %v", v["id"])
	}
	if v["amount_minor_units"] != int64(25000) {
		t.Fatalf("expected amount_minor_units 25000, got %v", v["amount_minor_units"])
	}
	if v["currency"] != "INR" {
		t.Fatalf("expected currency INR, got %v", v["currency"])
	}
	if v["charge_type"] != ChargeTypeManagementFee {
		t.Fatalf("expected charge_type management_fee, got %v", v["charge_type"])
	}
	if v["contract_rule_id"] != "rule-1" {
		t.Fatalf("expected contract_rule_id rule-1, got %v", v["contract_rule_id"])
	}
}

func TestInvoiceView(t *testing.T) {
	now := time.Now().UTC()
	ps := now.Add(-30 * 24 * time.Hour)
	pe := now
	inv := &Invoice{
		ID:              "inv-1",
		TenantID:        "tenant-1",
		PropertyID:      "prop-1",
		PeriodStart:     &ps,
		PeriodEnd:       &pe,
		TotalMinorUnits: 50000,
		Currency:        "INR",
		Status:          InvoiceStatusIssued,
		IdempotencyKey:  "ik-inv-1",
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	v := invoiceView(inv)
	if v["id"] != "inv-1" {
		t.Fatalf("expected id inv-1, got %v", v["id"])
	}
	if v["total_minor_units"] != int64(50000) {
		t.Fatalf("expected total_minor_units 50000, got %v", v["total_minor_units"])
	}
	if v["currency"] != "INR" {
		t.Fatalf("expected currency INR, got %v", v["currency"])
	}
	if v["status"] != InvoiceStatusIssued {
		t.Fatalf("expected status issued, got %v", v["status"])
	}
	if _, ok := v["period_start"]; !ok {
		t.Fatal("period_start must be present when set")
	}
	if _, ok := v["period_end"]; !ok {
		t.Fatal("period_end must be present when set")
	}

	inv2 := &Invoice{ID: "inv-2", Status: InvoiceStatusDraft}
	v2 := invoiceView(inv2)
	if _, ok := v2["period_start"]; ok {
		t.Fatal("period_start must be absent when nil")
	}
}

func TestCreditView(t *testing.T) {
	now := time.Now().UTC()
	c := &Credit{
		ID:                "crd-1",
		TenantID:          "tenant-1",
		PropertyID:        "prop-1",
		CreditType:        CreditTypeReversal,
		AmountMinorUnits:  10000,
		Currency:          "INR",
		Reason:            "Invoice correction",
		OriginalEntryID:   "inv-1",
		OriginalEntryType: "invoice",
		IdempotencyKey:    "ik-crd-1",
		Status:            CreditStatusIssued,
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	v := creditView(c)
	if v["credit_type"] != CreditTypeReversal {
		t.Fatalf("expected credit_type reversal, got %v", v["credit_type"])
	}
	if v["original_entry_id"] != "inv-1" {
		t.Fatalf("expected original_entry_id inv-1, got %v", v["original_entry_id"])
	}
	if v["original_entry_type"] != "invoice" {
		t.Fatalf("expected original_entry_type invoice, got %v", v["original_entry_type"])
	}
}

func TestFinancialApprovalView(t *testing.T) {
	now := time.Now().UTC()
	a := &FinancialApproval{
		ID:         "fap-1",
		TenantID:   "tenant-1",
		RequestID:  "req-1",
		ApproverID: "actor-1",
		Decision:   FinancialApprovalStatusApproved,
		Reason:     "Approved after review",
		CreatedAt:  now,
	}

	v := financialApprovalView(a)
	if v["decision"] != FinancialApprovalStatusApproved {
		t.Fatalf("expected decision approved, got %v", v["decision"])
	}
	if v["approver_id"] != "actor-1" {
		t.Fatalf("expected approver_id actor-1, got %v", v["approver_id"])
	}
}

func TestSentinelErrors(t *testing.T) {
	errs := []error{
		ErrChargeNotFound,
		ErrInvoiceNotFound,
		ErrCreditNotFound,
		ErrDuplicateCharge,
		ErrFloatNotAllowed,
		ErrOriginalEntryPreserved,
		ErrNegativeAmount,
	}
	for _, e := range errs {
		if !errors.Is(e, e) {
			t.Fatalf("%v must be self-wrapping", e)
		}
	}
}

func TestMapErrorMakerChecker(t *testing.T) {
	cases := []struct {
		err       error
		wantCode  string
		wantState int
	}{
		{ErrMakerCheckerRequestNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrBankVerificationNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrReconciliationExceptionNotFound, "NOT_FOUND", http.StatusNotFound},
		{ErrSelfApprovalDenied, "FORBIDDEN", http.StatusForbidden},
		{ErrAICannotPost, "FORBIDDEN", http.StatusForbidden},
		{ErrRequestNotPendingApproval, "CONFLICT", http.StatusConflict},
		{ErrRequestNotPendingVerification, "CONFLICT", http.StatusConflict},
		{ErrBankVerificationRequired, "VERIFICATION_REQUIRED", http.StatusUnprocessableEntity},
		{ErrBankVerificationExpired, "VERIFICATION_REQUIRED", http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		status, code := mapError(c.err)
		if code != c.wantCode || status != c.wantState {
			t.Fatalf("mapError(%v) = (%d, %s), want (%d, %s)", c.err, status, code, c.wantState, c.wantCode)
		}
	}
}

func TestMakerCheckerRequestView(t *testing.T) {
	now := time.Now().UTC()
	rr := &MakerCheckerRequest{
		ID:                   "mcr-1",
		TenantID:             "tenant-1",
		RequestType:          MakerCheckerRequestTypeBankChange,
		PropertyID:           "prop-1",
		Status:               RequestStatusPendingApproval,
		CreatedBy:            "actor-1",
		ApprovedBy:           "actor-2",
		RequiresVerification: true,
		Version:              1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	v := makerCheckerRequestView(rr)
	if v["id"] != "mcr-1" {
		t.Fatalf("expected id mcr-1, got %v", v["id"])
	}
	if v["request_type"] != MakerCheckerRequestTypeBankChange {
		t.Fatalf("expected request_type bank_change, got %v", v["request_type"])
	}
	if v["status"] != RequestStatusPendingApproval {
		t.Fatalf("expected status pending_approval, got %v", v["status"])
	}
	if v["created_by"] != "actor-1" {
		t.Fatalf("expected created_by actor-1, got %v", v["created_by"])
	}
	if v["approved_by"] != "actor-2" {
		t.Fatalf("expected approved_by actor-2, got %v", v["approved_by"])
	}
	if v["requires_verification"] != true {
		t.Fatalf("expected requires_verification true, got %v", v["requires_verification"])
	}
}

func TestBankVerificationView(t *testing.T) {
	bv := &BankVerification{
		ID:     "bv-1",
		Status: BankVerificationStatusPending,
	}

	v := bankVerificationView(bv)
	if v["id"] != "bv-1" {
		t.Fatalf("expected id bv-1, got %v", v["id"])
	}
	if v["status"] != BankVerificationStatusPending {
		t.Fatalf("expected status pending, got %v", v["status"])
	}
}

func TestReconciliationExceptionView(t *testing.T) {
	now := time.Now().UTC()
	re := &ReconciliationException{
		ID:            "re-1",
		TenantID:      "tenant-1",
		PropertyID:    "prop-1",
		EntryID:       "entry-1",
		EntryType:     "charge",
		ExceptionType: ExceptionTypeAmountMismatch,
		Description:   "Amount mismatch in subledger",
		Status:        ExceptionStatusOpen,
		RecordedBy:    "actor-1",
		CreatedAt:     now,
	}

	v := reconciliationExceptionView(re)
	if v["id"] != "re-1" {
		t.Fatalf("expected id re-1, got %v", v["id"])
	}
	if v["exception_type"] != ExceptionTypeAmountMismatch {
		t.Fatalf("expected exception_type amount_mismatch, got %v", v["exception_type"])
	}
	if v["status"] != ExceptionStatusOpen {
		t.Fatalf("expected status open, got %v", v["status"])
	}
	if v["recorded_by"] != "actor-1" {
		t.Fatalf("expected recorded_by actor-1, got %v", v["recorded_by"])
	}
}

func TestMakerCheckerRequestTypesValid(t *testing.T) {
	types := []string{
		MakerCheckerRequestTypeVendorCreation,
		MakerCheckerRequestTypeBankChange,
		MakerCheckerRequestTypePurchase,
		MakerCheckerRequestTypePayment,
		MakerCheckerRequestTypeJournalPosting,
	}
	for _, rt := range types {
		if !ValidMakerCheckerRequestType(rt) {
			t.Fatalf("%s must be valid", rt)
		}
	}
	if ValidMakerCheckerRequestType("invalid") {
		t.Fatal("invalid must not be a valid request type")
	}
}

func TestSelfApprovalDeniedError(t *testing.T) {
	if !errors.Is(ErrSelfApprovalDenied, ErrSelfApprovalDenied) {
		t.Fatal("ErrSelfApprovalDenied must be self-wrapping")
	}
	if errors.Is(ErrSelfApprovalDenied, ErrAICannotPost) {
		t.Fatal("ErrSelfApprovalDenied should not be ErrAICannotPost")
	}
}

func TestAICannotPostError(t *testing.T) {
	if !errors.Is(ErrAICannotPost, ErrAICannotPost) {
		t.Fatal("ErrAICannotPost must be self-wrapping")
	}
}
