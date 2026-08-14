package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// HashTerms returns the canonical content hash of the agreement terms. The
// terms are decoded and re-encoded so the hash is stable regardless of JSON
// key order: identical terms always produce an identical hash.
func HashTerms(terms []byte) (string, error) {
	var v any
	if err := json.Unmarshal(terms, &v); err != nil {
		return "", fmt.Errorf("parse agreement terms: %w", err)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("canonicalize agreement terms: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// AddVersion appends a new immutable version of the terms to a draft
// agreement. The content hash is computed from the canonical terms and the
// version record cannot change afterwards. An accepted agreement cannot
// mutate: adding a version is refused and a correction requires a new
// agreement.
func (a *Agreement) AddVersion(terms []byte, now time.Time) (*AgreementVersion, error) {
	if a.Status == AgreementStatusAccepted {
		return nil, ErrAcceptedImmutable
	}
	if len(terms) == 0 {
		return nil, ErrEmptyTerms
	}
	contentHash, err := HashTerms(terms)
	if err != nil {
		return nil, err
	}

	a.CurrentVersion++
	v := AgreementVersion{
		AgreementID:   a.ID,
		TenantID:      a.TenantID,
		VersionNumber: a.CurrentVersion,
		ContentHash:   contentHash,
		Terms:         terms,
		CreatedAt:     now,
	}
	a.Versions = append(a.Versions, v)
	a.Version++
	a.UpdatedAt = now
	return &v, nil
}

// CurrentVersionTerms returns the terms of the most recent version, or
// ErrAgreementVersionNotFound when the agreement has no version yet.
func (a *Agreement) CurrentVersionTerms() ([]byte, error) {
	if len(a.Versions) == 0 {
		return nil, ErrAgreementVersionNotFound
	}
	return a.Versions[len(a.Versions)-1].Terms, nil
}

// Accept accepts the current version of the agreement. Acceptance is terminal:
// the agreement status becomes accepted and no further version may be added.
// The returned acceptance points to the exact content hash of the accepted
// version so the accepted terms are fixed forever. The stored content hash is
// recomputed from the terms and any mismatch is refused.
func (a *Agreement) Accept(actorID string, now time.Time) (*ContractAcceptance, error) {
	if a.Status == AgreementStatusAccepted {
		return nil, ErrAlreadyAccepted
	}
	if len(a.Versions) == 0 {
		return nil, ErrAgreementVersionNotFound
	}
	last := a.Versions[len(a.Versions)-1]
	recomputed, err := HashTerms(last.Terms)
	if err != nil {
		return nil, err
	}
	if recomputed != last.ContentHash {
		return nil, fmt.Errorf("%w: version %d content hash mismatch", ErrInvalidAgreement, last.VersionNumber)
	}

	acceptance := &ContractAcceptance{
		AgreementID:   a.ID,
		TenantID:      a.TenantID,
		VersionNumber: last.VersionNumber,
		ContentHash:   last.ContentHash,
		AcceptedBy:    actorID,
		AcceptedAt:    now,
	}
	a.Acceptance = acceptance
	a.Status = AgreementStatusAccepted
	a.Version++
	a.UpdatedAt = now
	return acceptance, nil
}

// VerifyContentHash reports whether the version's stored content hash matches
// a recomputation from its terms. It is the read-side immutability check.
func (a *Agreement) VerifyContentHash() error {
	for _, v := range a.Versions {
		recomputed, err := HashTerms(v.Terms)
		if err != nil {
			return err
		}
		if recomputed != v.ContentHash {
			return fmt.Errorf("%w: version %d content hash mismatch", ErrInvalidAgreement, v.VersionNumber)
		}
	}
	return nil
}
