package billing

import (
	"errors"
	"testing"
)

func TestValidateMoney(t *testing.T) {
	err := ValidateMoney(Money{MinorUnits: 1000, Currency: "INR"})
	if err != nil {
		t.Fatalf("valid money must not error, got %v", err)
	}

	err = ValidateMoney(Money{MinorUnits: 1000, Currency: ""})
	if err == nil {
		t.Fatal("missing currency must fail")
	}

	err = ValidateMoney(Money{MinorUnits: 1000, Currency: "RUPEES"})
	if err == nil {
		t.Fatal("invalid currency must fail")
	}
}

func TestMoneyNeverUsesFloat(t *testing.T) {
	amount := Money{MinorUnits: 15250, Currency: "INR"}
	if amount.MinorUnits != 15250 {
		t.Fatalf("expected minor_units 15250, got %d", amount.MinorUnits)
	}
	if _, ok := any(amount.MinorUnits).(int64); !ok {
		t.Fatal("amount.MinorUnits must be int64, never float")
	}
}

func TestCurrencyValidation(t *testing.T) {
	if !ValidCurrency("INR") {
		t.Fatal("INR must be a valid currency")
	}
	if !ValidCurrency("USD") {
		t.Fatal("USD must be a valid currency")
	}
	if ValidCurrency("Rupee") {
		t.Fatal("Rupee must not be a valid currency")
	}
	if ValidCurrency("inr") {
		t.Fatal("lowercase must not be a valid currency")
	}
	if ValidCurrency("IN") {
		t.Fatal("two-char must not be a valid currency")
	}
}

func TestChargeTypes(t *testing.T) {
	types := []string{
		ChargeTypeManagementFee,
		ChargeTypeTaskService,
		ChargeTypePurchasedGoods,
		ChargeTypeReimbursement,
		ChargeTypeVendorFee,
		ChargeTypeDiscount,
		ChargeTypeRebate,
		ChargeTypeTax,
		ChargeTypeRefund,
		ChargeTypeCredit,
	}
	for _, ct := range types {
		if !ValidChargeType(ct) {
			t.Fatalf("%s must be a valid charge type", ct)
		}
	}
	if ValidChargeType("bogus") {
		t.Fatal("bogus must not be a valid charge type")
	}
}

func TestCreditTypes(t *testing.T) {
	types := []string{
		CreditTypeReversal,
		CreditTypeCreditNote,
		CreditTypeRefund,
		CreditTypeDiscount,
	}
	for _, ct := range types {
		if !ValidCreditType(ct) {
			t.Fatalf("%s must be a valid credit type", ct)
		}
	}
	if ValidCreditType("bogus") {
		t.Fatal("bogus must not be a valid credit type")
	}
}

func TestInvoiceStatuses(t *testing.T) {
	if !ValidInvoiceStatus(InvoiceStatusDraft) {
		t.Fatal("draft must be valid")
	}
	if !ValidInvoiceStatus(InvoiceStatusIssued) {
		t.Fatal("issued must be valid")
	}
	if !ValidInvoiceStatus(InvoiceStatusVoid) {
		t.Fatal("void must be valid")
	}
	if !ValidInvoiceStatus(InvoiceStatusPaid) {
		t.Fatal("paid must be valid")
	}
	if ValidInvoiceStatus("bogus") {
		t.Fatal("bogus must not be valid")
	}
}

func TestNegativeAmountsRejected(t *testing.T) {
	if !errors.Is(ErrNegativeAmount, ErrNegativeAmount) {
		t.Fatal("ErrNegativeAmount must be self-wrapping")
	}
}

func TestOriginalEntryPreserved(t *testing.T) {
	if !errors.Is(ErrOriginalEntryPreserved, ErrOriginalEntryPreserved) {
		t.Fatal("ErrOriginalEntryPreserved must be self-wrapping")
	}
}

func TestChargeStatuses(t *testing.T) {
	if !ValidChargeStatus(ChargeStatusPending) {
		t.Fatal("pending must be valid")
	}
	if !ValidChargeStatus(ChargeStatusApplied) {
		t.Fatal("applied must be valid")
	}
	if !ValidChargeStatus(ChargeStatusCorrected) {
		t.Fatal("corrected must be valid")
	}
	if ValidChargeStatus("deleted") {
		t.Fatal("deleted must not be valid (no hard deletes)")
	}
}

func TestSubledgerEntryTypes(t *testing.T) {
	types := []string{
		SubledgerEntryTypeCharge,
		SubledgerEntryTypePayment,
		SubledgerEntryTypeCredit,
		SubledgerEntryTypeRefund,
	}
	for _, et := range types {
		if !ValidSubledgerEntryType(et) {
			t.Fatalf("%s must be a valid subledger entry type", et)
		}
	}
	if ValidSubledgerEntryType("bogus") {
		t.Fatal("bogus must not be valid")
	}
}

func TestExportStatuses(t *testing.T) {
	if !ValidExportStatus(ExportStatusRequested) {
		t.Fatal("requested must be valid")
	}
	if !ValidExportStatus(ExportStatusProcessing) {
		t.Fatal("processing must be valid")
	}
	if !ValidExportStatus(ExportStatusCompleted) {
		t.Fatal("completed must be valid")
	}
	if !ValidExportStatus(ExportStatusFailed) {
		t.Fatal("failed must be valid")
	}
}

func TestFinancialApprovalStatuses(t *testing.T) {
	if !ValidFinancialApprovalStatus(FinancialApprovalStatusPending) {
		t.Fatal("pending must be valid")
	}
	if !ValidFinancialApprovalStatus(FinancialApprovalStatusApproved) {
		t.Fatal("approved must be valid")
	}
	if !ValidFinancialApprovalStatus(FinancialApprovalStatusRejected) {
		t.Fatal("rejected must be valid")
	}
}

func TestFeeExclusions(t *testing.T) {
	if !IsFeeBaseExcluded(FeExcludedTax) {
		t.Fatal("tax must be excluded from fee base")
	}
	if !IsFeeBaseExcluded(FeExcludedDeposit) {
		t.Fatal("deposits must be excluded from fee base")
	}
	if !IsFeeBaseExcluded(FeExcludedPassThroughCleaning) {
		t.Fatal("pass-through cleaning must be excluded from fee base")
	}
	if IsFeeBaseExcluded("management_fee") {
		t.Fatal("management_fee must not be fee-excluded")
	}
}

func TestHandlerNew(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.svc != svc {
		t.Fatal("handler must wrap the service")
	}
}

func TestRegisterRoutes(t *testing.T) {
	_ = NewHandler(&Service{})
}

func TestMakerCheckerRequestTypes(t *testing.T) {
	types := []string{
		MakerCheckerRequestTypeVendorCreation,
		MakerCheckerRequestTypeBankChange,
		MakerCheckerRequestTypePurchase,
		MakerCheckerRequestTypePayment,
		MakerCheckerRequestTypeJournalPosting,
	}
	for _, rt := range types {
		if !ValidMakerCheckerRequestType(rt) {
			t.Fatalf("%s must be a valid maker-checker request type", rt)
		}
	}
	if ValidMakerCheckerRequestType("bogus") {
		t.Fatal("bogus must not be a valid request type")
	}
}

func TestRequestStatuses(t *testing.T) {
	statuses := []string{
		RequestStatusDraft,
		RequestStatusPendingApproval,
		RequestStatusPendingVerification,
		RequestStatusApproved,
		RequestStatusRejected,
		RequestStatusCancelled,
	}
	for _, s := range statuses {
		if !ValidRequestStatus(s) {
			t.Fatalf("%s must be a valid request status", s)
		}
	}
	if ValidRequestStatus("bogus") {
		t.Fatal("bogus must not be a valid request status")
	}
}

func TestBankVerificationStatuses(t *testing.T) {
	if !ValidBankVerificationStatus(BankVerificationStatusPending) {
		t.Fatal("pending must be valid")
	}
	if !ValidBankVerificationStatus(BankVerificationStatusVerified) {
		t.Fatal("verified must be valid")
	}
	if !ValidBankVerificationStatus(BankVerificationStatusExpired) {
		t.Fatal("expired must be valid")
	}
}

func TestExceptionTypes(t *testing.T) {
	types := []string{
		ExceptionTypeUnmatchedEntry,
		ExceptionTypeAmountMismatch,
		ExceptionTypeCurrencyMismatch,
		ExceptionTypeMissingEntry,
		ExceptionTypeDuplicateEntry,
		ExceptionTypeOther,
	}
	for _, et := range types {
		if !ValidExceptionType(et) {
			t.Fatalf("%s must be a valid exception type", et)
		}
	}
	if ValidExceptionType("bogus") {
		t.Fatal("bogus must not be a valid exception type")
	}
}

func TestExceptionStatuses(t *testing.T) {
	if !ValidExceptionStatus(ExceptionStatusOpen) {
		t.Fatal("open must be valid")
	}
	if !ValidExceptionStatus(ExceptionStatusResolved) {
		t.Fatal("resolved must be valid")
	}
	if ValidExceptionStatus("deleted") {
		t.Fatal("deleted must not be valid")
	}
}

func TestMakerCheckerSentinelErrors(t *testing.T) {
	errs := []error{
		ErrSelfApprovalDenied,
		ErrAICannotPost,
		ErrBankVerificationRequired,
		ErrBankVerificationNotFound,
		ErrBankVerificationExpired,
		ErrMakerCheckerRequestNotFound,
		ErrInvalidMakerCheckerRequest,
		ErrRequestNotPendingApproval,
		ErrRequestNotPendingVerification,
		ErrReconciliationExceptionNotFound,
		ErrInvalidReconciliationException,
	}
	for _, e := range errs {
		if !errors.Is(e, e) {
			t.Fatalf("%v must be self-wrapping", e)
		}
	}
}
