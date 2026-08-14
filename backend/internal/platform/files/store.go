package files

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (s *FileStore) CreateObject(
	ctx context.Context,
	tenantID, objectKey, originalName, contentType string,
	sizeBytes int64, sha256Hash string,
	metadata json.RawMessage,
) (*FileObject, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	if err := ValidateContentType(contentType, s.cfg.AllowedContentTypes); err != nil {
		return nil, err
	}
	if err := ValidateSize(sizeBytes, s.cfg.MaxUploadSizeBytes); err != nil {
		return nil, err
	}
	if len(objectKey) > s.cfg.MaxObjectKeyLength {
		return nil, fmt.Errorf("object key exceeds max length of %d", s.cfg.MaxObjectKeyLength)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO file_objects (
			tenant_id, object_key, original_name, content_type,
			size_bytes, sha256_hash, scan_status, retention_policy,
			is_original, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, tenant_id, object_key, original_name, content_type,
			size_bytes, sha256_hash, scan_status, retention_policy,
			retention_until, is_original, metadata, created_at, updated_at, deleted_at
	`,
		tenantID, objectKey, originalName, contentType,
		sizeBytes, sha256Hash, string(ScanStatusUnscanned), "standard",
		true, metadata,
	)
	return scanFileObjectRow(row)
}

func (s *FileStore) GetObject(ctx context.Context, tenantID, objectID string) (*FileObject, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, object_key, original_name, content_type,
			size_bytes, sha256_hash, scan_status, retention_policy,
			retention_until, is_original, metadata, created_at, updated_at, deleted_at
		FROM file_objects
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, objectID, tenantID)
	return scanFileObjectRow(row)
}

func (s *FileStore) GetObjectByKey(ctx context.Context, objectKey string) (*FileObject, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, object_key, original_name, content_type,
			size_bytes, sha256_hash, scan_status, retention_policy,
			retention_until, is_original, metadata, created_at, updated_at, deleted_at
		FROM file_objects
		WHERE object_key = $1 AND deleted_at IS NULL
	`, objectKey)
	return scanFileObjectRow(row)
}

func (s *FileStore) GetAvailableObject(ctx context.Context, tenantID, objectID string) (*FileObject, error) {
	obj, err := s.GetObject(ctx, tenantID, objectID)
	if err != nil {
		return nil, err
	}

	if !downloadableScanStatuses[obj.ScanStatus] {
		return nil, fmt.Errorf("%w: scan status is %s", ErrUnscannedObject, obj.ScanStatus)
	}

	return obj, nil
}

func (s *FileStore) UpdateScanStatus(ctx context.Context, tenantID, objectID string, newStatus ScanStatus) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	existing, err := s.GetObject(ctx, tenantID, objectID)
	if err != nil {
		return err
	}

	if !isValidScanTransition(existing.ScanStatus, newStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrScanStatusConflict, existing.ScanStatus, newStatus)
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE file_objects
		SET scan_status = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(newStatus), objectID, tenantID)
	if err != nil {
		return fmt.Errorf("files: update scan status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *FileStore) SoftDeleteObject(ctx context.Context, tenantID, objectID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE file_objects
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, objectID, tenantID)
	if err != nil {
		return fmt.Errorf("files: soft delete object: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *FileStore) ListObjects(ctx context.Context, tenantID string, limit, offset int) ([]FileObject, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, object_key, original_name, content_type,
			size_bytes, sha256_hash, scan_status, retention_policy,
			retention_until, is_original, metadata, created_at, updated_at, deleted_at
		FROM file_objects
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("files: list objects: %w", err)
	}
	return scanFileObjectRows(rows)
}

func (s *FileStore) CreateUploadGrant(
	ctx context.Context,
	tenantID string,
	maxSizeBytes *int64,
	allowedContentTypes []string,
) (*Grant, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(s.cfg.UploadGrantTTL)

	row := s.pool.QueryRow(ctx, `
		INSERT INTO file_grants (
			tenant_id, grant_type, grant_token, expires_at,
			max_size_bytes, allowed_content_types
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, object_id, grant_type, grant_token,
			expires_at, max_size_bytes, allowed_content_types, used_at, created_at
	`,
		tenantID, string(GrantTypeUpload), token, expiresAt,
		maxSizeBytes, allowedContentTypes,
	)
	return scanGrantRow(row)
}

func (s *FileStore) CreateDownloadGrant(ctx context.Context, tenantID, objectID string) (*Grant, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	obj, err := s.GetObject(ctx, tenantID, objectID)
	if err != nil {
		return nil, fmt.Errorf("cannot create download grant: %w", err)
	}

	if !downloadableScanStatuses[obj.ScanStatus] {
		return nil, fmt.Errorf("%w: object %s has scan status %s", ErrUnscannedObject, objectID, obj.ScanStatus)
	}

	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(s.cfg.DownloadGrantTTL)

	row := s.pool.QueryRow(ctx, `
		INSERT INTO file_grants (
			tenant_id, object_id, grant_type, grant_token, expires_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, object_id, grant_type, grant_token,
			expires_at, max_size_bytes, allowed_content_types, used_at, created_at
	`,
		tenantID, objectID, string(GrantTypeDownload), token, expiresAt,
	)
	return scanGrantRow(row)
}

func (s *FileStore) ValidateUploadGrant(ctx context.Context, token string, sizeBytes int64, contentType string) (*Grant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("files: validate upload grant begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT id, tenant_id, object_id, grant_type, grant_token,
			expires_at, max_size_bytes, allowed_content_types, used_at, created_at
		FROM file_grants
		WHERE grant_token = $1
		FOR UPDATE
	`, token)
	grant, err := scanGrantRow(row)
	if err != nil {
		return nil, err
	}

	if grant.UsedAt != nil {
		return nil, ErrGrantUsed
	}

	if grant.GrantType != GrantTypeUpload {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrGrantInvalidType, GrantTypeUpload, grant.GrantType)
	}

	if time.Now().UTC().After(grant.ExpiresAt) {
		return nil, ErrGrantExpired
	}

	if grant.MaxSizeBytes != nil {
		if err := ValidateSize(sizeBytes, *grant.MaxSizeBytes); err != nil {
			return nil, err
		}
	}

	if len(grant.AllowedContentTypes) > 0 {
		if err := ValidateContentType(contentType, grant.AllowedContentTypes); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE file_grants SET used_at = $1 WHERE id = $2 AND used_at IS NULL
	`, now, grant.ID)
	if err != nil {
		return nil, fmt.Errorf("files: consume upload grant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrGrantUsed
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("files: validate upload grant commit: %w", err)
	}

	grant.UsedAt = &now
	return grant, nil
}

func (s *FileStore) ValidateDownloadGrant(ctx context.Context, tenantID, token string) (*Grant, *FileObject, error) {
	if tenantID == "" {
		return nil, nil, ErrTenantRequired
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("files: validate download grant begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT id, tenant_id, object_id, grant_type, grant_token,
			expires_at, max_size_bytes, allowed_content_types, used_at, created_at
		FROM file_grants
		WHERE grant_token = $1
		FOR UPDATE
	`, token)
	grant, err := scanGrantRow(row)
	if err != nil {
		return nil, nil, err
	}

	if grant.UsedAt != nil {
		return nil, nil, ErrGrantUsed
	}

	if grant.GrantType != GrantTypeDownload {
		return nil, nil, fmt.Errorf("%w: expected %s, got %s", ErrGrantInvalidType, GrantTypeDownload, grant.GrantType)
	}

	if grant.TenantID != tenantID {
		return nil, nil, ErrNotFound
	}

	if time.Now().UTC().After(grant.ExpiresAt) {
		return nil, nil, ErrGrantExpired
	}

	if grant.ObjectID == nil {
		return nil, nil, fmt.Errorf("download grant has no associated object")
	}

	objRow := tx.QueryRow(ctx, `
		SELECT id, tenant_id, object_key, original_name, content_type,
			size_bytes, sha256_hash, scan_status, retention_policy,
			retention_until, is_original, metadata, created_at, updated_at, deleted_at
		FROM file_objects
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, *grant.ObjectID, tenantID)
	obj, err := scanFileObjectRow(objRow)
	if err != nil {
		return nil, nil, err
	}

	if !downloadableScanStatuses[obj.ScanStatus] {
		return nil, nil, fmt.Errorf("%w: object %s has scan status %s", ErrUnscannedObject, obj.ID, obj.ScanStatus)
	}

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE file_grants SET used_at = $1 WHERE id = $2 AND used_at IS NULL
	`, now, grant.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("files: consume download grant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, nil, ErrGrantUsed
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("files: validate download grant commit: %w", err)
	}

	grant.UsedAt = &now
	return grant, obj, nil
}

func (s *FileStore) CleanupExpiredGrants(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM file_grants WHERE expires_at < NOW() AND used_at IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("files: cleanup expired grants: %w", err)
	}
	return tag.RowsAffected(), nil
}
