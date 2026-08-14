package security

import (
	"context"
	"errors"
)

var (
	ErrDenied = errors.New("access denied")
)

type Action string

type Resource struct {
	Type string
	ID   string
}

type Subject struct {
	ActorID  string
	TenantID string
	Roles    []string
}

type Authorizer interface {
	Can(ctx context.Context, subject Subject, action Action, resource Resource) error
}

type DenyAllAuthorizer struct{}

func (DenyAllAuthorizer) Can(ctx context.Context, subject Subject, action Action, resource Resource) error {
	return ErrDenied
}

type AllowAllAuthorizer struct{}

func (AllowAllAuthorizer) Can(ctx context.Context, subject Subject, action Action, resource Resource) error {
	return nil
}

type RoleBasedAuthorizer struct {
	policies []RolePolicy
}

type RolePolicy struct {
	Role      string
	Actions   []Action
	Resources []Resource
}

func NewRoleBasedAuthorizer(policies []RolePolicy) *RoleBasedAuthorizer {
	return &RoleBasedAuthorizer{policies: policies}
}

func (a *RoleBasedAuthorizer) Can(ctx context.Context, subject Subject, action Action, resource Resource) error {
	for _, role := range subject.Roles {
		for _, policy := range a.policies {
			if policy.Role != role {
				continue
			}
			if !actionMatches(action, policy.Actions) {
				continue
			}
			if !resourceMatches(resource, policy.Resources) {
				continue
			}
			return nil
		}
	}
	return ErrDenied
}

func actionMatches(action Action, allowed []Action) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == action {
			return true
		}
	}
	return false
}

func resourceMatches(resource Resource, allowed []Resource) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, r := range allowed {
		if r.Type == resource.Type && (r.ID == "" || r.ID == resource.ID) {
			return true
		}
	}
	return false
}
