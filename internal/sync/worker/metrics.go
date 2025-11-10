package worker

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var (
	// 1. netguard_outbox_entries_processed_total - Total processed entries by entity type, operation, and status
	outboxEntriesProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "netguard_outbox_entries_processed_total",
			Help: "Total number of outbox entries processed by entity type, operation, and status",
		},
		[]string{"entity_type", "operation", "status"}, // status = "success" | "failed"
	)

	// 2. netguard_outbox_entries_failed_total - Failed entries by entity type, operation, and error type
	outboxEntriesFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "netguard_outbox_entries_failed_total",
			Help: "Total number of failed outbox entries by entity type, operation, and error type",
		},
		[]string{"entity_type", "operation", "error_type"},
	)

	// 3. netguard_outbox_entries_retried_total - Retried entries by entity type and operation
	outboxEntriesRetriedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "netguard_outbox_entries_retried_total",
			Help: "Total number of retried outbox entries by entity type and operation",
		},
		[]string{"entity_type", "operation"},
	)

	// 4. netguard_outbox_queue_depth - Current number of pending entries in queue
	outboxQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "netguard_outbox_queue_depth",
		Help: "Number of pending entries in sync_outbox table ready for processing",
	})

	// 5. netguard_outbox_processing_duration_seconds - Processing duration histogram by entity type and operation
	outboxProcessingDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "netguard_outbox_processing_duration_seconds",
			Help:    "Duration of processing individual outbox entries in seconds",
			Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
		[]string{"entity_type", "operation"},
	)

	// 6. netguard_outbox_oldest_pending_seconds - Age of oldest pending entry in seconds
	outboxOldestPendingSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "netguard_outbox_oldest_pending_seconds",
		Help: "Age of the oldest pending entry in sync_outbox table (in seconds)",
	})

	// 7. netguard_outbox_success_rate - Success rate per entity type (calculated over time window)
	outboxSuccessRate = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "netguard_outbox_success_rate",
			Help: "Success rate of outbox processing by entity type (0.0 to 1.0)",
		},
		[]string{"entity_type"},
	)

	// 8. netguard_outbox_worker_status - Worker status (1 = running, 0 = stopped)
	outboxWorkerStatus = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "netguard_outbox_worker_status",
		Help: "Status of OutboxWorker: 1 = running, 0 = stopped",
	})

	// ========================================
	// Legacy metrics (kept for backward compatibility)
	// ========================================

	// outboxSyncTotal tracks total number of sync operations by resource type and result
	outboxSyncTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "netguard_outbox_sync_total",
			Help: "Total number of sync operations performed (LEGACY - use netguard_outbox_entries_processed_total)",
		},
		[]string{"resource_type", "result"}, // result = "success" | "failure"
	)

	// outboxSyncDuration tracks sync operation duration by resource type
	outboxSyncDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "netguard_outbox_sync_duration_seconds",
			Help:    "Duration of sync operations in seconds (LEGACY - use netguard_outbox_processing_duration_seconds)",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"resource_type"},
	)

	// outboxRetryAttempts tracks number of retry attempts before success/failure
	outboxRetryAttempts = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "netguard_outbox_retry_attempts",
			Help:    "Number of retry attempts before success or failure",
			Buckets: []float64{0, 1, 3, 5, 10, 20, 50, 100},
		},
		[]string{"resource_type", "result"},
	)

	// outboxFailedPermanent tracks permanently failed entries by resource type and error category
	outboxFailedPermanent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "netguard_outbox_failed_permanent_total",
			Help: "Total number of permanently failed outbox entries",
		},
		[]string{"resource_type", "error_category"},
	)

	// outboxWorkerUp indicates if the OutboxWorker is running (1) or stopped (0)
	outboxWorkerUp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "netguard_outbox_worker_up",
		Help: "1 if OutboxWorker is running, 0 if stopped (LEGACY - use netguard_outbox_worker_status)",
	})

	// outboxWorkerBatchProcessingDuration tracks time to process a full batch
	outboxWorkerBatchProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "netguard_outbox_worker_batch_duration_seconds",
		Help:    "Duration to process a full batch of outbox entries",
		Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
	})
)

// ========================================
// Public metric recording functions (NEW - P0-3)
// ========================================

// RecordProcessingSuccess records a successful processing operation
func RecordProcessingSuccess(entityType string, operation string, duration time.Duration) {
	outboxEntriesProcessedTotal.WithLabelValues(entityType, operation, "success").Inc()
	outboxProcessingDurationSeconds.WithLabelValues(entityType, operation).Observe(duration.Seconds())

	// Update legacy metrics for backward compatibility
	outboxSyncTotal.WithLabelValues(entityType, "success").Inc()
	outboxSyncDuration.WithLabelValues(entityType).Observe(duration.Seconds())
}

// RecordProcessingFailure records a failed processing operation
func RecordProcessingFailure(entityType string, operation string, errorType string, duration time.Duration) {
	outboxEntriesProcessedTotal.WithLabelValues(entityType, operation, "failed").Inc()
	outboxEntriesFailedTotal.WithLabelValues(entityType, operation, errorType).Inc()
	outboxProcessingDurationSeconds.WithLabelValues(entityType, operation).Observe(duration.Seconds())

	// Update legacy metrics for backward compatibility
	outboxSyncTotal.WithLabelValues(entityType, "failure").Inc()
	outboxSyncDuration.WithLabelValues(entityType).Observe(duration.Seconds())
}

// RecordRetry records a retry attempt
func RecordRetry(entityType string, operation string) {
	outboxEntriesRetriedTotal.WithLabelValues(entityType, operation).Inc()
}

// SetQueueDepth sets the current queue depth
func SetQueueDepth(depth int) {
	outboxQueueDepth.Set(float64(depth))
}

// SetOldestPendingAge sets the age of oldest pending entry
func SetOldestPendingAge(ageSeconds float64) {
	outboxOldestPendingSeconds.Set(ageSeconds)
}

// SetSuccessRate sets the success rate for an entity type
func SetSuccessRate(entityType string, rate float64) {
	outboxSuccessRate.WithLabelValues(entityType).Set(rate)
}

// SetWorkerRunning marks the worker as running
func SetWorkerRunning() {
	outboxWorkerStatus.Set(1)
	outboxWorkerUp.Set(1) // Legacy metric
}

// SetWorkerStopped marks the worker as stopped
func SetWorkerStopped() {
	outboxWorkerStatus.Set(0)
	outboxWorkerUp.Set(0) // Legacy metric
}

// RecordBatchDuration records the time taken to process a batch
func RecordBatchDuration(duration time.Duration) {
	outboxWorkerBatchProcessingDuration.Observe(duration.Seconds())
}

// ========================================
// Worker metrics updater
// ========================================

// updateMetrics updates Prometheus metrics during batch processing
// Should be called periodically from the worker loop
func (w *OutboxWorker) updateMetrics(ctx context.Context) {
	// 1. Update queue depth
	depth, err := w.outboxRepo.CountPending(ctx)
	if err != nil {
		w.logger.Error("failed to update queue depth metric", zap.Error(err))
		return
	}
	SetQueueDepth(depth)

	// 2. Update oldest pending entry age
	if err := w.updateOldestPendingAge(ctx); err != nil {
		w.logger.Debug("failed to update oldest pending age metric", zap.Error(err))
		// Non-critical, continue
	}
}

// updateOldestPendingAge calculates and updates the age of oldest pending entry
func (w *OutboxWorker) updateOldestPendingAge(ctx context.Context) error {
	query := `
		SELECT created_at
		FROM sync_outbox
		WHERE status IN ('PENDING', 'FAILED_RETRYABLE')
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT 1
	`

	var createdAt time.Time
	err := w.pool.QueryRow(ctx, query).Scan(&createdAt)
	if err != nil {
		// No pending entries - set to 0
		SetOldestPendingAge(0)
		return nil
	}

	ageSeconds := time.Since(createdAt).Seconds()
	SetOldestPendingAge(ageSeconds)
	return nil
}

// ========================================
// Legacy metric functions (kept for backward compatibility)
// ========================================

// recordSyncSuccess records a successful sync operation (LEGACY)
func recordSyncSuccess(resourceType string, duration time.Duration, attempts int) {
	outboxSyncTotal.WithLabelValues(resourceType, "success").Inc()
	outboxSyncDuration.WithLabelValues(resourceType).Observe(duration.Seconds())
	outboxRetryAttempts.WithLabelValues(resourceType, "success").Observe(float64(attempts))
}

// recordSyncFailure records a failed sync operation (LEGACY)
func recordSyncFailure(resourceType string, duration time.Duration, attempts int) {
	outboxSyncTotal.WithLabelValues(resourceType, "failure").Inc()
	outboxSyncDuration.WithLabelValues(resourceType).Observe(duration.Seconds())
	outboxRetryAttempts.WithLabelValues(resourceType, "failure").Observe(float64(attempts))
}

// recordPermanentFailure records a permanently failed entry (LEGACY)
func recordPermanentFailure(resourceType string, errorCategory string) {
	outboxFailedPermanent.WithLabelValues(resourceType, errorCategory).Inc()
}

// setWorkerUp marks the worker as running (LEGACY)
func setWorkerUp() {
	SetWorkerRunning()
}

// setWorkerDown marks the worker as stopped (LEGACY)
func setWorkerDown() {
	SetWorkerStopped()
}

// recordBatchDuration records the time taken to process a batch (LEGACY)
func recordBatchDuration(duration time.Duration) {
	RecordBatchDuration(duration)
}
