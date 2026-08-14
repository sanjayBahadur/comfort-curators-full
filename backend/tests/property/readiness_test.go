package property_test

import (
	"errors"
	"testing"
	"time"

	"comfort-curators-backend/internal/property"
)

func readyProperty() *property.Property {
	return &property.Property{
		ID:       "prop-1",
		TenantID: "tenant-1",
		State:    property.StateReadyInactive,
		Readiness: property.Readiness{
			OwnerContractAccepted: true,
			ComplianceComplete:    true,
			MandatoryFieldsSet:    true,
		},
		Version: 3,
	}
}

func hold(severity, status string) property.ComplianceHold {
	return property.ComplianceHold{
		ID:         "hold-" + severity + "-" + status,
		PropertyID: "prop-1",
		Kind:       property.HoldKindPermission,
		Severity:   severity,
		Status:     status,
		Reason:     "test hold",
	}
}

func TestPropertyReadyPropertyCanActivate(t *testing.T) {
	now := time.Now().UTC()
	if err := property.CanActivate(readyProperty(), nil, now); err != nil {
		t.Errorf("fully ready property with no holds must activate: %v", err)
	}
}

func TestPropertyNotReadyBlocksActivation(t *testing.T) {
	now := time.Now().UTC()

	noContract := readyProperty()
	noContract.Readiness.OwnerContractAccepted = false
	if err := property.CanActivate(noContract, nil, now); !errors.Is(err, property.ErrNotReady) {
		t.Errorf("missing owner contract must block with ErrNotReady, got %v", err)
	}

	noCompliance := readyProperty()
	noCompliance.Readiness.ComplianceComplete = false
	if err := property.CanActivate(noCompliance, nil, now); !errors.Is(err, property.ErrNotReady) {
		t.Errorf("incomplete compliance must block with ErrNotReady, got %v", err)
	}

	noFields := readyProperty()
	noFields.Readiness.MandatoryFieldsSet = false
	if err := property.CanActivate(noFields, nil, now); !errors.Is(err, property.ErrNotReady) {
		t.Errorf("missing mandatory fields must block with ErrNotReady, got %v", err)
	}
}

func TestPropertyCriticalHoldBlocksActivation(t *testing.T) {
	now := time.Now().UTC()
	holds := []property.ComplianceHold{hold(property.HoldSeverityCritical, property.HoldStatusOpen)}

	if err := property.CanActivate(readyProperty(), holds, now); !errors.Is(err, property.ErrComplianceHold) {
		t.Errorf("open critical hold must block with ErrComplianceHold, got %v", err)
	}
}

func TestPropertyResolvedCriticalHoldDoesNotBlock(t *testing.T) {
	now := time.Now().UTC()
	resolved := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	resolved.ResolvedAt = &now
	// Resolve via the domain method to exercise real state change.
	if err := resolved.Resolve(now); err != nil {
		t.Fatalf("resolve hold: %v", err)
	}
	holds := []property.ComplianceHold{resolved}
	if err := property.CanActivate(readyProperty(), holds, now); err != nil {
		t.Errorf("resolved critical hold must not block activation: %v", err)
	}
}

func TestPropertyNonCriticalHoldDoesNotBlock(t *testing.T) {
	now := time.Now().UTC()
	holds := []property.ComplianceHold{hold(property.HoldSeverityNonCritical, property.HoldStatusOpen)}
	if err := property.CanActivate(readyProperty(), holds, now); err != nil {
		t.Errorf("non-critical hold must not block activation: %v", err)
	}
}

func TestPropertyReviewerExceptionAllowsActivation(t *testing.T) {
	now := time.Now().UTC()
	h := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	if err := h.GrantException("reviewer-1", now.Add(24*time.Hour), now); err != nil {
		t.Fatalf("grant exception: %v", err)
	}
	holds := []property.ComplianceHold{h}

	if err := property.CanActivate(readyProperty(), holds, now); err != nil {
		t.Errorf("reviewer exception must allow activation: %v", err)
	}
	if !h.HasValidException(now) {
		t.Error("exception must be valid at grant time")
	}
	if h.ExceptionBy != "reviewer-1" {
		t.Errorf("exception must be reviewer-attributed, got %q", h.ExceptionBy)
	}
}

func TestPropertyExpiredExceptionBlocksActivation(t *testing.T) {
	now := time.Now().UTC()
	h := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	if err := h.GrantException("reviewer-1", now.Add(10*time.Minute), now); err != nil {
		t.Fatalf("grant exception: %v", err)
	}
	later := now.Add(11 * time.Minute)
	holds := []property.ComplianceHold{h}

	if h.HasValidException(later) {
		t.Error("expired exception must not be valid")
	}
	if err := property.CanActivate(readyProperty(), holds, later); !errors.Is(err, property.ErrComplianceHold) {
		t.Errorf("expired exception must block activation with ErrComplianceHold, got %v", err)
	}
}

func TestPropertyCriticalHoldBlocksUntilResolved(t *testing.T) {
	now := time.Now().UTC()

	// An open critical hold raised for an expired critical permission still
	// blocks activation (PROP-004): the hold is the enforcement vehicle and is
	// only released by resolution or a reviewer exception.
	open := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	if !open.BlocksActivation(now) {
		t.Error("open critical hold must block activation")
	}
	if err := property.CanActivate(readyProperty(), []property.ComplianceHold{open}, now); !errors.Is(err, property.ErrComplianceHold) {
		t.Errorf("open critical hold must block activation with ErrComplianceHold, got %v", err)
	}

	if err := open.Resolve(now); err != nil {
		t.Fatalf("resolve hold: %v", err)
	}
	if open.BlocksActivation(now) {
		t.Error("resolved hold must not block activation")
	}
	if err := property.CanActivate(readyProperty(), []property.ComplianceHold{open}, now); err != nil {
		t.Errorf("resolved hold must allow activation: %v", err)
	}
}

func TestPropertyGrantExceptionRequiresReviewerAndExpiry(t *testing.T) {
	now := time.Now().UTC()

	h := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	if err := h.GrantException("", now.Add(time.Hour), now); !errors.Is(err, property.ErrExceptionDenied) {
		t.Errorf("exception without reviewer must be denied, got %v", err)
	}

	h2 := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	if err := h2.GrantException("reviewer-1", now.Add(-time.Hour), now); err == nil {
		t.Error("exception with past expiry must be denied")
	}

	h3 := hold(property.HoldSeverityCritical, property.HoldStatusOpen)
	if err := h3.Resolve(now); err != nil {
		t.Fatalf("resolve hold: %v", err)
	}
	if err := h3.GrantException("reviewer-1", now.Add(time.Hour), now); !errors.Is(err, property.ErrHoldNotOpen) {
		t.Errorf("exception on resolved hold must be denied, got %v", err)
	}
}

func TestPropertyHoldValidation(t *testing.T) {
	now := time.Now().UTC()
	_, err := property.NewComplianceHold("prop-1", "tenant-1", property.ComplianceHoldParams{
		Kind:     "bogus",
		Severity: property.HoldSeverityCritical,
		Reason:   "x",
	}, now)
	if !errors.Is(err, property.ErrInvalidComplianceHold) {
		t.Errorf("bogus kind must be rejected, got %v", err)
	}

	_, err = property.NewComplianceHold("prop-1", "tenant-1", property.ComplianceHoldParams{
		Kind:     property.HoldKindInsurance,
		Severity: "severe",
		Reason:   "x",
	}, now)
	if !errors.Is(err, property.ErrInvalidComplianceHold) {
		t.Errorf("bogus severity must be rejected, got %v", err)
	}

	_, err = property.NewComplianceHold("prop-1", "tenant-1", property.ComplianceHoldParams{
		Kind:     property.HoldKindInsurance,
		Severity: property.HoldSeverityCritical,
		Reason:   "",
	}, now)
	if !errors.Is(err, property.ErrInvalidComplianceHold) {
		t.Errorf("missing reason must be rejected, got %v", err)
	}
}

func TestPropertyReadinessReadyFlag(t *testing.T) {
	if !readyProperty().Readiness.Ready() {
		t.Error("all mandatory readiness inputs present must report ready")
	}
	partial := readyProperty()
	partial.Readiness.OwnerContractAccepted = false
	if partial.Readiness.Ready() {
		t.Error("missing owner contract must not report ready")
	}
}
