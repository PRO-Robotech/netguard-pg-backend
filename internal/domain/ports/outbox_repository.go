package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
	"netguard-pg-backend/internal/domain"
)

// OutboxRepository defines the port for outbox persistence
// Implements Clean Architecture Ports & Adapters pattern
type OutboxRepository interface {
	// Create inserts or updates an outbox entry (UPSERT on unique constraint)
	// Uses ON CONFLICT (resource_type, resource_id, operation, target_system)
	// DO UPDATE SET payload = EXCLUDED.payload, delta = EXCLUDED.delta, ...
	//
	// This ensures idempotency: if the same resource+operation+target is inserted twice,
	// the second insert will update the existing entry with new payload/metadata.
	//
	// Example usage:
	//   entry := &domain.OutboxEntry{
	//     ResourceType: "Host",
	//     ResourceID:   hostID,
	//     Operation:    domain.SyncOperationCreate,
	//     TargetSystem: domain.TargetSystemSGROUP,
	//     Payload:      hostJSON,
	//     Status:       domain.OutboxStatusPending,
	//     MaxRetries:   5,
	//   }
	//   err := repo.Create(ctx, entry)
	Create(ctx context.Context, entry *domain.OutboxEntry) error

	// FindByID returns an outbox entry by its ID
	// Returns error if entry is not found
	FindByID(ctx context.Context, id uuid.UUID) (*domain.OutboxEntry, error)

	// FindByResource returns all outbox entries for a specific resource
	// Ordered by created_at DESC (newest first)
	//
	// Useful for:
	// - Checking sync status of a resource
	// - Finding pending syncs before resource deletion
	// - Audit trail of sync operations for a resource
	FindByResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*domain.OutboxEntry, error)

	// FindPending returns pending/retryable entries ready to process
	// Uses FOR UPDATE SKIP LOCKED for multi-worker concurrency
	//
	// Filters:
	//   - status IN ('PENDING', 'FAILED_RETRYABLE')
	//   - next_retry_at IS NULL OR next_retry_at <= NOW()
	//
	// Ordering: created_at ASC (oldest first, FIFO processing)
	//
	// FOR UPDATE SKIP LOCKED guarantees:
	//   - Each worker gets exclusive lock on selected rows
	//   - Already locked rows are skipped (no wait, no deadlock)
	//   - Multiple OutboxWorker processes can run safely in parallel
	//
	// Example: 3 workers polling every 5 seconds, each gets different entries
	//
	// This is the MOST CRITICAL method for OutboxWorker!
	FindPending(ctx context.Context, limit int) ([]*domain.OutboxEntry, error)

	// CountPending returns the number of pending/retryable entries ready to process
	// Used for metrics and health checks
	//
	// Filters same as FindPending:
	//   - status IN ('PENDING', 'FAILED_RETRYABLE')
	//   - next_retry_at IS NULL OR next_retry_at <= NOW()
	//
	// Returns: count of entries ready to process
	CountPending(ctx context.Context) (int, error)

	// UpdateStatus updates entry status and metadata
	// Used after processing attempt to record success/failure
	//
	// Example after successful sync:
	//   now := time.Now()
	//   repo.UpdateStatus(ctx, entryID, domain.OutboxStatusSuccess, &now, nil, nil)
	//
	// Example after failure:
	//   errorMsg := "timeout contacting SGROUP"
	//   errorCat := "NETWORK"
	//   repo.UpdateStatus(ctx, entryID, domain.OutboxStatusFailedRetryable, nil, &errorMsg, &errorCat)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OutboxStatus,
		processedAt *time.Time, lastError *string, errorCategory *string) error

	// IncrementAttempts increments attempts counter and sets next retry time
	// Used when entry fails and needs to be retried later
	//
	// Example with exponential backoff:
	//   nextRetry := time.Now().Add(time.Duration(math.Pow(2, attempts)) * time.Second)
	//   repo.IncrementAttempts(ctx, entryID, &nextRetry)
	IncrementAttempts(ctx context.Context, id uuid.UUID, nextRetryAt *time.Time) error

	// Delete removes an outbox entry
	// Used for cleanup of SUCCESS entries after retention period
	//
	// Note: Most SUCCESS entries should be kept for audit trail,
	// only delete after configured retention period (e.g., 7 days)
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteOldSuccessful deletes SUCCESS entries older than specified duration
	// Used by cleanup job to prevent unbounded table growth
	//
	// Example: delete SUCCESS entries older than 7 days
	//   deleted, err := repo.DeleteOldSuccessful(ctx, 7 * 24 * time.Hour)
	//
	// Returns: number of deleted entries
	DeleteOldSuccessful(ctx context.Context, olderThan time.Duration) (int64, error)
}
