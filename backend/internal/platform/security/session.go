package security

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionRevoked  = errors.New("session has been revoked")
	ErrSessionExpired  = errors.New("session has expired")
)

type SessionID string

type Session struct {
	ID           SessionID
	ActorID      string
	TenantID     string
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	RevokeReason string
}

type SessionRevocation struct {
	SessionID SessionID
	Reason    string
	RevokedAt time.Time
	RevokedBy string
}

type SessionStore interface {
	GetSession(ctx context.Context, id SessionID) (*Session, error)
	RevokeSession(ctx context.Context, revocation SessionRevocation) error
	IsRevoked(ctx context.Context, id SessionID) (bool, error)
}

type InMemorySessionStore struct {
	sessions    map[SessionID]*Session
	revocations map[SessionID]*SessionRevocation
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions:    make(map[SessionID]*Session),
		revocations: make(map[SessionID]*SessionRevocation),
	}
}

func (s *InMemorySessionStore) AddSession(session *Session) {
	s.sessions[session.ID] = session
}

func (s *InMemorySessionStore) GetSession(ctx context.Context, id SessionID) (*Session, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.RevokedAt != nil {
		return nil, ErrSessionRevoked
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return sess, nil
}

func (s *InMemorySessionStore) RevokeSession(ctx context.Context, revocation SessionRevocation) error {
	sess, ok := s.sessions[revocation.SessionID]
	if !ok {
		return ErrSessionNotFound
	}
	now := time.Now()
	sess.RevokedAt = &now
	sess.RevokeReason = revocation.Reason
	s.revocations[revocation.SessionID] = &revocation
	return nil
}

func (s *InMemorySessionStore) IsRevoked(ctx context.Context, id SessionID) (bool, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return false, ErrSessionNotFound
	}
	return sess.RevokedAt != nil, nil
}
