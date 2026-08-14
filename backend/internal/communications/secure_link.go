package communications

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

// tokenBytes is the entropy of a stay-link token (32 bytes = 256 bits).
const tokenBytes = 32

func generateToken() (string, error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenTail(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}

// CreateSecureLink issues a short-lived, single-use stay link. The raw token
// is returned exactly once and is stored only as a hash.
func (s *CommunicationsService) CreateSecureLink(ctx context.Context, tenantID string, params SecureLinkParams, actorID string) (*SecureLink, string, error) {
	if !IsValidAudience(params.Audience) {
		return nil, "", ErrInvalidAudience
	}
	if params.PropertyID == "" || params.RecipientID == "" {
		return nil, "", ErrInvalidSecureLink
	}
	if params.ExpiresAt.IsZero() || !params.ExpiresAt.After(time.Now().UTC()) {
		return nil, "", ErrInvalidSecureLink
	}

	token, err := generateToken()
	if err != nil {
		return nil, "", err
	}

	link := &SecureLink{
		TenantID:    tenantID,
		PropertyID:  params.PropertyID,
		Audience:    params.Audience,
		RecipientID: params.RecipientID,
		Purpose:     params.Purpose,
		TokenTail:   tokenTail(token),
		TokenHash:   hashToken(token),
		ExpiresAt:   params.ExpiresAt.UTC(),
		Status:      LinkStatusActive,
	}

	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertSecureLink(ctx, tx, link); err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				TenantID:     tenantID,
				ActorID:      actorID,
				Action:       "communications.secure_link.created",
				ResourceType: "conversation_link",
				ResourceID:   link.ID,
				NewState:     marshalJSON(link),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	return link, token, nil
}

// RedeemSecureLink validates expiry and single-use state atomically. A link
// that has expired, been used, or been revoked rejects redemption. Replaying
// the same token after a successful redemption always fails.
func (s *CommunicationsService) RedeemSecureLink(ctx context.Context, token string) (*SecureLink, error) {
	if token == "" {
		return nil, ErrLinkNotFound
	}

	hash := hashToken(token)
	link, err := s.store.RedeemSecureLink(ctx, hash)
	if err != nil {
		if !errors.Is(err, ErrLinkNotFound) {
			return nil, err
		}
		stored, lookupErr := s.store.GetSecureLinkByHash(ctx, hash)
		if lookupErr != nil {
			return nil, lookupErr
		}
		now := time.Now().UTC()
		switch {
		case stored.RevokedAt != nil || stored.Status == LinkStatusRevoked:
			return nil, ErrLinkRevoked
		case stored.UsedAt != nil || stored.Status == LinkStatusUsed:
			return nil, ErrLinkAlreadyUsed
		case !stored.ExpiresAt.After(now):
			return nil, ErrLinkExpired
		default:
			return nil, ErrLinkNotFound
		}
	}

	s.appendAudit(ctx, audit.AuditEvent{
		TenantID:     link.TenantID,
		ActorID:      "link-redeemer",
		Action:       "communications.secure_link.redeemed",
		ResourceType: "conversation_link",
		ResourceID:   link.ID,
		NewState:     marshalJSON(link),
	})

	return link, nil
}

// RevokeSecureLink immediately revokes an active link.
func (s *CommunicationsService) RevokeSecureLink(ctx context.Context, tenantID, linkID string) (*SecureLink, error) {
	var link *SecureLink
	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		link, err = s.store.RevokeSecureLink(ctx, tx, tenantID, linkID)
		if err != nil {
			return err
		}
		if s.auditStore != nil {
			if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				TenantID:     tenantID,
				ActorID:      "link-revoker",
				Action:       "communications.secure_link.revoked",
				ResourceType: "conversation_link",
				ResourceID:   link.ID,
				NewState:     marshalJSON(link),
			}); err != nil {
				return fmt.Errorf("audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return link, nil
}

func (s *CommunicationsService) ListSecureLinks(ctx context.Context, tenantID, propertyID string) ([]SecureLink, error) {
	return s.store.ListSecureLinks(ctx, tenantID, propertyID)
}
