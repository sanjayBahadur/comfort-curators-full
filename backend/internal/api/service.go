package api

import (
	"context"
	"time"

	"comfort-curators-backend/internal/property"
)

type PropService interface {
	CreateProperty(ctx context.Context, params property.CreatePropertyParams, actorID string) (*property.Property, error)
	GetProperty(ctx context.Context, tenantID, propertyID string) (*property.Property, error)
	ListProperties(ctx context.Context, tenantID string) ([]property.Property, error)
	TransitionProperty(ctx context.Context, tenantID, propertyID, toState, reason, actorID string) (*property.Property, error)
	ListTransitions(ctx context.Context, tenantID, propertyID string) ([]property.PropertyTransition, error)
	SetReadiness(ctx context.Context, tenantID, propertyID string, readiness property.Readiness, actorID string) (*property.Property, error)
	AddComplianceHold(ctx context.Context, tenantID, propertyID string, params property.ComplianceHoldParams, actorID string) (*property.ComplianceHold, error)
	ResolveComplianceHold(ctx context.Context, tenantID, propertyID, holdID, actorID string) (*property.Property, error)
	GrantComplianceException(ctx context.Context, tenantID, propertyID, holdID, reviewerID, reason string, ttl time.Duration, actorID string) (*property.Property, error)
}
