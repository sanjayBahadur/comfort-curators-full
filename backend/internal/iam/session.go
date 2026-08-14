package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sqler interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const (
	DefaultSessionTTL = 24 * time.Hour
)

type SessionStore struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

func (s *SessionStore) Create(ctx context.Context, userID, tenantID string, roles []string) (*Session, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return nil, err
	}

	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, fmt.Errorf("marshal roles: %w", err)
	}

	session := &Session{
		ID:        token,
		UserID:    userID,
		TenantID:  tenantID,
		ActorID:   userID,
		Roles:     roles,
		ExpiresAt: time.Now().UTC().Add(DefaultSessionTTL),
		CreatedAt: time.Now().UTC(),
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, tenant_id, actor_id, roles, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, session.ID, session.UserID, session.TenantID, session.ActorID, rolesJSON, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

func (s *SessionStore) CreateTx(ctx context.Context, q sqler, userID, tenantID string, roles []string) (*Session, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return nil, err
	}

	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, fmt.Errorf("marshal roles: %w", err)
	}

	session := &Session{
		ID:        token,
		UserID:    userID,
		TenantID:  tenantID,
		ActorID:   userID,
		Roles:     roles,
		ExpiresAt: time.Now().UTC().Add(DefaultSessionTTL),
		CreatedAt: time.Now().UTC(),
	}

	_, err = q.Exec(ctx, `
		INSERT INTO sessions (id, user_id, tenant_id, actor_id, roles, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, session.ID, session.UserID, session.TenantID, session.ActorID, rolesJSON, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

func (s *SessionStore) RevokeTx(ctx context.Context, q sqler, token, reason, revokedBy string) error {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1)`, token).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return ErrSessionNotFound
	}

	_, err = q.Exec(ctx, `
		INSERT INTO session_revocations (session_id, reason, revoked_by, revoked_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (session_id) DO NOTHING
	`, token, reason, revokedBy)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	session, err := s.scanSession(ctx, s.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, actor_id, roles, expires_at, created_at
		FROM sessions
		WHERE id = $1
	`, token))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	revoked, err := s.IsRevoked(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("check revocation: %w", err)
	}
	if revoked {
		return nil, ErrSessionRevoked
	}

	return session, nil
}

func (s *SessionStore) Revoke(ctx context.Context, token, reason, revokedBy string) error {
	if _, err := s.Get(ctx, token); err != nil {
		return fmt.Errorf("cannot revoke: %w", err)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO session_revocations (session_id, reason, revoked_by, revoked_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (session_id) DO NOTHING
	`, token, reason, revokedBy)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

func (s *SessionStore) IsRevoked(ctx context.Context, token string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM session_revocations WHERE session_id = $1)
	`, token).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check session revocation: %w", err)
	}
	return exists, nil
}

func (s *SessionStore) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM sessions WHERE expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *SessionStore) scanSession(ctx context.Context, row pgx.Row) (*Session, error) {
	var session Session
	var rolesJSON []byte
	err := row.Scan(&session.ID, &session.UserID, &session.TenantID,
		&session.ActorID, &rolesJSON, &session.ExpiresAt, &session.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rolesJSON, &session.Roles); err != nil {
		return nil, fmt.Errorf("unmarshal roles: %w", err)
	}
	return &session, nil
}
