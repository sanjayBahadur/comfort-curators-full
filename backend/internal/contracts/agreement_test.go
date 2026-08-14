package contracts

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func sampleTerms() []byte {
	return []byte(`{"scope":{"tier":"full_service","units":3},"fee":{"percentage_basis_points":1800,"minimum_monthly_minor_units":60000000},"exclusions":["taxes","refundable_deposits","pass_through_cleaning"]}`)
}

func newDraftAgreement(t *testing.T, terms []byte) (*Agreement, *AgreementVersion) {
	t.Helper()
	a := &Agreement{
		ID:         "agree-1",
		TenantID:   "tenant-1",
		PropertyID: "prop-1",
		Status:     AgreementStatusDraft,
		Version:    1,
	}
	v, err := a.AddVersion(terms, time.Now().UTC())
	if err != nil {
		t.Fatalf("add version: %v", err)
	}
	return a, v
}

func TestAgreementHashIsCanonicalAcrossKeyOrder(t *testing.T) {
	termsA := []byte(`{"fee":{"percentage_basis_points":1800},"scope":{"tier":"full_service"}}`)
	termsB := []byte(`{"scope":{"tier":"full_service"},"fee":{"percentage_basis_points":1800}}`)

	hashA, err := HashTerms(termsA)
	if err != nil {
		t.Fatalf("hash A: %v", err)
	}
	hashB, err := HashTerms(termsB)
	if err != nil {
		t.Fatalf("hash B: %v", err)
	}
	if hashA != hashB {
		t.Errorf("canonical hash must be stable across JSON key order: %s vs %s", hashA, hashB)
	}
	if !json.Valid(termsA) || !json.Valid(termsB) {
		t.Fatal("test terms must be valid JSON")
	}
}

func TestAgreementVersionIsImmutableOnceWritten(t *testing.T) {
	a, v1 := newDraftAgreement(t, sampleTerms())

	// A correction creates a new version instead of mutating v1.
	corrected := []byte(`{"scope":{"tier":"full_service","units":4},"fee":{"percentage_basis_points":1800,"minimum_monthly_minor_units":60000000}}`)
	v2, err := a.AddVersion(corrected, time.Now().UTC())
	if err != nil {
		t.Fatalf("add corrected version: %v", err)
	}
	if v1.VersionNumber != 1 || v2.VersionNumber != 2 {
		t.Errorf("versions must be sequential: %d then %d", v1.VersionNumber, v2.VersionNumber)
	}
	if a.CurrentVersion != 2 || len(a.Versions) != 2 {
		t.Errorf("agreement must keep both versions, current=%d count=%d", a.CurrentVersion, len(a.Versions))
	}
	if v1.ContentHash == v2.ContentHash {
		t.Error("different terms must produce different content hashes")
	}
	// The original version record must be byte-identical: no in-place mutation.
	if string(v1.Terms) != string(sampleTerms()) {
		t.Error("original version terms must be unchanged")
	}
}

func TestAgreementAcceptedCannotMutate(t *testing.T) {
	a, _ := newDraftAgreement(t, sampleTerms())
	acc, err := a.Accept("owner-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if a.Status != AgreementStatusAccepted {
		t.Fatalf("agreement must be accepted, got %q", a.Status)
	}

	// The accepted agreement cannot mutate: adding a version is refused.
	if _, err := a.AddVersion([]byte(`{"scope":{}}`), time.Now().UTC()); !errors.Is(err, ErrAcceptedImmutable) {
		t.Errorf("adding a version to an accepted agreement must be refused, got %v", err)
	}
	// Acceptance is single-shot too.
	if _, err := a.Accept("owner-2", time.Now().UTC()); !errors.Is(err, ErrAlreadyAccepted) {
		t.Errorf("double acceptance must be refused, got %v", err)
	}
	_ = acc
}

func TestAgreementAcceptancePointsToExactContentHash(t *testing.T) {
	a, v1 := newDraftAgreement(t, sampleTerms())
	acceptance, err := a.Accept("owner-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	if acceptance.AgreementID != a.ID {
		t.Errorf("acceptance must reference the agreement, got %s", acceptance.AgreementID)
	}
	if acceptance.VersionNumber != v1.VersionNumber {
		t.Errorf("acceptance must point to the accepted version %d, got %d", v1.VersionNumber, acceptance.VersionNumber)
	}
	if acceptance.ContentHash != v1.ContentHash {
		t.Errorf("acceptance must point to the exact content hash, got %s want %s", acceptance.ContentHash, v1.ContentHash)
	}
	recomputed, _ := HashTerms(v1.Terms)
	if acceptance.ContentHash != recomputed {
		t.Errorf("acceptance content hash must match the recomputed terms hash")
	}
	if acceptance.AcceptedBy != "owner-1" {
		t.Errorf("acceptance must record who accepted, got %q", acceptance.AcceptedBy)
	}
}

func TestAgreementAcceptRefusesHashMismatch(t *testing.T) {
	a, _ := newDraftAgreement(t, sampleTerms())
	// Tamper with the stored hash to simulate drift.
	a.Versions[0].ContentHash = "sha256:" + "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f"
	if _, err := a.Accept("owner-1", time.Now().UTC()); !errors.Is(err, ErrInvalidAgreement) {
		t.Errorf("accepting with a mismatched content hash must be refused, got %v", err)
	}
}

func TestAgreementVerifyContentHashDetectsDrift(t *testing.T) {
	a, _ := newDraftAgreement(t, sampleTerms())
	if err := a.VerifyContentHash(); err != nil {
		t.Fatalf("clean agreement must verify: %v", err)
	}
	a.Versions[0].ContentHash = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := a.VerifyContentHash(); !errors.Is(err, ErrInvalidAgreement) {
		t.Errorf("drifted version must fail verification, got %v", err)
	}
}

func TestAgreementCannotAcceptWithoutVersion(t *testing.T) {
	a := &Agreement{
		ID:       "agree-empty",
		TenantID: "tenant-1",
		Status:   AgreementStatusDraft,
		Version:  1,
	}
	if _, err := a.Accept("owner-1", time.Now().UTC()); !errors.Is(err, ErrAgreementVersionNotFound) {
		t.Errorf("accepting an agreement without versions must be refused, got %v", err)
	}
}

func TestAgreementRejectsEmptyTerms(t *testing.T) {
	a := &Agreement{ID: "agree-1", TenantID: "tenant-1", Status: AgreementStatusDraft, Version: 1}
	if _, err := a.AddVersion(nil, time.Now().UTC()); !errors.Is(err, ErrEmptyTerms) {
		t.Errorf("empty terms must be rejected, got %v", err)
	}
	if _, err := a.AddVersion([]byte(`{}`), time.Now().UTC()); err != nil {
		t.Errorf("valid empty object terms must be accepted, got %v", err)
	}
}

func TestAgreementAddVersionChecksAcceptedStatusBeforeTerms(t *testing.T) {
	a, _ := newDraftAgreement(t, sampleTerms())
	if _, err := a.Accept("owner-1", time.Now().UTC()); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := a.AddVersion(nil, time.Now().UTC()); !errors.Is(err, ErrAcceptedImmutable) {
		t.Errorf("accepted agreement must refuse even empty terms, got %v", err)
	}
}

func TestAgreementCurrentVersionTerms(t *testing.T) {
	a, v1 := newDraftAgreement(t, sampleTerms())
	terms, err := a.CurrentVersionTerms()
	if err != nil {
		t.Fatalf("current version terms: %v", err)
	}
	if string(terms) != string(v1.Terms) {
		t.Error("current version terms must match the last version")
	}

	empty := &Agreement{ID: "agree-empty", Status: AgreementStatusDraft}
	if _, err := empty.CurrentVersionTerms(); !errors.Is(err, ErrAgreementVersionNotFound) {
		t.Errorf("empty agreement must report no version, got %v", err)
	}
}
