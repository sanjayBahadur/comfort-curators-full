package iam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/security"
)

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

const mfaSatisfactionTTL = 5 * time.Minute

type IdentityService struct {
	pool         *pgxpool.Pool
	sessions     *SessionStore
	auditStore   *audit.AuditStore
	mfaVerifier  security.MFAVerifier
	rateLimiter  *RateLimiter
	tenancy      *TenancyService
	mfaMu        sync.Mutex
	mfaSatisfied map[string]time.Time
}

func NewIdentityService(pool *pgxpool.Pool, auditStore *audit.AuditStore) *IdentityService {
	s := &IdentityService{
		pool:         pool,
		sessions:     NewSessionStore(pool),
		auditStore:   auditStore,
		rateLimiter:  NewRateLimiter(5, time.Minute),
		tenancy:      NewTenancyService(pool, auditStore),
		mfaSatisfied: make(map[string]time.Time),
	}

	mfaChecker := func(ctx context.Context, subject security.Subject) (security.MFAState, error) {
		return s.actorMFAState(ctx, subject.ActorID), nil
	}
	s.mfaVerifier = security.NewMFAVerifier("privileged", mfaChecker)

	return s
}

func (s *IdentityService) GetSessionStore() *SessionStore {
	return s.sessions
}

func (s *IdentityService) GetMFAVerifier() security.MFAVerifier {
	return s.mfaVerifier
}

func (s *IdentityService) GetTenancyService() *TenancyService {
	return s.tenancy
}

// actorMFAState returns the MFA state for an actor. A recently successful
// challenge short-circuits to disabled so that "check recent MFA" does not
// demand a fresh code on every request. Otherwise, verified enrollment makes
// MFA enabled, and staff without any verified method have MFA required.
func (s *IdentityService) actorMFAState(ctx context.Context, actorID string) security.MFAState {
	if s.recentlySatisfied(actorID) {
		return security.MFAStateDisabled
	}

	hasMFA, err := s.userHasVerifiedMFA(ctx, actorID)
	if err != nil {
		return security.MFAStateDisabled
	}
	if hasMFA {
		return security.MFAStateEnabled
	}

	user, err := s.getUserByID(ctx, actorID)
	if err != nil {
		return security.MFAStateDisabled
	}
	if user.Role == RoleStaff {
		return security.MFAStateRequired
	}
	return security.MFAStateDisabled
}

func (s *IdentityService) markMFASatisfied(actorID string) {
	s.mfaMu.Lock()
	defer s.mfaMu.Unlock()
	s.mfaSatisfied[actorID] = time.Now().UTC()
}

func (s *IdentityService) recentlySatisfied(actorID string) bool {
	s.mfaMu.Lock()
	defer s.mfaMu.Unlock()

	cutoff := time.Now().UTC().Add(-mfaSatisfactionTTL)
	for id, t := range s.mfaSatisfied {
		if t.Before(cutoff) {
			delete(s.mfaSatisfied, id)
		}
	}

	t, ok := s.mfaSatisfied[actorID]
	return ok && t.After(cutoff)
}

func (s *IdentityService) tenantHasActiveOwner(ctx context.Context, tenantID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE tenant_id = $1 AND role = $2 AND state = 'active')`,
		tenantID, RoleOwner).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check active owner: %w", err)
	}
	return exists, nil
}

func (s *IdentityService) EnsureUser(ctx context.Context, params CreateUserParams) (*User, error) {
	if !ValidRole(params.Role) {
		return nil, ErrRoleNotAllowed
	}

	existing, err := s.getUserByTenantAndContact(ctx, params.TenantID, params.Contact)
	if err != nil && err != ErrUserNotFound {
		return nil, err
	}
	if existing != nil && existing.Role == params.Role {
		return existing, nil
	}

	if params.Role == RoleGuest {
		var count int
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND role = $2 AND state = 'active'`,
			params.TenantID, RoleOwner).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("check owner count: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("guest cannot be created before an owner exists in tenant")
		}
	}

	var user User
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (tenant_id, contact, role, state, created_at, updated_at)
			VALUES ($1, $2, $3, 'active', NOW(), NOW())
			RETURNING id, tenant_id, contact, role, state, created_at, updated_at
		`, params.TenantID, params.Contact, params.Role).Scan(
			&user.ID, &user.TenantID, &user.Contact, &user.Role, &user.State,
			&user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeAuth,
			TenantID:     user.TenantID,
			ActorID:      user.ID,
			Action:       "user.create",
			ResourceType: "user",
			ResourceID:   user.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *IdentityService) RequestOTP(ctx context.Context, params RequestOTPParams) (*OTP, error) {
	rateKey := fmt.Sprintf("otp:%s:%s", params.TenantID, params.Contact)
	if !s.rateLimiter.Allow(rateKey) {
		return nil, ErrRateLimited
	}

	// OTP authentication must not be the mechanism that grants a role. The
	// caller-supplied role is intentionally ignored; existing users are
	// authenticated as-is, and brand-new identities receive the safe default
	// role for their tenant (owner only when no active owner exists yet).
	user, err := s.getUserByTenantAndContact(ctx, params.TenantID, params.Contact)
	if err != nil {
		if err != ErrUserNotFound {
			return nil, err
		}

		role := RoleGuest
		hasOwner, ownerErr := s.tenantHasActiveOwner(ctx, params.TenantID)
		if ownerErr != nil {
			return nil, ownerErr
		}
		if !hasOwner {
			role = RoleOwner
		}

		user, err = s.EnsureUser(ctx, CreateUserParams{
			TenantID: params.TenantID,
			Contact:  params.Contact,
			Role:     role,
		})
		if err != nil {
			return nil, err
		}
	}

	if user.State != "active" {
		return nil, fmt.Errorf("user is not active")
	}

	s.cleanupExpiredOTPs(ctx, user.ID)

	otp, err := GenerateOTP(DefaultOTPLength)
	if err != nil {
		return nil, err
	}

	hash := HashOTP(otp.Token)
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO authentication_methods (user_id, method, secret_hash, expires_at, consumed, created_at)
			VALUES ($1, 'otp', $2, $3, false, NOW())
		`, user.ID, hash, otp.ExpiresAt)
		if err != nil {
			return fmt.Errorf("insert otp: %w", err)
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeAuth,
			TenantID:     params.TenantID,
			ActorID:      user.ID,
			Action:       "otp.request",
			ResourceType: "authentication_method",
		})
	})
	if err != nil {
		return nil, err
	}

	return otp, nil
}

func (s *IdentityService) VerifyOTP(ctx context.Context, params VerifyOTPParams) (*Session, error) {
	user, err := s.getUserByTenantAndContact(ctx, params.TenantID, params.Contact)
	if err != nil {
		return nil, err
	}

	if user.State != "active" {
		return nil, fmt.Errorf("user is not active")
	}

	var authMethod AuthenticationMethod
	err = s.pool.QueryRow(ctx, `
		SELECT id, user_id, method, secret_hash, expires_at, consumed, created_at
		FROM authentication_methods
		WHERE user_id = $1 AND method = 'otp' AND consumed = false
		ORDER BY created_at DESC
		LIMIT 1
	`, user.ID).Scan(&authMethod.ID, &authMethod.UserID, &authMethod.Method,
		&authMethod.SecretHash, &authMethod.ExpiresAt, &authMethod.Consumed, &authMethod.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrOTPNotFound
		}
		return nil, fmt.Errorf("lookup otp: %w", err)
	}

	if authMethod.Consumed {
		return nil, ErrOTPConsumed
	}

	if time.Now().UTC().After(authMethod.ExpiresAt) {
		return nil, ErrOTPExpired
	}

	if !VerifyOTPHash(params.Token, authMethod.SecretHash) {
		return nil, ErrOTPInvalid
	}

	roles := []string{user.Role}
	var session *Session
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE authentication_methods SET consumed = true WHERE id = $1
		`, authMethod.ID)
		if err != nil {
			return fmt.Errorf("consume otp: %w", err)
		}

		session, err = s.sessions.CreateTx(ctx, tx, user.ID, user.TenantID, roles)
		if err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeAuth,
			TenantID:     params.TenantID,
			ActorID:      user.ID,
			Action:       "otp.verify",
			ResourceType: "session",
			ResourceID:   session.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (s *IdentityService) GetSession(ctx context.Context, token string) (*Session, error) {
	session, err := s.sessions.Get(ctx, token)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *IdentityService) RevokeSession(ctx context.Context, token, reason, revokedBy string) error {
	session, _ := s.sessions.Get(ctx, token)
	tenantID := ""
	if session != nil {
		tenantID = session.TenantID
	}

	return database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.sessions.RevokeTx(ctx, tx, token, reason, revokedBy); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeSecurity,
			TenantID:     tenantID,
			ActorID:      revokedBy,
			Action:       "session.revoke",
			ResourceType: "session",
			ResourceID:   token,
		})
	})
}

func (s *IdentityService) EnrollMFA(ctx context.Context, userID string) (*MFAEnrollmentResult, error) {
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var existingCount int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mfa_methods WHERE user_id = $1 AND verified = true`, userID).Scan(&existingCount)
	if err != nil {
		return nil, fmt.Errorf("check existing mfa: %w", err)
	}
	if existingCount > 0 {
		return nil, ErrMFAAlreadyEnrolled
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		return nil, err
	}

	// Remove any stale, unconfirmed enrollments before creating a fresh one.
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM mfa_methods WHERE user_id = $1 AND verified = false`, userID); err != nil {
		return nil, fmt.Errorf("clean pending mfa enrollments: %w", err)
	}

	var mfa MFAMethod
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO mfa_methods (user_id, method, secret, verified, created_at)
			VALUES ($1, 'totp', $2, false, NOW())
			RETURNING id, user_id, method, secret, verified, created_at
		`, userID, secret).Scan(
			&mfa.ID, &mfa.UserID, &mfa.Method, &mfa.Secret, &mfa.Verified, &mfa.CreatedAt,
		); err != nil {
			return fmt.Errorf("enroll mfa: %w", err)
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			ID:           newID("aud"),
			EventType:    audit.EventTypeSecurity,
			TenantID:     user.TenantID,
			ActorID:      userID,
			Action:       "mfa.enroll",
			ResourceType: "mfa_method",
			ResourceID:   mfa.ID,
		})
	})
	if err != nil {
		return nil, err
	}

	uri := TOTPProvisioningURI(secret, user.Contact, "ComfortCurators")
	return &MFAEnrollmentResult{
		Method:          &mfa,
		Secret:          secret,
		ProvisioningURI: uri,
	}, nil
}

// ConfirmMFA verifies a TOTP code against a pending (unverified) enrollment
// and marks the method as verified only when the code is valid.
func (s *IdentityService) ConfirmMFA(ctx context.Context, params ConfirmMFAParams) error {
	user, err := s.getUserByID(ctx, params.UserID)
	if err != nil {
		return err
	}

	var method MFAMethod
	err = s.pool.QueryRow(ctx, `
		SELECT id, user_id, method, secret, verified, created_at
		FROM mfa_methods
		WHERE user_id = $1 AND verified = false
		ORDER BY created_at DESC
		LIMIT 1
	`, params.UserID).Scan(
		&method.ID, &method.UserID, &method.Method, &method.Secret, &method.Verified, &method.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrMFAEnrollmentNotFound
		}
		return fmt.Errorf("lookup pending mfa enrollment: %w", err)
	}

	ok, err := VerifyTOTP(method.Secret, params.Code, time.Now().UTC(), TOTPSkew)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMFAInvalid
	}

	if _, err := s.pool.Exec(ctx,
		`UPDATE mfa_methods SET verified = true WHERE id = $1`, method.ID); err != nil {
		return fmt.Errorf("confirm mfa enrollment: %w", err)
	}

	s.appendAudit(ctx, audit.AuditEvent{
		EventType:    audit.EventTypeSecurity,
		TenantID:     user.TenantID,
		ActorID:      params.UserID,
		Action:       "mfa.confirm",
		ResourceType: "mfa_method",
		ResourceID:   method.ID,
	})

	return nil
}

// VerifyMFA verifies a TOTP code against the user's verified MFA method and,
// on success, records that the actor has recently satisfied MFA so that
// subsequent "check MFA" calls can succeed without demanding a fresh code.
func (s *IdentityService) VerifyMFA(ctx context.Context, params VerifyMFAParams) error {
	user, err := s.getUserByID(ctx, params.UserID)
	if err != nil {
		return err
	}

	var method MFAMethod
	err = s.pool.QueryRow(ctx, `
		SELECT id, user_id, method, secret, verified, created_at
		FROM mfa_methods
		WHERE user_id = $1 AND verified = true
		ORDER BY created_at DESC
		LIMIT 1
	`, params.UserID).Scan(
		&method.ID, &method.UserID, &method.Method, &method.Secret, &method.Verified, &method.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrMFAUnverified
		}
		return fmt.Errorf("lookup verified mfa method: %w", err)
	}

	ok, err := VerifyTOTP(method.Secret, params.Code, time.Now().UTC(), TOTPSkew)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMFAInvalid
	}

	s.markMFASatisfied(params.UserID)

	s.appendAudit(ctx, audit.AuditEvent{
		EventType:    audit.EventTypeSecurity,
		TenantID:     user.TenantID,
		ActorID:      params.UserID,
		Action:       "mfa.verify",
		ResourceType: "mfa_method",
		ResourceID:   method.ID,
	})

	return nil
}

func (s *IdentityService) CheckMFARequired(ctx context.Context, subject security.Subject, action security.Action) error {
	return s.mfaVerifier.RequireMFA(ctx, subject, action)
}

func (s *IdentityService) GetUserByID(ctx context.Context, userID string) (*User, error) {
	return s.getUserByID(ctx, userID)
}

func (s *IdentityService) getUserByID(ctx context.Context, userID string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, contact, role, state, created_at, updated_at
		FROM users
		WHERE id = $1
	`, userID).Scan(
		&user.ID, &user.TenantID, &user.Contact, &user.Role, &user.State,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &user, nil
}

func (s *IdentityService) getUserByTenantAndContact(ctx context.Context, tenantID, contact string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, contact, role, state, created_at, updated_at
		FROM users
		WHERE tenant_id = $1 AND contact = $2 AND state = 'active'
	`, tenantID, contact).Scan(
		&user.ID, &user.TenantID, &user.Contact, &user.Role, &user.State,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by contact: %w", err)
	}
	return &user, nil
}

func (s *IdentityService) userHasVerifiedMFA(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM mfa_methods WHERE user_id = $1 AND verified = true)`, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check mfa: %w", err)
	}
	return exists, nil
}

func (s *IdentityService) cleanupExpiredOTPs(ctx context.Context, userID string) {
	_, _ = s.pool.Exec(ctx, `
		DELETE FROM authentication_methods WHERE user_id = $1 AND expires_at < NOW()
	`, userID)
}

func (s *IdentityService) appendAudit(ctx context.Context, evt audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if evt.ID == "" {
		evt.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, evt); err != nil {
		return
	}
}
