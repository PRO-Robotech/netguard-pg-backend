package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories"
	"netguard-pg-backend/internal/sync/monitor"
	"netguard-pg-backend/internal/sync/syncers"
)

// OutboxWorker polls sync_outbox table and processes pending entries
type OutboxWorker struct {
	// Database
	pool       *pgxpool.Pool
	outboxRepo ports.OutboxRepository

	// In-memory registry for loading resources
	registry ports.Registry

	// Syncers for entity resources
	hostSyncer         *syncers.HostSyncer
	addressGroupSyncer *syncers.AddressGroupSyncer
	networkSyncer      *syncers.NetworkSyncer
	serviceSyncer      *syncers.ServiceSyncer
	svcSvcRuleSyncer   *syncers.SvcSvcRuleSyncer

	// Configuration
	config *WorkerConfig
	logger *zap.Logger

	// Connection monitoring (NEW)
	connectionMonitor *monitor.SGroupConnectionMonitor
	isPaused          bool
	pausedMu          sync.RWMutex // protects isPaused

	// Lifecycle management
	stopCh    chan struct{}
	doneCh    chan struct{}
	wg        sync.WaitGroup
	startedAt time.Time
	mu        sync.RWMutex // protects metrics and lastError

	// Metrics and status tracking
	processed int64
	failed    int64
	lastError string
	lastPoll  time.Time
}

// NewOutboxWorker creates a new OutboxWorker instance
func NewOutboxWorker(
	pool *pgxpool.Pool,
	registry ports.Registry,
	hostSyncer *syncers.HostSyncer,
	addressGroupSyncer *syncers.AddressGroupSyncer,
	networkSyncer *syncers.NetworkSyncer,
	serviceSyncer *syncers.ServiceSyncer,
	svcSvcRuleSyncer *syncers.SvcSvcRuleSyncer,
	logger *zap.Logger,
	config *WorkerConfig,
	connectionMonitor *monitor.SGroupConnectionMonitor,
) *OutboxWorker {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		logger.Fatal("invalid worker config", zap.Error(err))
	}

	if connectionMonitor == nil {
		logger.Fatal("connectionMonitor cannot be nil")
	}

	return &OutboxWorker{
		pool:               pool,
		outboxRepo:         repositories.NewOutboxRepository(pool),
		registry:           registry,
		hostSyncer:         hostSyncer,
		addressGroupSyncer: addressGroupSyncer,
		networkSyncer:      networkSyncer,
		serviceSyncer:      serviceSyncer,
		svcSvcRuleSyncer:   svcSvcRuleSyncer,
		config:             config,
		logger:             logger.With(zap.String("component", "outbox-worker")),
		connectionMonitor:  connectionMonitor,
		isPaused:           false,
		stopCh:             make(chan struct{}),
		doneCh:             make(chan struct{}),
	}
}

// Start starts the OutboxWorker background goroutine
func (w *OutboxWorker) Start(ctx context.Context) error {
	w.startedAt = time.Now()
	defer close(w.doneCh)
	defer setWorkerDown()

	// Mark worker as running
	setWorkerUp()

	// Subscribe to connection events (NEW)
	w.connectionMonitor.Subscribe(w)
	w.logger.Info("OutboxWorker subscribed to connection events")

	w.logger.Info("OutboxWorker starting",
		zap.Duration("poll_interval", w.config.PollInterval),
		zap.Int("batch_size", w.config.BatchSize),
	)
	defer w.logger.Info("OutboxWorker stopped")

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	// Metrics update ticker (every 10 seconds)
	metricsTicker := time.NewTicker(10 * time.Second)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("context cancelled, stopping worker")
			return ctx.Err()

		case <-w.stopCh:
			w.logger.Info("stop signal received")
			return nil

		case <-ticker.C:
			w.mu.Lock()
			w.lastPoll = time.Now()
			w.mu.Unlock()

			batchStart := time.Now()
			if err := w.processBatch(ctx); err != nil {
				w.logger.Error("failed to process batch", zap.Error(err))
				w.mu.Lock()
				w.lastError = err.Error()
				w.mu.Unlock()
				atomic.AddInt64(&w.failed, 1)
				// Continue polling (don't stop on transient errors)
			} else {
				w.mu.Lock()
				w.lastError = ""
				w.mu.Unlock()
			}
			recordBatchDuration(time.Since(batchStart))

		case <-metricsTicker.C:
			// Update metrics periodically
			if w.config.MetricsEnabled {
				w.updateMetrics(ctx)
			}
		}
	}
}

// Stop stops the OutboxWorker gracefully
func (w *OutboxWorker) Stop() {
	w.logger.Info("OutboxWorker stopping...")

	// Close stop channel (safe - only called once)
	select {
	case <-w.stopCh:
		// Already stopped
		return
	default:
		close(w.stopCh)
	}

	// Wait for goroutines to finish (if any)
	w.wg.Wait()

	w.logger.Info("OutboxWorker stopped gracefully")
}

// Done returns a channel that is closed when the worker has completely stopped
func (w *OutboxWorker) Done() <-chan struct{} {
	return w.doneCh
}

// ProcessOnce processes one batch of pending outbox entries (for testing)
// This is a public wrapper around processBatch for use in integration tests
func (w *OutboxWorker) ProcessOnce(ctx context.Context) error {
	return w.processBatch(ctx)
}

// processBatch processes a single batch of pending outbox entries
func (w *OutboxWorker) processBatch(ctx context.Context) error {
	// Check if processing is paused (NEW)
	w.pausedMu.RLock()
	paused := w.isPaused
	w.pausedMu.RUnlock()

	if paused {
		w.logger.Debug("Skipping batch processing - SGROUP disconnected")
		return nil
	}

	// Find pending entries using FOR UPDATE SKIP LOCKED
	entries, err := w.outboxRepo.FindPending(ctx, w.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to find pending entries: %w", err)
	}

	if len(entries) == 0 {
		// No work to do
		return nil
	}

	w.logger.Info("processing batch",
		zap.Int("count", len(entries)),
	)

	// Process each entry
	for _, entry := range entries {
		if err := w.processEntry(ctx, entry); err != nil {
			w.logger.Error("failed to process entry",
				zap.String("resource_type", entry.ResourceType),
				zap.String("resource_id", entry.ResourceID.String()),
				zap.Error(err),
			)
			// Note: failed is incremented per batch, not per entry
		} else {
			atomic.AddInt64(&w.processed, 1)
		}
	}

	return nil
}

// processEntry processes a single outbox entry by dispatching to the correct processor
// based on the entry's TargetSystem field:
//   - SGROUP: Entity resources that sync to SGROUP (Host, AddressGroup, Network, Service)
//   - INTERNAL: Process resources that manage dependencies (HostBinding, NetworkBinding)
func (w *OutboxWorker) processEntry(ctx context.Context, entry *domain.OutboxEntry) error {
	w.logger.Debug("dispatching entry to processor",
		zap.String("entry_id", entry.ID.String()),
		zap.String("resource_type", entry.ResourceType),
		zap.String("target_system", string(entry.TargetSystem)),
		zap.String("operation", string(entry.Operation)))

	switch entry.TargetSystem {
	case domain.TargetSystemSGROUP:
		// Entity resources: Host, AddressGroup, Network, Service
		// These sync to SGROUP after dependency checks
		return w.processEntityResource(ctx, entry)

	case domain.TargetSystemInternal:
		// Process resources: HostBinding, NetworkBinding
		// These check dependencies and propagate Ready conditions
		return w.processProcessResource(ctx, entry)

	default:
		// Unknown target system - this is a permanent error (data corruption)
		err := fmt.Errorf("unknown target system: %s", entry.TargetSystem)
		w.logger.Error("invalid target system in outbox entry",
			zap.String("entry_id", entry.ID.String()),
			zap.String("resource_type", entry.ResourceType),
			zap.String("target_system", string(entry.TargetSystem)),
			zap.Error(err))
		return err
	}
}

// OnConnectionEvent обрабатывает события связи с SGROUP
// Implements monitor.ConnectionListener interface
func (w *OutboxWorker) OnConnectionEvent(event monitor.ConnectionEvent) {
	switch event.Type {
	case monitor.EventConnected:
		w.pausedMu.Lock()
		wasPaused := w.isPaused
		w.isPaused = false
		w.pausedMu.Unlock()

		if wasPaused {
			w.logger.Info("OutboxWorker: SGROUP connected, resuming processing")
		}

	case monitor.EventDisconnected:
		w.pausedMu.Lock()
		w.isPaused = true
		w.pausedMu.Unlock()

		w.logger.Warn("OutboxWorker: SGROUP disconnected, pausing processing",
			zap.Error(event.Error))

	case monitor.EventTimestamp:
		// OutboxWorker не интересуют timestamps
	}
}

// GetListenerName возвращает имя listener для логирования
// Implements monitor.ConnectionListener interface
func (w *OutboxWorker) GetListenerName() string {
	return "OutboxWorker"
}
