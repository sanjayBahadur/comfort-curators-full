package database

import (
	"context"
	"fmt"
	"runtime/debug"

	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxFunc func(ctx context.Context, tx pgx.Tx) error

func WithTx(ctx context.Context, pool *pgxpool.Pool, fn TxFunc) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: begin tx: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logging.Error(ctx, "tx rollback after panic failed",
					"panic", fmt.Sprintf("%v", r),
					"rollback_error", rbErr,
				)
			}
			logging.Error(ctx, "transaction panicked",
				"panic", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()),
			)
			panic(r)
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				logging.Error(ctx, "tx rollback after error failed",
					"error", err,
					"rollback_error", rbErr,
				)
			}
		}
	}()

	if err := fn(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit tx: %w", err)
	}

	return nil
}
