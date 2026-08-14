package documents

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	var b [12]byte
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

type DocumentStore struct {
	pool *pgxpool.Pool
}

func NewDocumentStore(pool *pgxpool.Pool) *DocumentStore {
	return &DocumentStore{pool: pool}
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

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func computeReceiptHash(packetID string, refs []DocumentVersionRef, confirmedBy string, confirmedAt time.Time) string {
	h := sha256.New()
	h.Write([]byte(packetID))
	for _, r := range refs {
		h.Write([]byte(r.DocumentVersionID))
		h.Write([]byte(r.ContentHash))
	}
	h.Write([]byte(confirmedBy))
	h.Write([]byte(confirmedAt.Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))
}

// --- Document CRUD ---

func (s *DocumentStore) InsertDocument(ctx context.Context, q querier, doc *Document) error {
	doc.ID = newID("doc")
	_, err := q.Exec(ctx, `
		INSERT INTO documents (
			id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, doc.ID, doc.TenantID, doc.PropertyID, doc.Title, doc.DocumentType,
		doc.Status, doc.ExpiresAt, doc.CurrentVersion)
	return err
}

func (s *DocumentStore) GetDocument(ctx context.Context, tenantID, documentID string) (*Document, error) {
	var doc Document
	var expiresAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version, version,
			created_at, updated_at
		FROM documents
		WHERE id = $1 AND tenant_id = $2
	`, documentID, tenantID).Scan(
		&doc.ID, &doc.TenantID, &doc.PropertyID, &doc.Title, &doc.DocumentType,
		&doc.Status, &expiresAt, &doc.CurrentVersion, &doc.Version,
		&doc.CreatedAt, &doc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentNotFound
	}
	if expiresAt != nil {
		doc.ExpiresAt = expiresAt
	}
	return &doc, err
}

func (s *DocumentStore) ListDocuments(ctx context.Context, tenantID, propertyID string) ([]Document, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version, version,
			created_at, updated_at
		FROM documents
		WHERE tenant_id = $1 AND property_id = $2
		ORDER BY created_at DESC
	`, tenantID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		var doc Document
		var expiresAt *time.Time
		if err := rows.Scan(
			&doc.ID, &doc.TenantID, &doc.PropertyID, &doc.Title, &doc.DocumentType,
			&doc.Status, &expiresAt, &doc.CurrentVersion, &doc.Version,
			&doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if expiresAt != nil {
			doc.ExpiresAt = expiresAt
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

// --- Document Version ---

func (s *DocumentStore) InsertVersion(ctx context.Context, q querier, v *DocumentVersion) error {
	v.ID = newID("dvr")
	_, err := q.Exec(ctx, `
		INSERT INTO document_versions (
			id, document_id, tenant_id, version_number,
			content_hash, object_key, filename, content_type,
			size_bytes, uploaded_by, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, v.ID, v.DocumentID, v.TenantID, v.VersionNumber,
		v.ContentHash, v.ObjectKey, v.Filename, v.ContentType,
		v.SizeBytes, v.UploadedBy, v.Metadata)
	if isUniqueViolation(err) {
		return ErrDuplicateVersion
	}
	return err
}

func (s *DocumentStore) GetVersion(ctx context.Context, tenantID, versionID string) (*DocumentVersion, error) {
	var v DocumentVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, document_id, tenant_id, version_number,
			content_hash, object_key, filename, content_type,
			size_bytes, uploaded_by, metadata, created_at
		FROM document_versions
		WHERE id = $1 AND tenant_id = $2
	`, versionID, tenantID).Scan(
		&v.ID, &v.DocumentID, &v.TenantID, &v.VersionNumber,
		&v.ContentHash, &v.ObjectKey, &v.Filename, &v.ContentType,
		&v.SizeBytes, &v.UploadedBy, &v.Metadata, &v.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentVersionNotFound
	}
	return &v, err
}

func (s *DocumentStore) GetVersionByNumber(ctx context.Context, tenantID, documentID string, versionNumber int) (*DocumentVersion, error) {
	var v DocumentVersion
	err := s.pool.QueryRow(ctx, `
		SELECT id, document_id, tenant_id, version_number,
			content_hash, object_key, filename, content_type,
			size_bytes, uploaded_by, metadata, created_at
		FROM document_versions
		WHERE document_id = $1 AND tenant_id = $2 AND version_number = $3
	`, documentID, tenantID, versionNumber).Scan(
		&v.ID, &v.DocumentID, &v.TenantID, &v.VersionNumber,
		&v.ContentHash, &v.ObjectKey, &v.Filename, &v.ContentType,
		&v.SizeBytes, &v.UploadedBy, &v.Metadata, &v.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentVersionNotFound
	}
	return &v, err
}

func (s *DocumentStore) ListVersions(ctx context.Context, tenantID, documentID string) ([]DocumentVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, document_id, tenant_id, version_number,
			content_hash, object_key, filename, content_type,
			size_bytes, uploaded_by, metadata, created_at
		FROM document_versions
		WHERE document_id = $1 AND tenant_id = $2
		ORDER BY version_number DESC
	`, documentID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DocumentVersion
	for rows.Next() {
		var v DocumentVersion
		if err := rows.Scan(
			&v.ID, &v.DocumentID, &v.TenantID, &v.VersionNumber,
			&v.ContentHash, &v.ObjectKey, &v.Filename, &v.ContentType,
			&v.SizeBytes, &v.UploadedBy, &v.Metadata, &v.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *DocumentStore) BumpDocumentVersion(ctx context.Context, q querier, tenantID, documentID string) (*Document, error) {
	var doc Document
	var expiresAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE documents
		SET current_version = current_version + 1,
			version = version + 1,
			updated_at = $3
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version, version,
			created_at, updated_at
	`, documentID, tenantID, now).Scan(
		&doc.ID, &doc.TenantID, &doc.PropertyID, &doc.Title, &doc.DocumentType,
		&doc.Status, &expiresAt, &doc.CurrentVersion, &doc.Version,
		&doc.CreatedAt, &doc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentNotFound
	}
	if expiresAt != nil {
		doc.ExpiresAt = expiresAt
	}
	return &doc, err
}

func (s *DocumentStore) UpdateDocumentStatus(ctx context.Context, q querier, tenantID, documentID, status string) (*Document, error) {
	var doc Document
	var expiresAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE documents
		SET status = $3, updated_at = $4, version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version, version,
			created_at, updated_at
	`, documentID, tenantID, status, now).Scan(
		&doc.ID, &doc.TenantID, &doc.PropertyID, &doc.Title, &doc.DocumentType,
		&doc.Status, &expiresAt, &doc.CurrentVersion, &doc.Version,
		&doc.CreatedAt, &doc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentNotFound
	}
	if expiresAt != nil {
		doc.ExpiresAt = expiresAt
	}
	return &doc, err
}

// --- Extractions ---

func (s *DocumentStore) InsertExtraction(ctx context.Context, q querier, ext *Extraction) error {
	ext.ID = newID("dex")
	_, err := q.Exec(ctx, `
		INSERT INTO document_extractions (
			id, document_version_id, tenant_id,
			field_name, field_value, field_category,
			source_location, confidence, confidence_score,
			extracted_by, extracted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, ext.ID, ext.DocumentVersionID, ext.TenantID,
		ext.FieldName, ext.FieldValue, ext.FieldCategory,
		ext.SourceLocation, ext.Confidence, ext.ConfidenceScore,
		ext.ExtractedBy, ext.ExtractedAt)
	return err
}

func (s *DocumentStore) ListExtractions(ctx context.Context, tenantID, versionID string) ([]Extraction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, document_version_id, tenant_id,
			field_name, field_value, field_category,
			source_location, confidence, confidence_score,
			extracted_by, extracted_at
		FROM document_extractions
		WHERE document_version_id = $1 AND tenant_id = $2
		ORDER BY field_name
	`, versionID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Extraction
	for rows.Next() {
		var ext Extraction
		if err := rows.Scan(
			&ext.ID, &ext.DocumentVersionID, &ext.TenantID,
			&ext.FieldName, &ext.FieldValue, &ext.FieldCategory,
			&ext.SourceLocation, &ext.Confidence, &ext.ConfidenceScore,
			&ext.ExtractedBy, &ext.ExtractedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ext)
	}
	return out, rows.Err()
}

func (s *DocumentStore) HasLowConfidenceOrLegalExtractions(ctx context.Context, tenantID, versionID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(1)
		FROM document_extractions
		WHERE document_version_id = $1 AND tenant_id = $2
			AND (confidence = 'low' OR field_category = 'legal')
	`, versionID, tenantID).Scan(&count)
	return count > 0, err
}

// --- Reviews ---

func (s *DocumentStore) InsertReview(ctx context.Context, q querier, review *Review) error {
	review.ID = newID("drv")
	_, err := q.Exec(ctx, `
		INSERT INTO document_reviews (
			id, document_id, document_version_id, tenant_id,
			reviewer_id, status, decision, comments, reviewed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, review.ID, review.DocumentID, review.DocumentVersionID, review.TenantID,
		review.ReviewerID, review.Status, review.Decision, review.Comments, review.ReviewedAt)
	return err
}

func (s *DocumentStore) GetReview(ctx context.Context, tenantID, reviewID string) (*Review, error) {
	var r Review
	err := s.pool.QueryRow(ctx, `
		SELECT id, document_id, document_version_id, tenant_id,
			reviewer_id, status, decision, comments,
			reviewed_at, created_at
		FROM document_reviews
		WHERE id = $1 AND tenant_id = $2
	`, reviewID, tenantID).Scan(
		&r.ID, &r.DocumentID, &r.DocumentVersionID, &r.TenantID,
		&r.ReviewerID, &r.Status, &r.Decision, &r.Comments,
		&r.ReviewedAt, &r.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReviewNotFound
	}
	return &r, err
}

func (s *DocumentStore) ListReviews(ctx context.Context, tenantID, documentID string) ([]Review, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, document_id, document_version_id, tenant_id,
			reviewer_id, status, decision, comments,
			reviewed_at, created_at
		FROM document_reviews
		WHERE document_id = $1 AND tenant_id = $2
		ORDER BY reviewed_at DESC
	`, documentID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(
			&r.ID, &r.DocumentID, &r.DocumentVersionID, &r.TenantID,
			&r.ReviewerID, &r.Status, &r.Decision, &r.Comments,
			&r.ReviewedAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Submission Packets ---

func (s *DocumentStore) InsertSubmissionPacket(ctx context.Context, q querier, packet *SubmissionPacket) error {
	packet.ID = newID("sbp")
	docIDsJSON, err := json.Marshal(packet.DocumentIDs)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO submission_packets (
			id, tenant_id, property_id, status, document_ids, created_by
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, packet.ID, packet.TenantID, packet.PropertyID, packet.Status, docIDsJSON, packet.CreatedBy)
	return err
}

func (s *DocumentStore) GetSubmissionPacket(ctx context.Context, tenantID, packetID string) (*SubmissionPacket, error) {
	var p SubmissionPacket
	var docIDsJSON []byte
	var submittedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, property_id, status,
			document_ids, created_by, created_at,
			submitted_at, version, updated_at
		FROM submission_packets
		WHERE id = $1 AND tenant_id = $2
	`, packetID, tenantID).Scan(
		&p.ID, &p.TenantID, &p.PropertyID, &p.Status,
		&docIDsJSON, &p.CreatedBy, &p.CreatedAt,
		&submittedAt, &p.Version, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubmissionPacketNotFound
	}
	if submittedAt != nil {
		p.SubmittedAt = submittedAt
	}
	if err := json.Unmarshal(docIDsJSON, &p.DocumentIDs); err != nil {
		p.DocumentIDs = []string{}
	}
	return &p, err
}

func (s *DocumentStore) UpdatePacketStatus(ctx context.Context, q querier, tenantID, packetID, status string) (*SubmissionPacket, error) {
	var p SubmissionPacket
	var docIDsJSON []byte
	var submittedAt *time.Time
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		UPDATE submission_packets
		SET status = $3,
			submitted_at = CASE WHEN $3 = 'submitted' THEN $4 ELSE submitted_at END,
			updated_at = $4,
			version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, property_id, status,
			document_ids, created_by, created_at,
			submitted_at, version, updated_at
	`, packetID, tenantID, status, now).Scan(
		&p.ID, &p.TenantID, &p.PropertyID, &p.Status,
		&docIDsJSON, &p.CreatedBy, &p.CreatedAt,
		&submittedAt, &p.Version, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubmissionPacketNotFound
	}
	if submittedAt != nil {
		p.SubmittedAt = submittedAt
	}
	if err := json.Unmarshal(docIDsJSON, &p.DocumentIDs); err != nil {
		p.DocumentIDs = []string{}
	}
	return &p, err
}

// --- Submission Receipt ---

func (s *DocumentStore) InsertReceipt(ctx context.Context, q querier, receipt *SubmissionReceipt) error {
	receipt.ID = newID("rcp")
	refsJSON, err := json.Marshal(receipt.DocumentVersionRefs)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO submission_receipts (
			id, packet_id, tenant_id, confirmed_by,
			receipt_hash, document_version_refs, confirmed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, receipt.ID, receipt.PacketID, receipt.TenantID, receipt.ConfirmedBy,
		receipt.ReceiptHash, refsJSON, receipt.ConfirmedAt)
	return err
}

func (s *DocumentStore) GetReceipt(ctx context.Context, tenantID, packetID string) (*SubmissionReceipt, error) {
	var r SubmissionReceipt
	var refsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, packet_id, tenant_id, confirmed_by,
			receipt_hash, document_version_refs, confirmed_at
		FROM submission_receipts
		WHERE packet_id = $1 AND tenant_id = $2
		ORDER BY confirmed_at DESC
		LIMIT 1
	`, packetID, tenantID).Scan(
		&r.ID, &r.PacketID, &r.TenantID, &r.ConfirmedBy,
		&r.ReceiptHash, &refsJSON, &r.ConfirmedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err := json.Unmarshal(refsJSON, &r.DocumentVersionRefs); err != nil {
		r.DocumentVersionRefs = []DocumentVersionRef{}
	}
	return &r, err
}

// --- Expiry detection ---

func (s *DocumentStore) FindExpiredDocuments(ctx context.Context, tenantID string) ([]Document, error) {
	now := time.Now().UTC()
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version, version,
			created_at, updated_at
		FROM documents
		WHERE tenant_id = $1
			AND expires_at IS NOT NULL
			AND expires_at <= $2
			AND status NOT IN ('expired', 'superseded', 'revoked')
	`, tenantID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		var doc Document
		var expiresAt *time.Time
		if err := rows.Scan(
			&doc.ID, &doc.TenantID, &doc.PropertyID, &doc.Title, &doc.DocumentType,
			&doc.Status, &expiresAt, &doc.CurrentVersion, &doc.Version,
			&doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if expiresAt != nil {
			doc.ExpiresAt = expiresAt
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (s *DocumentStore) FindDocumentsNearingExpiry(ctx context.Context, tenantID string, withinDays int) ([]Document, error) {
	now := time.Now().UTC()
	threshold := now.Add(time.Duration(withinDays) * 24 * time.Hour)
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, property_id, title, document_type,
			status, expires_at, current_version, version,
			created_at, updated_at
		FROM documents
		WHERE tenant_id = $1
			AND expires_at IS NOT NULL
			AND expires_at > $2
			AND expires_at <= $3
			AND status NOT IN ('expired', 'superseded', 'revoked')
	`, tenantID, now, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		var doc Document
		var expiresAt *time.Time
		if err := rows.Scan(
			&doc.ID, &doc.TenantID, &doc.PropertyID, &doc.Title, &doc.DocumentType,
			&doc.Status, &expiresAt, &doc.CurrentVersion, &doc.Version,
			&doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if expiresAt != nil {
			doc.ExpiresAt = expiresAt
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}
