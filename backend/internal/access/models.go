package access

import (
	"errors"
	"time"
)

var (
	ErrSecretNotFound           = errors.New("access secret not found")
	ErrGrantNotFound            = errors.New("access grant not found")
	ErrGrantNotActive           = errors.New("access grant is not active")
	ErrGrantExpired             = errors.New("access grant has expired")
	ErrGrantNotYetActive        = errors.New("access grant is not yet active")
	ErrGrantAlreadyRevoked      = errors.New("access grant already revoked")
	ErrGrantAlreadyReturned     = errors.New("access grant already returned")
	ErrGrantAlreadyAcknowledged = errors.New("access grant already acknowledged")
	ErrGrantNotAcknowledged     = errors.New("access grant not yet acknowledged")
	ErrGrantWindowMismatch      = errors.New("disclosure is outside the grant service window")
	ErrHoldNotFound             = errors.New("access hold not found")
	ErrHoldAlreadyReleased      = errors.New("access hold already released")
	ErrAccessHeld               = errors.New("property access is held")
	ErrCrossTenantDenied        = errors.New("cross-tenant access denied")
	ErrPropertyNotFound         = errors.New("property not found")
	ErrInvalidSecret            = errors.New("invalid access secret")
	ErrInvalidGrant             = errors.New("invalid access grant")
	ErrInvalidWindow            = errors.New("invalid service window")
	ErrInvalidDisclosure        = errors.New("invalid disclosure")
	ErrInvalidEmergency         = errors.New("invalid emergency access")
	ErrDuplicateGrant           = errors.New("duplicate access grant")
	ErrUnauthorized             = errors.New("unauthorized access action")
)

const (
	SecretTypeKeyCode      = "key_code"
	SecretTypeLockboxCode  = "lockbox_code"
	SecretTypeSmartLockPIN = "smart_lock_pin"
	SecretTypeGateCode     = "gate_code"
	SecretTypeAlarmCode    = "alarm_code"
	SecretTypeOther        = "other"
)

var validSecretTypes = map[string]bool{
	SecretTypeKeyCode:      true,
	SecretTypeLockboxCode:  true,
	SecretTypeSmartLockPIN: true,
	SecretTypeGateCode:     true,
	SecretTypeAlarmCode:    true,
	SecretTypeOther:        true,
}

func ValidSecretType(t string) bool {
	return validSecretTypes[t]
}

const (
	GrantStatusActive       = "active"
	GrantStatusAcknowledged = "acknowledged"
	GrantStatusReturned     = "returned"
	GrantStatusRevoked      = "revoked"
	GrantStatusExpired      = "expired"
)

const (
	CustodyEventTypeIssued          = "issued"
	CustodyEventTypeDisclosed       = "disclosed"
	CustodyEventTypeAcknowledged    = "acknowledged"
	CustodyEventTypeReturned        = "returned"
	CustodyEventTypeRevoked         = "revoked"
	CustodyEventTypeEmergencyAccess = "emergency_access"
	CustodyEventTypeHoldPlaced      = "hold_placed"
	CustodyEventTypeHoldReleased    = "hold_released"
	CustodyEventTypeAnomaly         = "anomaly"
)

const (
	HoldStatusActive   = "active"
	HoldStatusReleased = "released"
)

const (
	DisclosureResultSuccess     = "success"
	DisclosureResultDenied      = "denied"
	DisclosureResultOutOfWindow = "out_of_window"
	DisclosureResultRevoked     = "revoked"
	DisclosureResultHeld        = "held"
)

type PropertyAccessSecret struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	PropertyID      string    `json:"property_id"`
	SecretType      string    `json:"secret_type"`
	Label           string    `json:"label"`
	EncryptedValue  string    `json:"encrypted_value"`
	EncryptionKeyID string    `json:"encryption_key_id"`
	Metadata        string    `json:"metadata"`
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AccessGrant struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	PropertyID      string     `json:"property_id"`
	SecretID        string     `json:"secret_id"`
	GranteeID       string     `json:"grantee_id"`
	GranterID       string     `json:"granter_id"`
	WindowStart     time.Time  `json:"window_start"`
	WindowEnd       time.Time  `json:"window_end"`
	Reason          string     `json:"reason"`
	Status          string     `json:"status"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
	ReturnedAt      *time.Time `json:"returned_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RevokedBy       string     `json:"revoked_by,omitempty"`
	RevokeReason    string     `json:"revoke_reason,omitempty"`
	IsEmergency     bool       `json:"is_emergency"`
	EmergencyReason string     `json:"emergency_reason,omitempty"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (g *AccessGrant) IsWithinWindow(now time.Time) bool {
	return !now.Before(g.WindowStart) && now.Before(g.WindowEnd)
}

func (g *AccessGrant) IsActive() bool {
	return g.Status == GrantStatusActive || g.Status == GrantStatusAcknowledged
}

type AccessDisclosure struct {
	ID           string    `json:"id"`
	GrantID      string    `json:"grant_id"`
	TenantID     string    `json:"tenant_id"`
	PropertyID   string    `json:"property_id"`
	SecretID     string    `json:"secret_id"`
	RequestorID  string    `json:"requestor_id"`
	Result       string    `json:"result"`
	DenialReason string    `json:"denial_reason,omitempty"`
	DisclosedAt  time.Time `json:"disclosed_at"`
}

type AccessCustodyEvent struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	PropertyID string    `json:"property_id"`
	GrantID    string    `json:"grant_id,omitempty"`
	SecretID   string    `json:"secret_id,omitempty"`
	EventType  string    `json:"event_type"`
	ActorID    string    `json:"actor_id"`
	GranteeID  string    `json:"grantee_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Metadata   string    `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type AccessHold struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	PropertyID string     `json:"property_id"`
	Reason     string     `json:"reason"`
	PlacedBy   string     `json:"placed_by"`
	Status     string     `json:"status"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ReleasedBy string     `json:"released_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreateSecretParams struct {
	SecretType      string `json:"secret_type"`
	Label           string `json:"label"`
	EncryptedValue  string `json:"encrypted_value"`
	EncryptionKeyID string `json:"encryption_key_id"`
	Metadata        string `json:"metadata"`
}

type CreateGrantParams struct {
	SecretID    string    `json:"secret_id"`
	GranteeID   string    `json:"grantee_id"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Reason      string    `json:"reason"`
}

type EmergencyAccessParams struct {
	Reason      string    `json:"reason"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

type CreateHoldParams struct {
	Reason string `json:"reason"`
}
