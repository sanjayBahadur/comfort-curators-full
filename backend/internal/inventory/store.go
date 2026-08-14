package inventory

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

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const locationColumns = `id, tenant_id, property_id, name, location_type, version, created_at, updated_at`

func scanLocation(row pgx.Row) (*StockLocation, error) {
	var loc StockLocation
	err := row.Scan(
		&loc.ID, &loc.TenantID, &loc.PropertyID, &loc.Name,
		&loc.LocationType, &loc.Version, &loc.CreatedAt, &loc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLocationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &loc, nil
}

func (s *Store) insertLocation(ctx context.Context, q querier, loc *StockLocation) error {
	loc.ID = newID("loc")
	_, err := q.Exec(ctx, `
		INSERT INTO stock_locations (id, tenant_id, property_id, name, location_type)
		VALUES ($1, $2, $3, $4, $5)
	`, loc.ID, loc.TenantID, loc.PropertyID, loc.Name, loc.LocationType)
	return err
}

func (s *Store) InsertLocation(ctx context.Context, loc *StockLocation) error {
	return s.insertLocation(ctx, s.pool, loc)
}

func (s *Store) GetLocation(ctx context.Context, tenantID, locationID string) (*StockLocation, error) {
	return scanLocation(s.pool.QueryRow(ctx, `
		SELECT `+locationColumns+`
		FROM stock_locations
		WHERE id = $1 AND tenant_id = $2
	`, locationID, tenantID))
}

func (s *Store) ListLocations(ctx context.Context, tenantID string) ([]StockLocation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+locationColumns+`
		FROM stock_locations
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StockLocation
	for rows.Next() {
		loc, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *loc)
	}
	return out, rows.Err()
}

func (s *Store) LockLocation(ctx context.Context, q querier, tenantID, locationID string) (*StockLocation, error) {
	return scanLocation(q.QueryRow(ctx, `
		SELECT `+locationColumns+`
		FROM stock_locations
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE
	`, locationID, tenantID))
}

const movementColumns = `id, tenant_id, location_id, catalog_item_id, movement_type,
	quantity, reference_type, reference_id, reason, actor_id, expires_at, created_at`

func scanMovement(row pgx.Row) (*InventoryMovement, error) {
	var m InventoryMovement
	var expiresAt *time.Time
	err := row.Scan(
		&m.ID, &m.TenantID, &m.LocationID, &m.CatalogItemID,
		&m.MovementType, &m.Quantity, &m.ReferenceType, &m.ReferenceID,
		&m.Reason, &m.ActorID, &expiresAt, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMovementNotFound
	}
	if err != nil {
		return nil, err
	}
	if expiresAt != nil {
		m.ExpiresAt = expiresAt
	}
	return &m, nil
}

func (s *Store) InsertMovement(ctx context.Context, q querier, m *InventoryMovement) error {
	m.ID = newID("mov")
	_, err := q.Exec(ctx, `
		INSERT INTO inventory_movements (
			id, tenant_id, location_id, catalog_item_id, movement_type,
			quantity, reference_type, reference_id, reason, actor_id, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, m.ID, m.TenantID, m.LocationID, m.CatalogItemID, m.MovementType,
		m.Quantity, m.ReferenceType, m.ReferenceID, m.Reason, m.ActorID,
		nullTime(m.ExpiresAt))
	return err
}

func (s *Store) ListMovements(ctx context.Context, tenantID, locationID, catalogItemID string) ([]InventoryMovement, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+movementColumns+`
		FROM inventory_movements
		WHERE tenant_id = $1 AND location_id = $2 AND catalog_item_id = $3
		ORDER BY created_at ASC, id ASC
	`, tenantID, locationID, catalogItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InventoryMovement
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Store) GetBalance(ctx context.Context, q querier, tenantID, locationID, catalogItemID string) (int64, error) {
	var balance int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(quantity), 0)
		FROM inventory_movements
		WHERE tenant_id = $1 AND location_id = $2 AND catalog_item_id = $3
	`, tenantID, locationID, catalogItemID).Scan(&balance)
	return balance, err
}

func (s *Store) GetEffectiveBalance(ctx context.Context, q querier, tenantID, locationID, catalogItemID string) (int64, error) {
	var balance int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(quantity), 0)
		FROM inventory_movements
		WHERE tenant_id = $1 AND location_id = $2 AND catalog_item_id = $3
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, tenantID, locationID, catalogItemID).Scan(&balance)
	return balance, err
}

func (s *Store) ListAllMovements(ctx context.Context, tenantID, locationID string) ([]InventoryMovement, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+movementColumns+`
		FROM inventory_movements
		WHERE tenant_id = $1 AND location_id = $2
		ORDER BY created_at ASC, id ASC
	`, tenantID, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InventoryMovement
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Store) CountMovements(ctx context.Context, tenantID, locationID, catalogItemID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM inventory_movements
		WHERE tenant_id = $1 AND location_id = $2 AND catalog_item_id = $3
	`, tenantID, locationID, catalogItemID).Scan(&count)
	return count, err
}

const countColumns = `id, tenant_id, location_id, status, counted_by, reviewed_by, version, created_at, updated_at`

func scanCount(row pgx.Row) (*InventoryCount, error) {
	var c InventoryCount
	err := row.Scan(
		&c.ID, &c.TenantID, &c.LocationID, &c.Status,
		&c.CountedBy, &c.ReviewedBy, &c.Version, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) InsertCount(ctx context.Context, q querier, c *InventoryCount) error {
	c.ID = newID("cnt")
	_, err := q.Exec(ctx, `
		INSERT INTO inventory_counts (id, tenant_id, location_id, status, counted_by)
		VALUES ($1, $2, $3, $4, $5)
	`, c.ID, c.TenantID, c.LocationID, c.Status, c.CountedBy)
	return err
}

func (s *Store) GetCount(ctx context.Context, tenantID, countID string) (*InventoryCount, error) {
	return scanCount(s.pool.QueryRow(ctx, `
		SELECT `+countColumns+`
		FROM inventory_counts
		WHERE id = $1 AND tenant_id = $2
	`, countID, tenantID))
}

func (s *Store) UpdateCountStatus(ctx context.Context, q querier, tenantID, countID, status, reviewedBy string) (*InventoryCount, error) {
	return scanCount(q.QueryRow(ctx, `
		UPDATE inventory_counts
		SET status = $3, reviewed_by = $4, updated_at = NOW(), version = version + 1
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+countColumns+`
	`, countID, tenantID, status, reviewedBy))
}

const countLineColumns = `id, tenant_id, count_id, catalog_item_id, expected_quantity, counted_quantity, variance, created_at`

func scanCountLine(row pgx.Row) (*InventoryCountLine, error) {
	var line InventoryCountLine
	err := row.Scan(
		&line.ID, &line.TenantID, &line.CountID, &line.CatalogItemID,
		&line.ExpectedQuantity, &line.CountedQuantity, &line.Variance,
		&line.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &line, nil
}

func (s *Store) UpsertCountLine(ctx context.Context, q querier, line *InventoryCountLine) error {
	var existingID string
	err := q.QueryRow(ctx, `
		SELECT id FROM inventory_count_lines
		WHERE tenant_id = $1 AND count_id = $2 AND catalog_item_id = $3
	`, line.TenantID, line.CountID, line.CatalogItemID).Scan(&existingID)
	if errors.Is(err, pgx.ErrNoRows) {
		line.ID = newID("cnl")
		_, err = q.Exec(ctx, `
			INSERT INTO inventory_count_lines (id, tenant_id, count_id, catalog_item_id, expected_quantity, counted_quantity, variance)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, line.ID, line.TenantID, line.CountID, line.CatalogItemID,
			line.ExpectedQuantity, line.CountedQuantity, line.Variance)
		return err
	}
	if err != nil {
		return err
	}
	line.ID = existingID
	_, err = q.Exec(ctx, `
		UPDATE inventory_count_lines
		SET counted_quantity = $4, variance = $5
		WHERE id = $1 AND tenant_id = $2 AND count_id = $3
	`, line.ID, line.TenantID, line.CountID, line.CountedQuantity, line.Variance)
	return err
}

func (s *Store) ListCountLines(ctx context.Context, tenantID, countID string) ([]InventoryCountLine, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+countLineColumns+`
		FROM inventory_count_lines
		WHERE tenant_id = $1 AND count_id = $2
		ORDER BY created_at ASC
	`, tenantID, countID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InventoryCountLine
	for rows.Next() {
		line, err := scanCountLine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *line)
	}
	return out, rows.Err()
}

func nullTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
