package database

import (
	"context"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, cfg config.Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DBDSN())
	if err != nil {
		return nil, fmt.Errorf("database: parse dsn: %w", err)
	}

	poolCfg.MaxConns = 20
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := connectWithRetry(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	logging.Info(ctx, "database connection pool established",
		"host", cfg.DBHost,
		"port", cfg.DBPort,
		"dbname", cfg.DBName,
	)

	return &DB{Pool: pool}, nil
}

func connectWithRetry(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		return pool, nil
	}
	return nil, fmt.Errorf("database: connect after 30s: %w", lastErr)
}

func (db *DB) Close() {
	db.Pool.Close()
}
