package readers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// GetSyncStatus gets the sync status (singleton pattern)
func (r *Reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	query := `SELECT updated_at FROM sync_status WHERE id = 1`

	row := r.queryRow(ctx, query)

	var syncStatus models.SyncStatus
	err := row.Scan(&syncStatus.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ports.ErrNotFound
		}
		return nil, errors.Wrap(err, "failed to scan sync status")
	}

	return &syncStatus, nil
}
