package repositories

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/ports"
	"time"
)

type PgxExecutor interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

var _ PgxExecutor = (*pgxpool.Pool)(nil)

type PgxOutboxRepository struct {
	executor PgxExecutor
}

func NewOutboxRepository(executor PgxExecutor) ports.OutboxRepository {
	return &PgxOutboxRepository{executor: executor}
}
func (r *PgxOutboxRepository) Create(ctx context.Context, entry *domain.OutboxEntry) error {
	query := `
		INSERT INTO sync_outbox (
			id, resource_type, resource_id, resource_namespace, resource_name,
			operation, target_system,
			payload, delta, affects_resources,
			status, attempts, max_retries, next_retry_at,
			last_error, error_category,
			created_at, updated_at, processed_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7,
			$8, $9, $10,
			$11, $12, $13, $14,
			$15, $16,
			$17, $18, $19
		)
		ON CONFLICT (resource_type, resource_id, operation, target_system)
		DO UPDATE SET
			resource_namespace = EXCLUDED.resource_namespace,
			resource_name = EXCLUDED.resource_name,
			payload = EXCLUDED.payload,
			delta = EXCLUDED.delta,
			affects_resources = EXCLUDED.affects_resources,
			status = EXCLUDED.status,
			attempts = EXCLUDED.attempts,
			max_retries = EXCLUDED.max_retries,
			next_retry_at = EXCLUDED.next_retry_at,
			last_error = EXCLUDED.last_error,
			error_category = EXCLUDED.error_category,
			updated_at = EXCLUDED.updated_at,
			processed_at = EXCLUDED.processed_at
		RETURNING id
	`
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}
	err := r.executor.QueryRow(ctx, query,
		entry.ID, entry.ResourceType, entry.ResourceID, entry.ResourceNamespace, entry.ResourceName,
		entry.Operation, entry.TargetSystem,
		entry.Payload, entry.Delta, entry.AffectsResources,
		entry.Status, entry.Attempts, entry.MaxRetries, entry.NextRetryAt,
		entry.LastError, entry.ErrorCategory,
		entry.CreatedAt, entry.UpdatedAt, entry.ProcessedAt,
	).Scan(&entry.ID)
	if err != nil {
		return fmt.Errorf("failed to create/update outbox entry: %w", err)
	}
	return nil
}
func (r *PgxOutboxRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.OutboxEntry, error) {
	query := `
		SELECT
			id, resource_type, resource_id, resource_namespace, resource_name,
			operation, target_system,
			payload, delta, affects_resources,
			status, attempts, max_retries, next_retry_at,
			last_error, error_category,
			created_at, updated_at, processed_at
		FROM sync_outbox
		WHERE id = $1
	`
	entry := &domain.OutboxEntry{}
	err := r.executor.QueryRow(ctx, query, id).Scan(
		&entry.ID, &entry.ResourceType, &entry.ResourceID, &entry.ResourceNamespace, &entry.ResourceName,
		&entry.Operation, &entry.TargetSystem,
		&entry.Payload, &entry.Delta, &entry.AffectsResources,
		&entry.Status, &entry.Attempts, &entry.MaxRetries, &entry.NextRetryAt,
		&entry.LastError, &entry.ErrorCategory,
		&entry.CreatedAt, &entry.UpdatedAt, &entry.ProcessedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("outbox entry not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find outbox entry: %w", err)
	}
	return entry, nil
}
func (r *PgxOutboxRepository) FindByResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*domain.OutboxEntry, error) {
	query := `
		SELECT
			id, resource_type, resource_id, resource_namespace, resource_name,
			operation, target_system,
			payload, delta, affects_resources,
			status, attempts, max_retries, next_retry_at,
			last_error, error_category,
			created_at, updated_at, processed_at
		FROM sync_outbox
		WHERE resource_type = $1 AND resource_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.executor.Query(ctx, query, resourceType, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entries for resource: %w", err)
	}
	defer rows.Close()
	var entries []*domain.OutboxEntry
	for rows.Next() {
		entry := &domain.OutboxEntry{}
		err := rows.Scan(
			&entry.ID, &entry.ResourceType, &entry.ResourceID, &entry.ResourceNamespace, &entry.ResourceName,
			&entry.Operation, &entry.TargetSystem,
			&entry.Payload, &entry.Delta, &entry.AffectsResources,
			&entry.Status, &entry.Attempts, &entry.MaxRetries, &entry.NextRetryAt,
			&entry.LastError, &entry.ErrorCategory,
			&entry.CreatedAt, &entry.UpdatedAt, &entry.ProcessedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating entries: %w", rows.Err())
	}
	return entries, nil
}
func (r *PgxOutboxRepository) FindPending(ctx context.Context, limit int) ([]*domain.OutboxEntry, error) {
	query := `
		SELECT
			id, resource_type, resource_id, resource_namespace, resource_name,
			operation, target_system,
			payload, delta, affects_resources,
			status, attempts, max_retries, next_retry_at,
			last_error, error_category,
			created_at, updated_at, processed_at
		FROM sync_outbox
		WHERE status IN ('PENDING', 'FAILED_RETRYABLE')
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`
	rows, err := r.executor.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending entries: %w", err)
	}
	defer rows.Close()
	var entries []*domain.OutboxEntry
	for rows.Next() {
		entry := &domain.OutboxEntry{}
		err := rows.Scan(
			&entry.ID, &entry.ResourceType, &entry.ResourceID, &entry.ResourceNamespace, &entry.ResourceName,
			&entry.Operation, &entry.TargetSystem,
			&entry.Payload, &entry.Delta, &entry.AffectsResources,
			&entry.Status, &entry.Attempts, &entry.MaxRetries, &entry.NextRetryAt,
			&entry.LastError, &entry.ErrorCategory,
			&entry.CreatedAt, &entry.UpdatedAt, &entry.ProcessedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pending entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating pending entries: %w", rows.Err())
	}
	return entries, nil
}
func (r *PgxOutboxRepository) CountPending(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE status IN ('PENDING', 'FAILED_RETRYABLE')
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
	`
	var count int
	err := r.executor.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending entries: %w", err)
	}
	return count, nil
}
func (r *PgxOutboxRepository) CountPendingForResource(ctx context.Context, resourceType, namespace, name string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE resource_type = $1
		  AND resource_namespace = $2
		  AND resource_name = $3
		  AND status IN ('PENDING', 'FAILED_RETRYABLE')
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
	`
	var count int
	err := r.executor.QueryRow(ctx, query, resourceType, namespace, name).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending entries for resource: %w", err)
	}
	return count, nil
}
func (r *PgxOutboxRepository) UpdateStatus(
	ctx context.Context,
	id uuid.UUID,
	status domain.OutboxStatus,
	processedAt *time.Time,
	lastError *string,
	errorCategory *string,
) error {
	query := `
		UPDATE sync_outbox
		SET
			status = $2,
			processed_at = $3,
			last_error = $4,
			error_category = $5,
			updated_at = NOW()
		WHERE id = $1
	`
	result, err := r.executor.Exec(ctx, query, id, status, processedAt, lastError, errorCategory)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox entry not found: %s", id)
	}
	return nil
}
func (r *PgxOutboxRepository) IncrementAttempts(ctx context.Context, id uuid.UUID, nextRetryAt *time.Time) error {
	query := `
		UPDATE sync_outbox
		SET
			attempts = attempts + 1,
			next_retry_at = $2,
			updated_at = NOW()
		WHERE id = $1
	`
	result, err := r.executor.Exec(ctx, query, id, nextRetryAt)
	if err != nil {
		return fmt.Errorf("failed to increment attempts: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox entry not found: %s", id)
	}
	return nil
}
func (r *PgxOutboxRepository) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE sync_outbox
		SET status = 'SUCCESS', processed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	result, err := r.executor.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark outbox entry as completed: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox entry not found: %s", id)
	}
	return nil
}
func (r *PgxOutboxRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM sync_outbox WHERE id = $1`
	result, err := r.executor.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete outbox entry: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("outbox entry not found: %s", id)
	}
	return nil
}
func (r *PgxOutboxRepository) DeleteOldSuccessful(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	query := `
		DELETE FROM sync_outbox
		WHERE status = 'SUCCESS'
		  AND processed_at < $1
	`
	result, err := r.executor.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old successful entries: %w", err)
	}
	return result.RowsAffected(), nil
}
