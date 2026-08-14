package security

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrReplayedWebhook = errors.New("webhook request has already been processed")
	ErrExpiredWebhook  = errors.New("webhook token has expired")
	ErrInvalidWebhook  = errors.New("invalid webhook signature")
)

type WebhookRecord struct {
	ID        string
	Hash      string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type WebhookReplayProtector struct {
	mu        sync.RWMutex
	processed map[string]time.Time
	secret    []byte
	ttl       time.Duration
}

func NewWebhookReplayProtector(secret []byte, ttl time.Duration) *WebhookReplayProtector {
	return &WebhookReplayProtector{
		processed: make(map[string]time.Time),
		secret:    secret,
		ttl:       ttl,
	}
}

func (w *WebhookReplayProtector) Sign(payload []byte) string {
	mac := hmac.New(sha256.New, w.secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (w *WebhookReplayProtector) Verify(payload []byte, signature string) bool {
	expected := w.Sign(payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (w *WebhookReplayProtector) CheckReplay(ctx context.Context, id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.cleanupExpired()

	processedAt, exists := w.processed[id]
	if exists {
		if time.Since(processedAt) < w.ttl {
			return ErrReplayedWebhook
		}
	}

	w.processed[id] = time.Now()
	return nil
}

func (w *WebhookReplayProtector) cleanupExpired() {
	cutoff := time.Now().Add(-w.ttl)
	for id, processedAt := range w.processed {
		if processedAt.Before(cutoff) {
			delete(w.processed, id)
		}
	}
}

type SecureToken struct {
	Token     string
	ExpiresAt time.Time
	Used      bool
}

type SecureTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*SecureToken
}

func NewSecureTokenStore() *SecureTokenStore {
	return &SecureTokenStore{
		tokens: make(map[string]*SecureToken),
	}
}

func (s *SecureTokenStore) CreateToken(tokenValue string, ttl time.Duration) *SecureToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok := &SecureToken{
		Token:     tokenValue,
		ExpiresAt: time.Now().Add(ttl),
	}
	s.tokens[tokenValue] = tok
	return tok
}

func (s *SecureTokenStore) Consume(tokenValue string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.tokens[tokenValue]
	if !ok {
		return ErrInvalidWebhook
	}

	if time.Now().After(tok.ExpiresAt) {
		return ErrExpiredWebhook
	}

	if tok.Used {
		return ErrReplayedWebhook
	}

	tok.Used = true
	return nil
}

func (s *SecureTokenStore) Validate(tokenValue string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tok, ok := s.tokens[tokenValue]
	if !ok {
		return ErrInvalidWebhook
	}

	if time.Now().After(tok.ExpiresAt) {
		return ErrExpiredWebhook
	}

	if tok.Used {
		return ErrReplayedWebhook
	}

	return nil
}
