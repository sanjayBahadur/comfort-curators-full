package iam

import (
	"errors"
	"time"
)

var (
	ErrUserNotFound          = errors.New("user not found")
	ErrOTPNotFound           = errors.New("otp not found")
	ErrOTPExpired            = errors.New("otp has expired")
	ErrOTPConsumed           = errors.New("otp has already been consumed")
	ErrOTPInvalid            = errors.New("invalid otp")
	ErrSessionNotFound       = errors.New("session not found")
	ErrSessionExpired        = errors.New("session has expired")
	ErrSessionRevoked        = errors.New("session has been revoked")
	ErrRoleNotAllowed        = errors.New("role not allowed")
	ErrRateLimited           = errors.New("rate limit exceeded")
	ErrTenantNotFound        = errors.New("tenant not found")
	ErrCrossTenantDenied     = errors.New("cross-tenant access denied")
	ErrMembershipNotFound    = errors.New("membership not found")
	ErrSupportAccessExpired  = errors.New("support access grant has expired")
	ErrSupportAccessRevoked  = errors.New("support access grant has been revoked")
	ErrSupportAccessNotFound = errors.New("support access grant not found")
	ErrPropertyNotFound      = errors.New("property not found")
	ErrNotTenantMember       = errors.New("not a member of this tenant")
	ErrMFAInvalid            = errors.New("invalid mfa code")
	ErrMFAUnverified         = errors.New("mfa method not verified")
	ErrMFAEnrollmentNotFound = errors.New("no pending mfa enrollment found")
	ErrMFAAlreadyEnrolled    = errors.New("user already has a verified mfa method")
)

const (
	RoleOwner = "owner"
	RoleGuest = "guest"
	RoleStaff = "staff"
)

func ValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleGuest, RoleStaff, "jarvis", "superhost":
		return true
	}
	return false
}

type User struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Contact   string    `json:"contact"`
	Role      string    `json:"role"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AuthenticationMethod struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Method     string    `json:"method"`
	SecretHash string    `json:"-"`
	ExpiresAt  time.Time `json:"expires_at"`
	Consumed   bool      `json:"consumed"`
	CreatedAt  time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	ActorID   string    `json:"actor_id"`
	Roles     []string  `json:"roles"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type MFAMethod struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Method    string    `json:"method"`
	Secret    string    `json:"-"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

// MFAEnrollmentResult is returned by EnrollMFA. It contains the unverified
// method record plus the plaintext secret and otpauth:// URI needed to
// provision an authenticator application.
type MFAEnrollmentResult struct {
	Method          *MFAMethod
	Secret          string
	ProvisioningURI string
}

type OTP struct {
	Token     string
	ExpiresAt time.Time
}

type CreateUserParams struct {
	TenantID string
	Contact  string
	Role     string
}

type RequestOTPParams struct {
	TenantID string
	Contact  string
	Role     string
}

type VerifyOTPParams struct {
	TenantID string
	Contact  string
	Token    string
}

type VerifyMFAParams struct {
	UserID string
	Code   string
}

type ConfirmMFAParams struct {
	UserID string
	Code   string
}

type CreateSessionParams struct {
	TenantID string
	Contact  string
}

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Membership struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type SupportAccessGrant struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	GrantedByUserID string    `json:"granted_by_user_id"`
	GrantedToUserID string    `json:"granted_to_user_id"`
	Reason          string    `json:"reason"`
	Scope           string    `json:"scope"`
	ExpiresAt       time.Time `json:"expires_at"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateTenantParams struct {
	Name string
}

type CreateSupportAccessGrantParams struct {
	TenantID        string
	GrantedByUserID string
	GrantedToUserID string
	Reason          string
	Scope           string
	TTL             time.Duration
}

type AttributePolicy struct {
	TenantID   *string
	PropertyID *string
	UserID     *string
	Role       *string
	State      *string
	Risk       *int
	Threshold  *int
	TimeWindow *TimeWindowRule
	Assignment *string
}

type TimeWindowRule struct {
	After  *time.Time
	Before *time.Time
}

const (
	SupportAccessScopeTenant   = "tenant"
	SupportAccessScopeProperty = "property"
)
