package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain"
)

// scheduleRetry handles retry logic with exponential backoff
// If max attempts reached, marks entry as FAILED_PERMANENT
// Otherwise, increments attempts and schedules next retry
func (w *OutboxWorker) scheduleRetry(
	ctx context.Context,
	item *domain.OutboxEntry,
	err error,
	customInterval ...time.Duration,
) error {
	// Categorize error to determine retry strategy
	category := w.categorizeError(err)
	maxAttempts := getMaxAttempts(category)

	// Log retry attempt with context
	w.logger.Info("scheduling retry",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.Int("current_attempts", item.Attempts),
		zap.Int("max_attempts", maxAttempts),
		zap.String("error_category", string(category)),
		zap.Error(err))

	// Check if max attempts reached
	if item.Attempts >= maxAttempts {
		w.logger.Warn("max retry attempts reached, marking as failed permanent",
			zap.String("resource_type", item.ResourceType),
			zap.String("resource_id", item.ResourceID.String()),
			zap.Int("attempts", item.Attempts),
			zap.Int("max_attempts", maxAttempts),
			zap.String("category", string(category)))

		return w.markFailedPermanent(ctx, item, err, category)
	}

	// P0-3: Record retry metric
	RecordRetry(item.ResourceType, string(item.Operation))

	// Calculate next retry time
	var nextRetry time.Duration
	if len(customInterval) > 0 {
		// Use custom interval (e.g., for dependency checks)
		nextRetry = customInterval[0]
	} else {
		// Use exponential backoff
		nextRetry = w.calculateBackoff(item.Attempts)
	}

	nextRetryAt := time.Now().Add(nextRetry)

	// Integration Point #3: Update PendingSync condition with retry info
	if err := w.updatePendingSyncRetrying(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName,
		item.Attempts+1, maxAttempts, nextRetry, err); err != nil {
		w.logger.Warn("failed to update PendingSync condition for retry", zap.Error(err))
	}

	// Update Outbox entry with retry information
	errMsg := err.Error()
	categoryStr := string(category)

	updateQuery := `
		UPDATE sync_outbox
		SET attempts = attempts + 1,
		    status = $1,
		    last_error = $2,
		    error_category = $3,
		    next_retry_at = $4,
		    updated_at = NOW()
		WHERE id = $5
	`

	_, updateErr := w.pool.Exec(ctx, updateQuery,
		domain.OutboxStatusPending, // Keep as PENDING for retry
		errMsg,
		categoryStr,
		nextRetryAt,
		item.ID,
	)

	if updateErr != nil {
		return fmt.Errorf("failed to update outbox for retry: %w", updateErr)
	}

	w.logger.Info("retry scheduled",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.Int("attempt", item.Attempts+1),
		zap.Int("max_attempts", maxAttempts),
		zap.Duration("retry_in", nextRetry),
		zap.Time("next_retry_at", nextRetryAt),
		zap.String("category", string(category)))

	return nil // Retry scheduled successfully (not an error)
}

// calculateBackoff calculates exponential backoff duration
// Sequence: 10s → 30s → 90s → 270s → 600s (max)
func (w *OutboxWorker) calculateBackoff(attempts int) time.Duration {
	backoffs := []time.Duration{
		10 * time.Second,  // Attempt 1: 10 seconds
		30 * time.Second,  // Attempt 2: 30 seconds
		90 * time.Second,  // Attempt 3: 90 seconds (1.5 minutes)
		270 * time.Second, // Attempt 4: 270 seconds (4.5 minutes)
		600 * time.Second, // Attempt 5+: 600 seconds (10 minutes, max)
	}

	// attempts is 0-indexed, so use directly
	if attempts < len(backoffs) {
		return backoffs[attempts]
	}

	// Return max backoff for attempts beyond sequence
	return backoffs[len(backoffs)-1]
}

// markFailedPermanent marks an outbox entry as permanently failed
// No more retries will be attempted
func (w *OutboxWorker) markFailedPermanent(
	ctx context.Context,
	item *domain.OutboxEntry,
	err error,
	category ErrorCategory,
) error {
	// Integration Point #4: Update PendingSync condition with permanent failure
	if updateErr := w.updatePendingSyncFailedPermanent(ctx, item.ResourceType, item.ResourceNamespace, item.ResourceName,
		item.Attempts, err); updateErr != nil {
		w.logger.Warn("failed to update PendingSync condition for permanent failure", zap.Error(updateErr))
	}

	// P0-3: Record permanent failure metric (using legacy function for backward compatibility)
	recordPermanentFailure(item.ResourceType, string(category))

	errMsg := err.Error()
	categoryStr := string(category)

	updateQuery := `
		UPDATE sync_outbox
		SET status = $1,
		    last_error = $2,
		    error_category = $3,
		    processed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $4
	`

	_, updateErr := w.pool.Exec(ctx, updateQuery,
		domain.OutboxStatusFailedPermanent,
		errMsg,
		categoryStr,
		item.ID,
	)

	if updateErr != nil {
		return fmt.Errorf("failed to mark as failed permanent: %w", updateErr)
	}

	w.logger.Error("marked as failed permanent",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.Int("attempts", item.Attempts),
		zap.String("category", string(category)),
		zap.Error(err))

	return nil // No more retries (success from retry logic perspective)
}

// handleSyncSuccess handles successful sync operations
// Called by processEntityResource and processProcessResource
func (w *OutboxWorker) handleSyncSuccess(ctx context.Context, item *domain.OutboxEntry) error {
	// Delete Outbox entry on success
	if err := w.outboxRepo.Delete(ctx, item.ID); err != nil {
		return fmt.Errorf("failed to delete outbox entry: %w", err)
	}

	w.logger.Info("sync successful",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)),
		zap.Int("attempts", item.Attempts))

	return nil
}

// handleSyncFailure handles sync failures with retry logic
// Called by processEntityResource and processProcessResource
func (w *OutboxWorker) handleSyncFailure(
	ctx context.Context,
	item *domain.OutboxEntry,
	err error,
) error {
	w.logger.Error("sync failed",
		zap.String("resource_type", item.ResourceType),
		zap.String("resource_id", item.ResourceID.String()),
		zap.String("operation", string(item.Operation)),
		zap.Int("attempts", item.Attempts),
		zap.Error(err))

	// Schedule retry (or mark as failed permanent if max attempts reached)
	return w.scheduleRetry(ctx, item, err)
}
