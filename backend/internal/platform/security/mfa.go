package security

import (
	"context"
	"errors"
)

var (
	ErrMFARequired = errors.New("MFA verification required for privileged action")
)

type MFAState string

const (
	MFAStateDisabled MFAState = "disabled"
	MFAStateEnabled  MFAState = "enabled"
	MFAStateRequired MFAState = "required"
)

type MFAPolicy struct {
	Level   string
	Require bool
}

type MFAVerifier interface {
	RequireMFA(ctx context.Context, subject Subject, action Action) error
}

type policyMFAVerifier struct {
	enforceLevel string
	mfaChecker   func(ctx context.Context, subject Subject) (MFAState, error)
}

func NewMFAVerifier(enforceLevel string, checker func(ctx context.Context, subject Subject) (MFAState, error)) MFAVerifier {
	if enforceLevel == "" {
		enforceLevel = "privileged"
	}
	return &policyMFAVerifier{
		enforceLevel: enforceLevel,
		mfaChecker:   checker,
	}
}

func (v *policyMFAVerifier) RequireMFA(ctx context.Context, subject Subject, action Action) error {
	isPrivileged := isPrivilegedAction(string(action))
	if !isPrivileged && v.enforceLevel != "all" {
		return nil
	}

	state, err := v.mfaChecker(ctx, subject)
	if err != nil {
		return err
	}

	if state == MFAStateDisabled {
		return nil
	}

	if state == MFAStateRequired || (state == MFAStateEnabled && isPrivileged) {
		return ErrMFARequired
	}

	return nil
}

func isPrivilegedAction(action string) bool {
	privilegedPrefixes := []string{
		"admin.", "privileged.", "system.",
		"user.delete", "user.role.",
		"tenant.delete", "property.delete",
		"financial.approve", "financial.finalize",
		"audit.access", "key.rotate",
	}
	for _, prefix := range privilegedPrefixes {
		if len(action) >= len(prefix) && action[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

type NoOpMFAVerifier struct {
	state MFAState
}

func NewNoOpMFAVerifier(state MFAState) *NoOpMFAVerifier {
	return &NoOpMFAVerifier{state: state}
}

func (v *NoOpMFAVerifier) RequireMFA(ctx context.Context, subject Subject, action Action) error {
	if !isPrivilegedAction(string(action)) {
		return nil
	}
	if v.state == MFAStateRequired || v.state == MFAStateEnabled {
		return ErrMFARequired
	}
	return nil
}
