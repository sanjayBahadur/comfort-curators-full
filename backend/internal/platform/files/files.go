package files

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound           = errors.New("object not found")
	ErrGrantExpired       = errors.New("grant has expired")
	ErrGrantUsed          = errors.New("grant has already been used")
	ErrGrantInvalidType   = errors.New("grant type mismatch")
	ErrUnscannedObject    = errors.New("unscanned objects are not available for download")
	ErrContentTypeDenied  = errors.New("content type not allowed")
	ErrSizeLimitExceeded  = errors.New("object size exceeds limit")
	ErrScanStatusConflict = errors.New("invalid scan status transition")
	ErrInvalidGrantToken  = errors.New("invalid grant token")
	ErrTenantRequired     = errors.New("tenant_id is required")
)

type ScanStatus string

const (
	ScanStatusUnscanned   ScanStatus = "unscanned"
	ScanStatusScanning    ScanStatus = "scanning"
	ScanStatusClean       ScanStatus = "clean"
	ScanStatusInfected    ScanStatus = "infected"
	ScanStatusUnscannable ScanStatus = "unscannable"
)

type GrantType string

const (
	GrantTypeUpload   GrantType = "upload"
	GrantTypeDownload GrantType = "download"
)

var validScanTransitions = map[ScanStatus][]ScanStatus{
	ScanStatusUnscanned:   {ScanStatusScanning},
	ScanStatusScanning:    {ScanStatusClean, ScanStatusInfected, ScanStatusUnscannable},
	ScanStatusClean:       {},
	ScanStatusInfected:    {ScanStatusScanning},
	ScanStatusUnscannable: {},
}

var (
	downloadableScanStatuses = map[ScanStatus]bool{
		ScanStatusClean:       true,
		ScanStatusUnscannable: true,
	}
)

type FileObject struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	ObjectKey       string          `json:"object_key"`
	OriginalName    string          `json:"original_name"`
	ContentType     string          `json:"content_type"`
	SizeBytes       int64           `json:"size_bytes"`
	SHA256Hash      string          `json:"sha256_hash"`
	ScanStatus      ScanStatus      `json:"scan_status"`
	RetentionPolicy string          `json:"retention_policy"`
	RetentionUntil  *time.Time      `json:"retention_until,omitempty"`
	IsOriginal      bool            `json:"is_original"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	DeletedAt       *time.Time      `json:"deleted_at,omitempty"`
}

type Grant struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	ObjectID            *string    `json:"object_id,omitempty"`
	GrantType           GrantType  `json:"grant_type"`
	GrantToken          string     `json:"grant_token"`
	ExpiresAt           time.Time  `json:"expires_at"`
	MaxSizeBytes        *int64     `json:"max_size_bytes,omitempty"`
	AllowedContentTypes []string   `json:"allowed_content_types,omitempty"`
	UsedAt              *time.Time `json:"used_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type Config struct {
	MaxUploadSizeBytes  int64
	AllowedContentTypes []string
	UploadGrantTTL      time.Duration
	DownloadGrantTTL    time.Duration
	MaxObjectKeyLength  int
}

func DefaultConfig() Config {
	return Config{
		MaxUploadSizeBytes: 100 * 1024 * 1024,
		AllowedContentTypes: []string{
			"image/jpeg", "image/png", "image/gif", "image/webp",
			"application/pdf",
			"text/plain", "text/csv",
		},
		UploadGrantTTL:     15 * time.Minute,
		DownloadGrantTTL:   5 * time.Minute,
		MaxObjectKeyLength: 1024,
	}
}

type FileStore struct {
	pool *pgxpool.Pool
	cfg  Config
}

func NewFileStore(pool *pgxpool.Pool, cfg Config) *FileStore {
	return &FileStore{pool: pool, cfg: cfg}
}

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS file_objects (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			object_key TEXT NOT NULL UNIQUE,
			original_name TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT '',
			size_bytes BIGINT NOT NULL DEFAULT 0,
			sha256_hash TEXT NOT NULL DEFAULT '',
			scan_status TEXT NOT NULL DEFAULT 'unscanned',
			retention_policy TEXT NOT NULL DEFAULT 'standard',
			retention_until TIMESTAMPTZ,
			is_original BOOLEAN NOT NULL DEFAULT true,
			metadata JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_objects table: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_file_objects_tenant
			ON file_objects(tenant_id)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_objects tenant index: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_file_objects_scan_status
			ON file_objects(scan_status)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_objects scan_status index: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_file_objects_object_key
			ON file_objects(object_key)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_objects object_key index: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS file_grants (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			object_id UUID REFERENCES file_objects(id),
			grant_type TEXT NOT NULL,
			grant_token TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			max_size_bytes BIGINT,
			allowed_content_types TEXT[],
			used_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_grants table: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_file_grants_token
			ON file_grants(grant_token)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_grants grant_token index: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_file_grants_tenant
			ON file_grants(tenant_id)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_grants tenant index: %w", err)
	}

	_, err = db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_file_grants_expires
			ON file_grants(expires_at)
	`)
	if err != nil {
		return fmt.Errorf("files: create file_grants expires index: %w", err)
	}

	return nil
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("files: generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func ComputeSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("files: compute sha256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ValidateContentType(contentType string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	for _, a := range allowed {
		if strings.EqualFold(a, contentType) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrContentTypeDenied, contentType)
}

func ValidateSize(size int64, max int64) error {
	if max > 0 && size > max {
		return fmt.Errorf("%w: %d bytes exceeds limit of %d bytes", ErrSizeLimitExceeded, size, max)
	}
	return nil
}

func isValidScanTransition(from, to ScanStatus) bool {
	if from == to {
		return true
	}
	allowed, ok := validScanTransitions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

func tenantObjectKey(tenantID, key string) string {
	return fmt.Sprintf("%s/%s", tenantID, key)
}

type FileObjectRow struct {
	ID              string
	TenantID        string
	ObjectKey       string
	OriginalName    string
	ContentType     string
	SizeBytes       int64
	SHA256Hash      string
	ScanStatus      string
	RetentionPolicy string
	RetentionUntil  *time.Time
	IsOriginal      bool
	Metadata        []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

func (r *FileObjectRow) toFileObject() *FileObject {
	var m json.RawMessage
	if len(r.Metadata) > 0 {
		m = json.RawMessage(r.Metadata)
	}
	return &FileObject{
		ID:              r.ID,
		TenantID:        r.TenantID,
		ObjectKey:       r.ObjectKey,
		OriginalName:    r.OriginalName,
		ContentType:     r.ContentType,
		SizeBytes:       r.SizeBytes,
		SHA256Hash:      r.SHA256Hash,
		ScanStatus:      ScanStatus(r.ScanStatus),
		RetentionPolicy: r.RetentionPolicy,
		RetentionUntil:  r.RetentionUntil,
		IsOriginal:      r.IsOriginal,
		Metadata:        m,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		DeletedAt:       r.DeletedAt,
	}
}

type GrantRow struct {
	ID                  string
	TenantID            string
	ObjectID            *string
	GrantType           string
	GrantToken          string
	ExpiresAt           time.Time
	MaxSizeBytes        *int64
	AllowedContentTypes []string
	UsedAt              *time.Time
	CreatedAt           time.Time
}

func (r *GrantRow) toGrant() *Grant {
	return &Grant{
		ID:                  r.ID,
		TenantID:            r.TenantID,
		ObjectID:            r.ObjectID,
		GrantType:           GrantType(r.GrantType),
		GrantToken:          r.GrantToken,
		ExpiresAt:           r.ExpiresAt,
		MaxSizeBytes:        r.MaxSizeBytes,
		AllowedContentTypes: r.AllowedContentTypes,
		UsedAt:              r.UsedAt,
		CreatedAt:           r.CreatedAt,
	}
}

func scanFileObjectRow(row pgx.Row) (*FileObject, error) {
	var r FileObjectRow
	err := row.Scan(
		&r.ID, &r.TenantID, &r.ObjectKey, &r.OriginalName,
		&r.ContentType, &r.SizeBytes, &r.SHA256Hash,
		&r.ScanStatus, &r.RetentionPolicy, &r.RetentionUntil,
		&r.IsOriginal, &r.Metadata, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("files: scan file object: %w", err)
	}
	return r.toFileObject(), nil
}

func scanFileObjectRows(rows pgx.Rows) ([]FileObject, error) {
	defer rows.Close()
	var objects []FileObject
	for rows.Next() {
		var r FileObjectRow
		err := rows.Scan(
			&r.ID, &r.TenantID, &r.ObjectKey, &r.OriginalName,
			&r.ContentType, &r.SizeBytes, &r.SHA256Hash,
			&r.ScanStatus, &r.RetentionPolicy, &r.RetentionUntil,
			&r.IsOriginal, &r.Metadata, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("files: scan file object rows: %w", err)
		}
		objects = append(objects, *r.toFileObject())
	}
	return objects, rows.Err()
}

func scanGrantRow(row pgx.Row) (*Grant, error) {
	var r GrantRow
	err := row.Scan(
		&r.ID, &r.TenantID, &r.ObjectID, &r.GrantType,
		&r.GrantToken, &r.ExpiresAt, &r.MaxSizeBytes,
		&r.AllowedContentTypes, &r.UsedAt, &r.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidGrantToken
		}
		return nil, fmt.Errorf("files: scan grant: %w", err)
	}
	return r.toGrant(), nil
}
