package writers

import (
	"context"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/ports"
)

// Writer implements the PostgreSQL writer interface
// This is the base struct that contains all writer methods split across multiple files
type Writer struct {
	registry     ports.Registry // Keep reference to registry for potential use
	tx           pgx.Tx         // Transaction for write operations
	ctx          context.Context
	affectedRows *int64 // Track affected rows for sync status
	committed    bool   // Track if transaction is committed
}

// NewWriter creates a new PostgreSQL writer instance
func NewWriter(registry ports.Registry, tx pgx.Tx, ctx context.Context) *Writer {
	affectedRows := int64(0)
	return &Writer{
		registry:     registry,
		tx:           tx,
		ctx:          ctx,
		affectedRows: &affectedRows,
		committed:    false,
	}
}

// addAffectedRows atomically adds to the affected rows counter
// This is used by all resource writers to track changes for sync status
func (w *Writer) addAffectedRows(count int64) {
	atomic.AddInt64(w.affectedRows, count)
}

// getAffectedRows returns the current affected rows count
func (w *Writer) getAffectedRows() int64 {
	return atomic.LoadInt64(w.affectedRows)
}

// exec executes a statement and tracks affected rows
// This is a shared utility method used by all resource-specific writers
func (w *Writer) exec(ctx context.Context, query string, args ...interface{}) error {
	result, err := w.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	w.addAffectedRows(result.RowsAffected())
	return nil
}

// Commit commits the transaction and updates sync status
func (w *Writer) Commit() error {
	if w.committed {
		return errors.New("transaction already committed")
	}

	// Update sync status if there were changes
	affectedRowsCount := w.getAffectedRows()
	if affectedRowsCount > 0 {
		// Update sync status table
		syncStatusQuery := `
			INSERT INTO sync_status (updated_at, total_operations)
			VALUES (NOW(), $1)
			ON CONFLICT (id) DO UPDATE
			SET updated_at = NOW(), total_operations = total_operations + $1`

		if _, err := w.tx.Exec(w.ctx, syncStatusQuery, affectedRowsCount); err != nil {
			w.tx.Rollback(w.ctx)
			return errors.Wrap(err, "failed to update sync status")
		}
	}

	// Commit the transaction
	if err := w.tx.Commit(w.ctx); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	w.committed = true
	return nil
}

// Abort rolls back the transaction
func (w *Writer) Abort() {
	if !w.committed {
		w.tx.Rollback(w.ctx)
	}
}

// GetTx returns the underlying transaction (used by ReaderFromWriter)
func (w *Writer) GetTx() pgx.Tx {
	return w.tx
}
