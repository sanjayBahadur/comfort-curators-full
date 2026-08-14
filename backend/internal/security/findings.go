package security

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var ErrFindingNotFound = errors.New("finding not found")

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Status string

const (
	StatusOpen      Status = "open"
	StatusMitigated Status = "mitigated"
	StatusAccepted  Status = "accepted"
	StatusFalsePos  Status = "false_positive"
	StatusWontFix   Status = "wont_fix"
)

type FindingCategory string

const (
	CategorySecret      FindingCategory = "secret_exposure"
	CategoryInjection   FindingCategory = "injection"
	CategoryAuthZ       FindingCategory = "authorization"
	CategoryRate        FindingCategory = "rate_limit"
	CategoryDependency  FindingCategory = "dependency"
	CategoryContainer   FindingCategory = "container"
	CategoryWebhook     FindingCategory = "webhook"
	CategoryObjectOwner FindingCategory = "object_ownership"
	CategoryPrivileged  FindingCategory = "privileged_access"
	CategoryNetwork     FindingCategory = "network"
)

type Finding struct {
	ID          string          `json:"id"`
	Category    FindingCategory `json:"category"`
	Severity    Severity        `json:"severity"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Evidence    json.RawMessage `json:"evidence,omitempty"`
	Status      Status          `json:"status"`
	Reason      string          `json:"reason,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
}

type FindingStore struct {
	mu       sync.RWMutex
	findings map[string]*Finding
}

func NewFindingStore() *FindingStore {
	return &FindingStore{
		findings: make(map[string]*Finding),
	}
}

func (s *FindingStore) Upsert(id string, f *Finding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.findings[id]; ok {
		f.CreatedAt = existing.CreatedAt
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	s.findings[id] = f
}

func (s *FindingStore) Get(id string) (*Finding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.findings[id]
	if !ok {
		return nil, ErrFindingNotFound
	}
	return f, nil
}

func (s *FindingStore) Disposition(id string, status Status, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.findings[id]
	if !ok {
		return ErrFindingNotFound
	}
	f.Status = status
	f.Reason = reason
	now := time.Now()
	f.ResolvedAt = &now
	return nil
}

func (s *FindingStore) UnresolvedHighOrCritical() []Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Finding
	for _, f := range s.findings {
		if (f.Severity == SeverityHigh || f.Severity == SeverityCritical) &&
			(f.Status == StatusOpen) {
			result = append(result, *f)
		}
	}
	return result
}

func (s *FindingStore) ByCategory(category FindingCategory) []Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Finding
	for _, f := range s.findings {
		if f.Category == category {
			result = append(result, *f)
		}
	}
	return result
}

func (s *FindingStore) All() []Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Finding, 0, len(s.findings))
	for _, f := range s.findings {
		result = append(result, *f)
	}
	return result
}
