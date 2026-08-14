package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSchema creates the durable Hermes tables. Drafts, reviews and
// deliveries are never hard-deleted; delivery replay resolves to the existing
// row.
func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS hermes_drafts (
			draft_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			audience TEXT NOT NULL
				CHECK (audience IN ('owner', 'guest')),
			purpose TEXT NOT NULL,
			template_key TEXT NOT NULL DEFAULT '',
			language TEXT NOT NULL DEFAULT 'en',
			channel TEXT NOT NULL DEFAULT 'push',
			facts JSONB NOT NULL DEFAULT '[]',
			review_policy TEXT NOT NULL
				CHECK (review_policy IN ('approved_template', 'human_review')),
			state TEXT NOT NULL DEFAULT 'draft'
				CHECK (state IN ('draft', 'under_review', 'approved', 'rejected', 'delivered')),
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS hermes_reviews (
			review_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			draft_id TEXT NOT NULL REFERENCES hermes_drafts(draft_id),
			reviewer_id TEXT NOT NULL,
			decision TEXT NOT NULL
				CHECK (decision IN ('approved', 'rejected')),
			reason TEXT NOT NULL DEFAULT '',
			reviewed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS hermes_deliveries (
			delivery_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			draft_id TEXT NOT NULL REFERENCES hermes_drafts(draft_id),
			audience TEXT NOT NULL,
			recipient_id TEXT NOT NULL DEFAULT '',
			idempotency_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued'
				CHECK (status IN ('queued', 'sent', 'delivered', 'failed')),
			error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			delivered_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hermes_drafts_tenant
			ON hermes_drafts (tenant_id, state)`,
		`CREATE INDEX IF NOT EXISTS idx_hermes_reviews_draft
			ON hermes_reviews (tenant_id, draft_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hermes_deliveries_draft
			ON hermes_deliveries (tenant_id, draft_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_hermes_deliveries_key
			ON hermes_deliveries (tenant_id, idempotency_key)
			WHERE idempotency_key != ''`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("hermes: ensure schema: %w", err)
		}
	}
	return nil
}

// PGStore is the durable PostgreSQL implementation of Store.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) InsertDraft(ctx context.Context, d *HermesDraft) error {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	factsJSON, err := json.Marshal(d.Facts)
	if err != nil {
		return fmt.Errorf("hermes: marshal draft facts: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO hermes_drafts (
			draft_id, run_id, tenant_id, property_id, actor_id, audience, purpose,
			template_key, language, channel, facts, review_policy, state,
			subject, body, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		d.DraftID, d.RunID, d.TenantID, d.PropertyID, d.ActorID, d.Audience, d.Purpose,
		d.TemplateKey, d.Language, d.Channel, factsJSON, d.ReviewPolicy, d.State,
		d.Subject, d.Body, d.Version, d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("hermes: insert draft: %w", err)
	}
	return nil
}

const hermesDraftColumns = `draft_id, run_id, tenant_id, property_id, actor_id, audience,
	purpose, template_key, language, channel, facts, review_policy, state,
	subject, body, version, created_at, updated_at`

func scanDraft(row pgx.Row) (*HermesDraft, error) {
	var d HermesDraft
	var factsJSON []byte
	err := row.Scan(
		&d.DraftID, &d.RunID, &d.TenantID, &d.PropertyID, &d.ActorID, &d.Audience,
		&d.Purpose, &d.TemplateKey, &d.Language, &d.Channel, &factsJSON, &d.ReviewPolicy,
		&d.State, &d.Subject, &d.Body, &d.Version, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrDraftNotFound
		}
		return nil, fmt.Errorf("hermes: scan draft: %w", err)
	}
	if len(factsJSON) > 0 {
		if err := json.Unmarshal(factsJSON, &d.Facts); err != nil {
			return nil, fmt.Errorf("hermes: decode draft facts: %w", err)
		}
	}
	return &d, nil
}

func (s *PGStore) GetDraft(ctx context.Context, tenantID, draftID string) (*HermesDraft, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+hermesDraftColumns+` FROM hermes_drafts
		WHERE tenant_id=$1 AND draft_id=$2`, tenantID, draftID)
	return scanDraft(row)
}

func (s *PGStore) UpdateDraftState(ctx context.Context, tenantID, draftID, state string) (*HermesDraft, error) {
	row := s.pool.QueryRow(ctx, `UPDATE hermes_drafts
		SET state=$3, version=version+1, updated_at=NOW()
		WHERE tenant_id=$1 AND draft_id=$2
		RETURNING `+hermesDraftColumns, tenantID, draftID, state)
	return scanDraft(row)
}

func (s *PGStore) InsertReview(ctx context.Context, r *HermesReview) error {
	if r.ReviewedAt.IsZero() {
		r.ReviewedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hermes_reviews (
			review_id, tenant_id, draft_id, reviewer_id, decision, reason, reviewed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		r.ReviewID, r.TenantID, r.DraftID, r.ReviewerID, r.Decision, r.Reason, r.ReviewedAt,
	)
	if err != nil {
		return fmt.Errorf("hermes: insert review: %w", err)
	}
	return nil
}

const hermesDeliveryColumns = `delivery_id, tenant_id, draft_id, audience, recipient_id,
	idempotency_key, status, error, created_at, delivered_at, updated_at`

func scanDelivery(row pgx.Row) (*HermesDelivery, error) {
	var d HermesDelivery
	var errText string
	var deliveredAt *time.Time
	err := row.Scan(
		&d.DeliveryID, &d.TenantID, &d.DraftID, &d.Audience, &d.RecipientID,
		&d.IdempotencyKey, &d.Status, &errText, &d.CreatedAt, &deliveredAt, &d.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrDeliveryNotFound
		}
		return nil, fmt.Errorf("hermes: scan delivery: %w", err)
	}
	d.Error = errText
	d.DeliveredAt = deliveredAt
	return &d, nil
}

func (s *PGStore) InsertDelivery(ctx context.Context, d *HermesDelivery) error {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = DeliveryStateQueued
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hermes_deliveries (
			delivery_id, tenant_id, draft_id, audience, recipient_id,
			idempotency_key, status, error, created_at, delivered_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (tenant_id, idempotency_key) WHERE idempotency_key != '' DO NOTHING`,
		d.DeliveryID, d.TenantID, d.DraftID, d.Audience, d.RecipientID,
		d.IdempotencyKey, d.Status, d.Error, d.CreatedAt, d.DeliveredAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("hermes: insert delivery: %w", err)
	}
	return nil
}

func (s *PGStore) GetDeliveryByDraft(ctx context.Context, tenantID, draftID string) (*HermesDelivery, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+hermesDeliveryColumns+` FROM hermes_deliveries
		WHERE tenant_id=$1 AND draft_id=$2 ORDER BY created_at ASC LIMIT 1`, tenantID, draftID)
	return scanDelivery(row)
}

func (s *PGStore) GetDeliveryByKey(ctx context.Context, tenantID, idempotencyKey string) (*HermesDelivery, error) {
	if idempotencyKey == "" {
		return nil, ErrDeliveryNotFound
	}
	row := s.pool.QueryRow(ctx, `SELECT `+hermesDeliveryColumns+` FROM hermes_deliveries
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey)
	return scanDelivery(row)
}

func (s *PGStore) GetDelivery(ctx context.Context, tenantID, deliveryID string) (*HermesDelivery, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+hermesDeliveryColumns+` FROM hermes_deliveries
		WHERE tenant_id=$1 AND delivery_id=$2`, tenantID, deliveryID)
	return scanDelivery(row)
}

func (s *PGStore) ListDeliveries(ctx context.Context, tenantID string) ([]HermesDelivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+hermesDeliveryColumns+` FROM hermes_deliveries
		WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("hermes: list deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []HermesDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
