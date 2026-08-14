package iam

import (
	"context"
	"fmt"

	"comfort-curators-backend/internal/platform/security"
)

type TenantScopedAuthorizer struct {
	inner   security.Authorizer
	tenancy *TenancyService
}

func NewTenantScopedAuthorizer(inner security.Authorizer, tenancy *TenancyService) *TenantScopedAuthorizer {
	return &TenantScopedAuthorizer{
		inner:   inner,
		tenancy: tenancy,
	}
}

func (a *TenantScopedAuthorizer) Can(ctx context.Context, subject security.Subject, action security.Action, resource security.Resource) error {
	if a.inner != nil {
		if err := a.inner.Can(ctx, subject, action, resource); err != nil {
			return err
		}
	}
	return nil
}

func (a *TenantScopedAuthorizer) CanAccessTenant(ctx context.Context, subject security.Subject, resourceTenantID string) error {
	if subject.TenantID == "" {
		return ErrCrossTenantDenied
	}

	if subject.TenantID == resourceTenantID {
		return nil
	}

	if a.tenancy != nil {
		if err := a.tenancy.ValidateSupportAccess(ctx, subject.ActorID, resourceTenantID); err == nil {
			return nil
		}
	}

	return ErrCrossTenantDenied
}

func (a *TenantScopedAuthorizer) CanAccessProperty(ctx context.Context, subject security.Subject, resourceTenantID, propertyID string) error {
	if err := a.CanAccessTenant(ctx, subject, resourceTenantID); err != nil {
		return err
	}

	_ = propertyID
	return nil
}

type AttributeAuthorizer struct {
	tenancy  *TenancyService
	policies []AttributePolicyRule
}

type AttributePolicyRule struct {
	ResourceType string
	Actions      []security.Action
	Attributes   AttributePolicy
}

func NewAttributeAuthorizer(tenancy *TenancyService, rules []AttributePolicyRule) *AttributeAuthorizer {
	return &AttributeAuthorizer{
		tenancy:  tenancy,
		policies: rules,
	}
}

func (a *AttributeAuthorizer) Can(ctx context.Context, subject security.Subject, action security.Action, resource security.Resource) error {
	if subject.TenantID == "" {
		return ErrCrossTenantDenied
	}

	matched := false
	for _, rule := range a.policies {
		if rule.ResourceType != resource.Type {
			continue
		}

		actionMatch := len(rule.Actions) == 0
		for _, allowed := range rule.Actions {
			if allowed == action {
				actionMatch = true
				break
			}
		}
		if !actionMatch {
			continue
		}

		if err := ValidateAttributePolicy(rule.Attributes, subject); err != nil {
			continue
		}

		matched = true
		break
	}

	if !matched {
		return ErrCrossTenantDenied
	}

	return nil
}

func (a *AttributeAuthorizer) RequireTenantScope(ctx context.Context, subject security.Subject, resourceTenantID string) error {
	if subject.TenantID == "" {
		return ErrCrossTenantDenied
	}

	if subject.TenantID == resourceTenantID {
		return nil
	}

	if a.tenancy != nil {
		if err := a.tenancy.ValidateSupportAccess(ctx, subject.ActorID, resourceTenantID); err == nil {
			return nil
		}
	}

	return ErrCrossTenantDenied
}

func DenyBeforeDisclose(err error) error {
	switch err {
	case ErrCrossTenantDenied, ErrSupportAccessExpired, ErrSupportAccessRevoked, ErrSupportAccessNotFound:
		return ErrCrossTenantDenied
	case ErrTenantNotFound:
		return ErrTenantNotFound
	case ErrPropertyNotFound:
		return ErrPropertyNotFound
	case ErrNotTenantMember:
		return ErrCrossTenantDenied
	}
	return err
}

func AuthorizeTenantAccess(ctx context.Context, tenancy *TenancyService, resourceTenantID string, resourceType string, resourceID string) error {
	subject, ok := SubjectFromContext(ctx)
	if !ok {
		return fmt.Errorf("%w: no subject in context", ErrCrossTenantDenied)
	}

	_ = subject
	return tenancy.RequireResourceAccess(ctx, resourceTenantID, resourceType, resourceID)
}
