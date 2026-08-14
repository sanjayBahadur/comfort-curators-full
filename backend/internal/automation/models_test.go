package automation_test

import (
	"testing"

	"comfort-curators-backend/internal/automation"
)

func TestValidateTransition(t *testing.T) {
	valid := []struct{ from, to string }{
		{automation.StateQueued, automation.StateLeased},
		{automation.StateQueued, automation.StateCancelled},
		{automation.StateLeased, automation.StateRunning},
		{automation.StateLeased, automation.StateQueued},
		{automation.StateLeased, automation.StateCancelled},
		{automation.StateLeased, automation.StateFailed},
		{automation.StateRunning, automation.StateWaitingForTool},
		{automation.StateRunning, automation.StateWaitingForApproval},
		{automation.StateRunning, automation.StateUnknown},
		{automation.StateRunning, automation.StateRetryable},
		{automation.StateRunning, automation.StateCompleted},
		{automation.StateRunning, automation.StateCancelled},
		{automation.StateRunning, automation.StateFailed},
		{automation.StateWaitingForTool, automation.StateRunning},
		{automation.StateWaitingForTool, automation.StateWaitingForApproval},
		{automation.StateWaitingForTool, automation.StateRetryable},
		{automation.StateWaitingForTool, automation.StateCancelled},
		{automation.StateWaitingForTool, automation.StateFailed},
		{automation.StateWaitingForApproval, automation.StateRunning},
		{automation.StateWaitingForApproval, automation.StateCompleted},
		{automation.StateWaitingForApproval, automation.StateCancelled},
		{automation.StateWaitingForApproval, automation.StateFailed},
		{automation.StateWaitingForApproval, automation.StateQueued},
		{automation.StateRetryable, automation.StateQueued},
		{automation.StateRetryable, automation.StateCancelled},
		{automation.StateRetryable, automation.StateFailed},
		{automation.StateUnknown, automation.StateRetryable},
		{automation.StateUnknown, automation.StateQueued},
		{automation.StateUnknown, automation.StateCancelled},
		{automation.StateUnknown, automation.StateFailed},
		{automation.StateFailed, automation.StateQueued},
	}

	for _, tc := range valid {
		err := automation.ValidateTransition(tc.from, tc.to)
		if err != nil {
			t.Errorf("expected %s -> %s to be valid, got error: %v", tc.from, tc.to, err)
		}
	}

	invalid := []struct{ from, to string }{
		{automation.StateQueued, automation.StateCompleted},
		{automation.StateCompleted, automation.StateQueued},
		{automation.StateCancelled, automation.StateQueued},
		{automation.StateQueued, automation.StateRunning},
		{automation.StateLeased, automation.StateCompleted},
		{automation.StateRunning, automation.StateQueued},
		{automation.StateFailed, automation.StateCompleted},
	}

	for _, tc := range invalid {
		err := automation.ValidateTransition(tc.from, tc.to)
		if err == nil {
			t.Errorf("expected %s -> %s to be invalid, got nil", tc.from, tc.to)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if !automation.IsTerminal(automation.StateCompleted) {
		t.Error("expected Completed to be terminal")
	}
	if !automation.IsTerminal(automation.StateCancelled) {
		t.Error("expected Cancelled to be terminal")
	}
	if automation.IsTerminal(automation.StateFailed) {
		t.Error("expected Failed not to be terminal")
	}
	if automation.IsTerminal(automation.StateQueued) {
		t.Error("expected Queued not to be terminal")
	}
	if automation.IsTerminal(automation.StateRunning) {
		t.Error("expected Running not to be terminal")
	}
}

func TestIsCancellable(t *testing.T) {
	if !automation.IsCancellable(automation.StateQueued) {
		t.Error("expected Queued to be cancellable")
	}
	if !automation.IsCancellable(automation.StateRunning) {
		t.Error("expected Running to be cancellable")
	}
	if automation.IsCancellable(automation.StateCompleted) {
		t.Error("expected Completed not to be cancellable")
	}
	if automation.IsCancellable(automation.StateFailed) {
		t.Error("expected Failed not to be cancellable")
	}
	if automation.IsCancellable(automation.StateCancelled) {
		t.Error("expected Cancelled not to be cancellable")
	}
}

func TestFailedRunIsRetryableTransition(t *testing.T) {
	if err := automation.ValidateTransition(automation.StateFailed, automation.StateQueued); err != nil {
		t.Errorf("failed -> queued must be valid for manual retry during outage: %v", err)
	}

	if err := automation.ValidateTransition(automation.StateRetryable, automation.StateQueued); err != nil {
		t.Errorf("retryable -> queued must be valid for retry: %v", err)
	}

	if err := automation.ValidateTransition(automation.StateQueued, automation.StateFailed); err == nil {
		t.Error("queued -> failed must not be valid direct transition")
	}
}
