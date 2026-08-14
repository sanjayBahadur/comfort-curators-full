package procurement

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, db *pgxpool.Pool) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS suppliers (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			contact_info TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending_approval',
			created_by TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			approved_at TIMESTAMPTZ DEFAULT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_suppliers_tenant ON suppliers(tenant_id, status)`,

		`CREATE TABLE IF NOT EXISTS supplier_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			supplier_id TEXT NOT NULL,
			catalog_item_id TEXT NOT NULL,
			supplier_sku TEXT NOT NULL DEFAULT '',
			unit_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			unit_cost_currency TEXT NOT NULL DEFAULT 'INR',
			lead_time_days INTEGER NOT NULL DEFAULT 0,
			minimum_order_quantity BIGINT NOT NULL DEFAULT 1,
			is_preferred BOOLEAN NOT NULL DEFAULT false,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_supplier_items_supplier ON supplier_items(tenant_id, supplier_id)`,
		`CREATE INDEX IF NOT EXISTS idx_supplier_items_catalog ON supplier_items(tenant_id, catalog_item_id)`,

		`CREATE TABLE IF NOT EXISTS requisitions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			property_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			created_by TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			rejected_by TEXT NOT NULL DEFAULT '',
			total_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			notes TEXT NOT NULL DEFAULT '',
			new_supplier_ids JSONB NOT NULL DEFAULT '[]',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requisitions_tenant ON requisitions(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_requisitions_property ON requisitions(tenant_id, property_id)`,

		`CREATE TABLE IF NOT EXISTS requisition_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			requisition_id TEXT NOT NULL,
			catalog_item_id TEXT NOT NULL DEFAULT '',
			supplier_item_id TEXT NOT NULL DEFAULT '',
			quantity BIGINT NOT NULL DEFAULT 0,
			unit_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			unit_cost_currency TEXT NOT NULL DEFAULT 'INR',
			line_total_minor_units BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requisition_items_req ON requisition_items(tenant_id, requisition_id)`,

		`CREATE TABLE IF NOT EXISTS requisition_approvals (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			requisition_id TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			decision TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			is_ai_actor BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requisition_approvals_req ON requisition_approvals(tenant_id, requisition_id)`,

		`CREATE TABLE IF NOT EXISTS purchase_orders (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			requisition_id TEXT NOT NULL DEFAULT '',
			supplier_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			ordered_by TEXT NOT NULL DEFAULT '',
			total_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			order_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expected_delivery TIMESTAMPTZ DEFAULT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_orders_tenant ON purchase_orders(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_orders_requisition ON purchase_orders(tenant_id, requisition_id)`,

		`CREATE TABLE IF NOT EXISTS purchase_order_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			purchase_order_id TEXT NOT NULL,
			requisition_item_id TEXT NOT NULL DEFAULT '',
			catalog_item_id TEXT NOT NULL DEFAULT '',
			quantity BIGINT NOT NULL DEFAULT 0,
			unit_cost_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			line_total_minor_units BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_order_items_po ON purchase_order_items(tenant_id, purchase_order_id)`,

		`CREATE TABLE IF NOT EXISTS goods_receipts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			purchase_order_id TEXT NOT NULL DEFAULT '',
			received_by TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			condition TEXT NOT NULL DEFAULT 'good',
			condition_notes TEXT NOT NULL DEFAULT '',
			evidence_ref TEXT NOT NULL DEFAULT '',
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_goods_receipts_po ON goods_receipts(tenant_id, purchase_order_id)`,

		`CREATE TABLE IF NOT EXISTS goods_receipt_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			goods_receipt_id TEXT NOT NULL,
			purchase_order_item_id TEXT NOT NULL DEFAULT '',
			catalog_item_id TEXT NOT NULL DEFAULT '',
			quantity_ordered BIGINT NOT NULL DEFAULT 0,
			quantity_received BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_goods_receipt_items_gr ON goods_receipt_items(tenant_id, goods_receipt_id)`,

		`CREATE TABLE IF NOT EXISTS supplier_rebates (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			supplier_id TEXT NOT NULL DEFAULT '',
			purchase_order_id TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			amount_minor_units BIGINT NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'INR',
			status TEXT NOT NULL DEFAULT 'offered',
			offered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			settled_at TIMESTAMPTZ DEFAULT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_supplier_rebates_supplier ON supplier_rebates(tenant_id, supplier_id)`,
		`CREATE INDEX IF NOT EXISTS idx_supplier_rebates_po ON supplier_rebates(tenant_id, purchase_order_id)`,
	}

	for _, stmt := range ddl {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("procurement schema: %w", err)
		}
	}

	return nil
}
