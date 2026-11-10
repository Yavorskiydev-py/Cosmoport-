// Package postgres — слой repository: вся работа с PostgreSQL изолирована здесь.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool создаёт пул соединений и ждёт, пока БД станет доступна.
func NewPool(ctx context.Context, dsn string, maxConns int32, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	const maxAttempts = 30
	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			log.Info("connected to postgres", "attempt", attempt)
			return pool, nil
		}
		if attempt == maxAttempts {
			pool.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		log.Warn("postgres is not ready, retrying", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
