package communications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type CommunicationsStore struct {
	pool *pgxpool.Pool
}

func NewCommunicationsStore(pool *pgxpool.Pool) *CommunicationsStore {
	return &CommunicationsStore{pool: pool}
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// --- message templates ---

const templateColumns = `id, tenant_id, template_key, audience, consent_class,
	channel, severity, status, created_at, updated_at`

func scanTemplate(row pgx.Row) (*MessageTemplate, error) {
	var t MessageTemplate
	err := row.Scan(
		&t.ID, &t.TenantID, &t.TemplateKey, &t.Audience, &t.ConsentClass,
		&t.Channel, &t.Severity, &t.Status, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (s *CommunicationsStore) InsertTemplate(ctx context.Context, q querier, t *MessageTemplate) error {
	if t.ID == "" {
		t.ID = newID("tpl")
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	if t.Status == "" {
		t.Status = TemplateStatusActive
	}
	if t.Channel == "" {
		t.Channel = ChannelPush
	}
	if t.Severity == "" {
		t.Severity = SeverityNormal
	}
	_, err := q.Exec(ctx, `INSERT INTO message_templates (
		id, tenant_id, template_key, audience, consent_class, channel, severity, status, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		t.ID, t.TenantID, t.TemplateKey, t.Audience, t.ConsentClass, t.Channel, t.Severity, t.Status, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert message template: %w", err)
	}
	return nil
}

func (s *CommunicationsStore) GetTemplateByKey(ctx context.Context, tenantID, templateKey string) (*MessageTemplate, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+templateColumns+` FROM message_templates
		WHERE tenant_id=$1 AND template_key=$2`, tenantID, templateKey)
	return scanTemplate(row)
}

func (s *CommunicationsStore) GetTemplateByID(ctx context.Context, tenantID, templateID string) (*MessageTemplate, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+templateColumns+` FROM message_templates
		WHERE tenant_id=$1 AND id=$2`, tenantID, templateID)
	return scanTemplate(row)
}

func (s *CommunicationsStore) ListTemplates(ctx context.Context, tenantID, audience string) ([]MessageTemplate, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+templateColumns+` FROM message_templates
		WHERE tenant_id=$1 AND ($2='' OR audience=$2) ORDER BY template_key`, tenantID, audience)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []MessageTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *t)
	}
	return templates, rows.Err()
}

func (s *CommunicationsStore) InsertTemplateVersion(ctx context.Context, q querier, v *TemplateVersion) error {
	if v.ID == "" {
		v.ID = newID("tplv")
	}
	if v.Version == 0 {
		v.Version = 1
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO message_template_versions (
		id, tenant_id, template_id, version, language, subject, body, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		v.ID, v.TenantID, v.TemplateID, v.Version, v.Language, v.Subject, v.Body, v.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert template version: %w", err)
	}
	return nil
}

// LatestTemplateVersion returns the highest version available for a template
// and language. When exactLanguage is empty, the highest version of any
// language is returned so a caller can detect available fallbacks.
func (s *CommunicationsStore) LatestTemplateVersion(ctx context.Context, tenantID, templateID, exactLanguage string) (*TemplateVersion, error) {
	var v TemplateVersion
	var body string
	row := s.pool.QueryRow(ctx, `SELECT id, tenant_id, template_id, version, language, subject, body, created_at
		FROM message_template_versions
		WHERE tenant_id=$1 AND template_id=$2 AND ($3='' OR language=$3)
		ORDER BY version DESC, created_at DESC
		LIMIT 1`, tenantID, templateID, exactLanguage)
	if err := row.Scan(&v.ID, &v.TenantID, &v.TemplateID, &v.Version, &v.Language, &v.Subject, &body, &v.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTemplateVersionMissing
		}
		return nil, err
	}
	v.Body = body
	return &v, nil
}

func (s *CommunicationsStore) NextTemplateVersion(ctx context.Context, tenantID, templateID string) (int, error) {
	var maxVersion int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM message_template_versions
		WHERE tenant_id=$1 AND template_id=$2`, tenantID, templateID).Scan(&maxVersion)
	if err != nil {
		return 0, err
	}
	return maxVersion + 1, nil
}

// --- communication preferences ---

const preferencesColumns = `id, tenant_id, recipient_id, audience,
	consent_transactional, consent_urgent, consent_marketing, consent_sponsored,
	channel, severity, quiet_hours_start_minute, quiet_hours_end_minute,
	escalation_contacts, version, created_at, updated_at`

func scanPreferences(row pgx.Row) (*CommunicationPreferences, error) {
	var p CommunicationPreferences
	var escalationJSON []byte
	err := row.Scan(
		&p.ID, &p.TenantID, &p.RecipientID, &p.Audience,
		&p.ConsentTransactional, &p.ConsentUrgent, &p.ConsentMarketing, &p.ConsentSponsored,
		&p.Channel, &p.Severity, &p.QuietHoursStartMinute, &p.QuietHoursEndMinute,
		&escalationJSON, &p.Version, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPreferencesNotFound
		}
		return nil, err
	}
	if len(escalationJSON) > 0 {
		_ = json.Unmarshal(escalationJSON, &p.EscalationContacts)
	}
	return &p, nil
}

func (s *CommunicationsStore) UpsertPreferences(ctx context.Context, q querier, p *CommunicationPreferences) (*CommunicationPreferences, error) {
	if p.ID == "" {
		p.ID = newID("pref")
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.Version == 0 {
		p.Version = 1
	}
	p.UpdatedAt = now

	escalationJSON, err := json.Marshal(p.EscalationContacts)
	if err != nil {
		return nil, fmt.Errorf("marshal escalation contacts: %w", err)
	}

	row := q.QueryRow(ctx, `INSERT INTO communication_preferences (
		id, tenant_id, recipient_id, audience,
		consent_transactional, consent_urgent, consent_marketing, consent_sponsored,
		channel, severity, quiet_hours_start_minute, quiet_hours_end_minute,
		escalation_contacts, version, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	ON CONFLICT (tenant_id, recipient_id, audience) DO UPDATE SET
		consent_transactional=EXCLUDED.consent_transactional,
		consent_urgent=EXCLUDED.consent_urgent,
		consent_marketing=EXCLUDED.consent_marketing,
		consent_sponsored=EXCLUDED.consent_sponsored,
		channel=EXCLUDED.channel,
		severity=EXCLUDED.severity,
		quiet_hours_start_minute=EXCLUDED.quiet_hours_start_minute,
		quiet_hours_end_minute=EXCLUDED.quiet_hours_end_minute,
		escalation_contacts=EXCLUDED.escalation_contacts,
		version=communication_preferences.version+1,
		updated_at=EXCLUDED.updated_at
	RETURNING `+preferencesColumns,
		p.ID, p.TenantID, p.RecipientID, p.Audience,
		p.ConsentTransactional, p.ConsentUrgent, p.ConsentMarketing, p.ConsentSponsored,
		p.Channel, p.Severity, p.QuietHoursStartMinute, p.QuietHoursEndMinute,
		escalationJSON, p.Version, p.CreatedAt, p.UpdatedAt,
	)

	stored, err := scanPreferences(row)
	if err != nil {
		return nil, fmt.Errorf("upsert communication preferences: %w", err)
	}
	return stored, nil
}

func (s *CommunicationsStore) GetPreferences(ctx context.Context, tenantID, recipientID, audience string) (*CommunicationPreferences, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+preferencesColumns+` FROM communication_preferences
		WHERE tenant_id=$1 AND recipient_id=$2 AND audience=$3`, tenantID, recipientID, audience)
	return scanPreferences(row)
}

// --- communication drafts ---

const draftColumns = `id, tenant_id, audience, recipient_id, source, template_key,
	consent_class, channel, severity, subject, body, status, requires_review, created_at, updated_at`

func scanDraft(row pgx.Row) (*CommunicationDraft, error) {
	var d CommunicationDraft
	var templateKey *string
	err := row.Scan(
		&d.ID, &d.TenantID, &d.Audience, &d.RecipientID, &d.Source, &templateKey,
		&d.ConsentClass, &d.Channel, &d.Severity, &d.Subject, &d.Body,
		&d.Status, &d.RequiresReview, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDraftNotFound
		}
		return nil, err
	}
	if templateKey != nil {
		d.TemplateKey = *templateKey
	}
	return &d, nil
}

func (s *CommunicationsStore) InsertDraft(ctx context.Context, q querier, d *CommunicationDraft) error {
	if d.ID == "" {
		d.ID = newID("drf")
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.Status == "" {
		d.Status = DraftStatusDraft
	}
	_, err := q.Exec(ctx, `INSERT INTO communication_drafts (
		id, tenant_id, audience, recipient_id, source, template_key, consent_class,
		channel, severity, subject, body, status, requires_review, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		d.ID, d.TenantID, d.Audience, d.RecipientID, d.Source, nullString(d.TemplateKey), d.ConsentClass,
		d.Channel, d.Severity, d.Subject, d.Body, d.Status, d.RequiresReview, d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert communication draft: %w", err)
	}
	return nil
}

func (s *CommunicationsStore) GetDraft(ctx context.Context, tenantID, draftID string) (*CommunicationDraft, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+draftColumns+` FROM communication_drafts
		WHERE tenant_id=$1 AND id=$2`, tenantID, draftID)
	return scanDraft(row)
}

func (s *CommunicationsStore) UpdateDraftStatus(ctx context.Context, q querier, tenantID, draftID, status string) (*CommunicationDraft, error) {
	row := q.QueryRow(ctx, `UPDATE communication_drafts
		SET status=$3, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
		RETURNING `+draftColumns, tenantID, draftID, status)
	return scanDraft(row)
}

func (s *CommunicationsStore) ListDrafts(ctx context.Context, tenantID, recipientID string) ([]CommunicationDraft, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+draftColumns+` FROM communication_drafts
		WHERE tenant_id=$1 AND ($2='' OR recipient_id=$2) ORDER BY created_at DESC`, tenantID, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var drafts []CommunicationDraft
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, *d)
	}
	return drafts, rows.Err()
}

// --- communication reviews ---

func (s *CommunicationsStore) InsertReview(ctx context.Context, q querier, r *CommunicationReview) error {
	if r.ID == "" {
		r.ID = newID("rev")
	}
	if r.ReviewedAt.IsZero() {
		r.ReviewedAt = time.Now().UTC()
	}
	_, err := q.Exec(ctx, `INSERT INTO communication_reviews (
		id, tenant_id, draft_id, reviewer_id, decision, reason, reviewed_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		r.ID, r.TenantID, r.DraftID, r.ReviewerID, r.Decision, r.Reason, r.ReviewedAt,
	)
	if err != nil {
		return fmt.Errorf("insert communication review: %w", err)
	}
	return nil
}

func (s *CommunicationsStore) ListReviews(ctx context.Context, tenantID, draftID string) ([]CommunicationReview, error) {
	rows, err := s.pool.Query(ctx, `SELECT
		id, tenant_id, draft_id, reviewer_id, decision, reason, reviewed_at
		FROM communication_reviews WHERE tenant_id=$1 AND draft_id=$2 ORDER BY reviewed_at`, tenantID, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []CommunicationReview
	for rows.Next() {
		var r CommunicationReview
		if err := rows.Scan(&r.ID, &r.TenantID, &r.DraftID, &r.ReviewerID, &r.Decision, &r.Reason, &r.ReviewedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

// --- deliveries ---

const deliveryColumns = `id, tenant_id, draft_id, recipient_id, audience,
	consent_class, channel, status, error, created_at, delivered_at, updated_at`

func scanDelivery(row pgx.Row) (*Delivery, error) {
	var d Delivery
	var draftID, errMsg *string
	var deliveredAt *time.Time
	err := row.Scan(
		&d.ID, &d.TenantID, &draftID, &d.RecipientID, &d.Audience,
		&d.ConsentClass, &d.Channel, &d.Status, &errMsg, &d.CreatedAt, &deliveredAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeliveryNotFound
		}
		return nil, err
	}
	if draftID != nil {
		d.DraftID = *draftID
	}
	if errMsg != nil {
		d.Error = *errMsg
	}
	d.DeliveredAt = deliveredAt
	return &d, nil
}

func (s *CommunicationsStore) InsertDelivery(ctx context.Context, q querier, d *Delivery) error {
	if d.ID == "" {
		d.ID = newID("dlv")
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.Status == "" {
		d.Status = DeliveryStatusQueued
	}
	_, err := q.Exec(ctx, `INSERT INTO deliveries (
		id, tenant_id, draft_id, recipient_id, audience, consent_class, channel,
		status, error, created_at, delivered_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		d.ID, d.TenantID, nullString(d.DraftID), d.RecipientID, d.Audience, d.ConsentClass, d.Channel,
		d.Status, nullString(d.Error), d.CreatedAt, d.DeliveredAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert delivery: %w", err)
	}
	return nil
}

func (s *CommunicationsStore) UpdateDeliveryStatus(ctx context.Context, q querier, tenantID, deliveryID, status, failure string) (*Delivery, error) {
	var deliveredAt any = nil
	if status == DeliveryStatusDelivered {
		deliveredAt = time.Now().UTC()
	}
	row := q.QueryRow(ctx, `UPDATE deliveries
		SET status=$3, error=$4, delivered_at=COALESCE($5, delivered_at), updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
		RETURNING `+deliveryColumns, tenantID, deliveryID, status, nullString(failure), deliveredAt)
	return scanDelivery(row)
}

func (s *CommunicationsStore) ListDeliveries(ctx context.Context, tenantID, recipientID string) ([]Delivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+deliveryColumns+` FROM deliveries
		WHERE tenant_id=$1 AND ($2='' OR recipient_id=$2) ORDER BY created_at DESC`, tenantID, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

// --- secure stay links ---

const linkColumns = `id, tenant_id, property_id, audience, recipient_id, purpose,
	token_hash, token_tail, expires_at, used_at, revoked_at, status, created_at`

func scanLink(row pgx.Row) (*SecureLink, error) {
	var l SecureLink
	var tokenHash string
	var usedAt, revokedAt *time.Time
	err := row.Scan(
		&l.ID, &l.TenantID, &l.PropertyID, &l.Audience, &l.RecipientID, &l.Purpose,
		&tokenHash, &l.TokenTail, &l.ExpiresAt, &usedAt, &revokedAt, &l.Status, &l.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLinkNotFound
		}
		return nil, err
	}
	l.UsedAt = usedAt
	l.RevokedAt = revokedAt
	return &l, nil
}

func (s *CommunicationsStore) InsertSecureLink(ctx context.Context, q querier, l *SecureLink) error {
	if l.ID == "" {
		l.ID = newID("lkn")
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	if l.Status == "" {
		l.Status = LinkStatusActive
	}
	if l.Purpose == "" {
		l.Purpose = "stay"
	}
	_, err := q.Exec(ctx, `INSERT INTO conversation_links (
		id, tenant_id, property_id, audience, recipient_id, purpose,
		token_hash, token_tail, expires_at, used_at, revoked_at, status, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		l.ID, l.TenantID, l.PropertyID, l.Audience, l.RecipientID, l.Purpose,
		l.TokenHash, l.TokenTail, l.ExpiresAt, l.UsedAt, l.RevokedAt, l.Status, l.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert secure stay link: %w", err)
	}
	return nil
}

// RedeemSecureLink atomically marks an unused, unexpired link as used and
// returns the link. It returns ErrLinkNotFound when no row matches so the
// caller can classify expired/used/revoked links by re-reading the record.
func (s *CommunicationsStore) RedeemSecureLink(ctx context.Context, tokenHash string) (*SecureLink, error) {
	row := s.pool.QueryRow(ctx, `UPDATE conversation_links
		SET used_at=NOW(), status='used'
		WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > NOW()
		RETURNING `+linkColumns, tokenHash)
	return scanLink(row)
}

func (s *CommunicationsStore) GetSecureLinkByHash(ctx context.Context, tokenHash string) (*SecureLink, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+linkColumns+` FROM conversation_links
		WHERE token_hash=$1`, tokenHash)
	return scanLink(row)
}

func (s *CommunicationsStore) ListSecureLinks(ctx context.Context, tenantID, propertyID string) ([]SecureLink, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+linkColumns+` FROM conversation_links
		WHERE tenant_id=$1 AND ($2='' OR property_id=$2) ORDER BY created_at DESC`, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []SecureLink
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, *l)
	}
	return links, rows.Err()
}

func (s *CommunicationsStore) RevokeSecureLink(ctx context.Context, q querier, tenantID, linkID string) (*SecureLink, error) {
	row := q.QueryRow(ctx, `UPDATE conversation_links
		SET revoked_at=NOW(), status='revoked'
		WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL
		RETURNING `+linkColumns, tenantID, linkID)
	return scanLink(row)
}
