package hermes

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemStore is an in-memory implementation of Store used by tests and by the
// acceptance runner. It mirrors the durable PostgreSQL store's behavior,
// including idempotent replay semantics.
type MemStore struct {
	mu         sync.Mutex
	drafts     map[string]*HermesDraft
	reviews    map[string]*HermesReview
	deliveries map[string]*HermesDelivery
	byDraft    map[string]string
	byKey      map[string]string
	next       int
}

func NewMemStore() *MemStore {
	return &MemStore{
		drafts:     make(map[string]*HermesDraft),
		reviews:    make(map[string]*HermesReview),
		deliveries: make(map[string]*HermesDelivery),
		byDraft:    make(map[string]string),
		byKey:      make(map[string]string),
	}
}

func (s *MemStore) nextID(prefix string) string {
	s.next++
	return fmt.Sprintf("%s-%06d", prefix, s.next)
}

func (s *MemStore) InsertDraft(ctx context.Context, d *HermesDraft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.DraftID == "" {
		d.DraftID = s.nextID("herd")
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	cp := *d
	facts := make([]ApprovedFact, len(d.Facts))
	copy(facts, d.Facts)
	cp.Facts = facts
	s.drafts[d.DraftID] = &cp
	return nil
}

func (s *MemStore) GetDraft(ctx context.Context, tenantID, draftID string) (*HermesDraft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[draftID]
	if !ok || d.TenantID != tenantID {
		return nil, fmt.Errorf("%w: %s", ErrDraftNotFound, draftID)
	}
	cp := *d
	facts := make([]ApprovedFact, len(d.Facts))
	copy(facts, d.Facts)
	cp.Facts = facts
	return &cp, nil
}

func (s *MemStore) UpdateDraftState(ctx context.Context, tenantID, draftID, state string) (*HermesDraft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[draftID]
	if !ok || d.TenantID != tenantID {
		return nil, fmt.Errorf("%w: %s", ErrDraftNotFound, draftID)
	}
	d.State = state
	d.UpdatedAt = time.Now().UTC()
	d.Version++
	cp := *d
	facts := make([]ApprovedFact, len(d.Facts))
	copy(facts, d.Facts)
	cp.Facts = facts
	return &cp, nil
}

func (s *MemStore) InsertReview(ctx context.Context, r *HermesReview) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ReviewID == "" {
		r.ReviewID = s.nextID("herr")
	}
	if r.ReviewedAt.IsZero() {
		r.ReviewedAt = time.Now().UTC()
	}
	cp := *r
	s.reviews[r.ReviewID] = &cp
	return nil
}

func (s *MemStore) InsertDelivery(ctx context.Context, d *HermesDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.DeliveryID == "" {
		d.DeliveryID = s.nextID("herd")
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = DeliveryStateQueued
	}
	cp := *d
	s.deliveries[d.DeliveryID] = &cp
	s.byDraft[d.DraftID] = d.DeliveryID
	if d.IdempotencyKey != "" {
		s.byKey[d.IdempotencyKey] = d.DeliveryID
	}
	return nil
}

func (s *MemStore) GetDeliveryByDraft(ctx context.Context, tenantID, draftID string) (*HermesDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byDraft[draftID]
	if !ok {
		return nil, ErrDeliveryNotFound
	}
	d, ok := s.deliveries[id]
	if !ok || d.TenantID != tenantID {
		return nil, ErrDeliveryNotFound
	}
	return s.getDeliveryLocked(d)
}

func (s *MemStore) GetDeliveryByKey(ctx context.Context, tenantID, idempotencyKey string) (*HermesDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idempotencyKey == "" {
		return nil, ErrDeliveryNotFound
	}
	id, ok := s.byKey[idempotencyKey]
	if !ok {
		return nil, ErrDeliveryNotFound
	}
	d, ok := s.deliveries[id]
	if !ok || d.TenantID != tenantID {
		return nil, ErrDeliveryNotFound
	}
	return s.getDeliveryLocked(d)
}

func (s *MemStore) GetDelivery(ctx context.Context, tenantID, deliveryID string) (*HermesDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deliveries[deliveryID]
	if !ok || d.TenantID != tenantID {
		return nil, ErrDeliveryNotFound
	}
	return s.getDeliveryLocked(d)
}

func (s *MemStore) getDeliveryLocked(d *HermesDelivery) (*HermesDelivery, error) {
	cp := *d
	return &cp, nil
}

func (s *MemStore) ListDeliveries(ctx context.Context, tenantID string) ([]HermesDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []HermesDelivery
	for _, d := range s.deliveries {
		if d.TenantID != tenantID {
			continue
		}
		out = append(out, *d)
	}
	return out, nil
}
