package readers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"netguard-pg-backend/internal/domain/ports"
)

// Reader implements the PostgreSQL reader interface
// This is the base struct that contains all reader methods split across multiple files
type Reader struct {
	registry ports.Registry // Keep reference to registry for potential use
	pool     *pgxpool.Pool  // Connection pool for read-only operations
	tx       pgx.Tx         // Optional transaction for consistency with writer
	ctx      context.Context
}

// NewReader creates a new PostgreSQL reader instance
func NewReader(registry ports.Registry, pool *pgxpool.Pool, tx pgx.Tx, ctx context.Context) *Reader {
	return &Reader{
		registry: registry,
		pool:     pool,
		tx:       tx,
		ctx:      ctx,
	}
}

// Close closes the reader (connection returned to pool automatically)
func (r *Reader) Close() error {
	return nil
}

// query executes a query using either transaction or pool connection
// This is a shared utility method used by all resource-specific readers
func (r *Reader) query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	if r.tx != nil {
		return r.tx.Query(ctx, query, args...)
	}
	return r.pool.Query(ctx, query, args...)
}

// queryRow executes a single-row query using either transaction or pool connection
// This is a shared utility method used by all resource-specific readers
func (r *Reader) queryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	if r.tx != nil {
		return r.tx.QueryRow(ctx, query, args...)
	}
	return r.pool.QueryRow(ctx, query, args...)
}
