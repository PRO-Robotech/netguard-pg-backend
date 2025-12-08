package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"netguard-pg-backend/internal/sync/monitor"
)

// EntityProcessorInterface defines the interface for entity processors
type EntityProcessorInterface interface {
	// GetEntityType returns the entity type that this processor handles
	GetEntityType() string

	// ProcessChanges processes changes for entities of this type
	ProcessChanges(ctx context.Context, event monitor.SyncChangeEvent) error
}

// OnConnectionSync interface for processors that need to sync on each SGROUP connection/reconnection
type OnConnectionSync interface {
	// PerformConnectionSync is called on each SGROUP connection to sync existing data
	PerformConnectionSync(ctx context.Context) error
}

// ReverseSyncManager manages the reverse synchronization system
// It implements monitor.ConnectionListener to react to SGROUP connection events
type ReverseSyncManager struct {
	mu sync.RWMutex

	// Components
	connectionMonitor *monitor.SGroupConnectionMonitor
	processors        map[string]EntityProcessorInterface
	config            ReverseSyncConfig

	// State
	isRunning   bool
	isConnected bool // tracks SGROUP connection state
	ctx         context.Context
	cancel      context.CancelFunc

	// Statistics
	stats ReverseSyncStats
}

// ReverseSyncConfig holds configuration for the reverse sync manager
type ReverseSyncConfig struct {
	// AutoStart determines if sync should start automatically
	AutoStart bool

	// ProcessingTimeout is the timeout for processing changes
	ProcessingTimeout time.Duration

	// EnableStatistics enables collection of sync statistics
	EnableStatistics bool

	// MaxConcurrentProcessors limits concurrent entity processors
	MaxConcurrentProcessors int

	// HealthCheckInterval is the interval for health checks
	HealthCheckInterval time.Duration
}

// ReverseSyncStats holds statistics about synchronization operations
type ReverseSyncStats struct {
	mu sync.RWMutex

	// Start time
	StartTime time.Time

	// Event counters
	TotalEvents        int64
	ProcessedEvents    int64
	FailedEvents       int64
	LastEventTimestamp time.Time

	// Entity counters by type
	EntityCounts map[string]EntityStats

	// Performance metrics
	AverageProcessingTime time.Duration
	TotalProcessingTime   time.Duration
}

// EntityStats holds statistics for a specific entity type
type EntityStats struct {
	TotalRequests      int64
	SuccessfulSyncs    int64
	FailedSyncs        int64
	LastSyncTime       time.Time
	AverageSuccessRate float64
}

// DefaultReverseSyncConfig returns default configuration
func DefaultReverseSyncConfig() ReverseSyncConfig {
	return ReverseSyncConfig{
		AutoStart:               false,
		ProcessingTimeout:       30 * time.Second,
		EnableStatistics:        true,
		MaxConcurrentProcessors: 10,
		HealthCheckInterval:     60 * time.Second,
	}
}

// NewReverseSyncManager creates a new reverse sync manager
func NewReverseSyncManager(
	connectionMonitor *monitor.SGroupConnectionMonitor,
	config ReverseSyncConfig,
) *ReverseSyncManager {
	return &ReverseSyncManager{
		connectionMonitor: connectionMonitor,
		processors:        make(map[string]EntityProcessorInterface),
		config:            config,
		isConnected:       false,
		stats: ReverseSyncStats{
			EntityCounts: make(map[string]EntityStats),
		},
	}
}

// RegisterProcessor registers an entity processor
func (m *ReverseSyncManager) RegisterProcessor(processor EntityProcessorInterface) error {
	if processor == nil {
		return fmt.Errorf("processor cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entityType := processor.GetEntityType()
	if entityType == "" {
		return fmt.Errorf("processor must have a valid entity type")
	}

	if _, exists := m.processors[entityType]; exists {
		return fmt.Errorf("processor for entity type '%s' already registered", entityType)
	}

	m.processors[entityType] = processor

	return nil
}

// UnregisterProcessor removes an entity processor
func (m *ReverseSyncManager) UnregisterProcessor(entityType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processors[entityType]; !exists {
		return fmt.Errorf("no processor registered for entity type '%s'", entityType)
	}

	delete(m.processors, entityType)

	return nil
}

// Start starts the reverse synchronization system
func (m *ReverseSyncManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("reverse sync manager already running")
	}

	// Create context for this manager
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Initialize statistics
	if m.config.EnableStatistics {
		m.stats.StartTime = time.Now()
	}

	// Set running BEFORE subscribing to avoid race condition
	// where EventConnected arrives before isRunning is true
	m.isRunning = true

	// Subscribe to connection monitor
	if m.connectionMonitor != nil {
		m.connectionMonitor.Subscribe(m)
	}

	// Start health checks if configured
	if m.config.HealthCheckInterval > 0 {
		go m.runHealthChecks()
	}

	// Perform connection sync if already connected to SGROUP
	if m.connectionMonitor != nil && m.connectionMonitor.IsConnected() {
		go m.performConnectionSync()
	}

	return nil
}

// Stop stops the reverse synchronization system
func (m *ReverseSyncManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return nil // Already stopped
	}

	// Cancel context to stop all operations
	if m.cancel != nil {
		m.cancel()
	}

	// Unsubscribe from connection monitor
	if m.connectionMonitor != nil {
		m.connectionMonitor.Unsubscribe("ReverseSyncManager")
	}

	m.isRunning = false

	return nil
}

// OnConnectionEvent implements monitor.ConnectionListener interface
func (m *ReverseSyncManager) OnConnectionEvent(event monitor.ConnectionEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch event.Type {
	case monitor.EventConnected:
		m.isConnected = true
		if m.isRunning {
			go m.performConnectionSync()
		}

	case monitor.EventDisconnected:
		m.isConnected = false
		// Connection lost - stop processing until reconnection

	case monitor.EventTimestamp:
		// Only process if connected and running
		if !m.isConnected || !m.isRunning {
			return
		}

		// Convert ConnectionEvent to SyncChangeEvent
		syncEvent := monitor.SyncChangeEvent{
			Timestamp: event.Timestamp.AsTime(),
			Source:    "sgroup",
			Metadata:  map[string]interface{}{},
		}

		// Process change event asynchronously (don't block the listener)
		go m.processChange(syncEvent)
	}
}

// GetListenerName implements monitor.ConnectionListener interface
func (m *ReverseSyncManager) GetListenerName() string {
	return "ReverseSyncManager"
}

// processChange processes a sync change event
func (m *ReverseSyncManager) processChange(event monitor.SyncChangeEvent) {
	startTime := time.Now()

	m.updateEventStats(event)

	// Use manager context for processing
	m.mu.RLock()
	ctx := m.ctx
	m.mu.RUnlock()

	if ctx == nil {
		return
	}

	// Create processing context with timeout
	processCtx, cancel := context.WithTimeout(ctx, m.config.ProcessingTimeout)
	defer cancel()

	// Process with all registered processors
	var processingErrors []error
	var wg sync.WaitGroup
	errorChan := make(chan error, len(m.processors))

	m.mu.RLock()
	processorsCount := len(m.processors)
	processors := make(map[string]EntityProcessorInterface, processorsCount)
	for k, v := range m.processors {
		processors[k] = v
	}
	m.mu.RUnlock()

	// Limit concurrent processors
	semaphore := make(chan struct{}, m.config.MaxConcurrentProcessors)

	for entityType, processor := range processors {
		wg.Add(1)
		go func(et string, p EntityProcessorInterface) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			err := m.processWithProcessor(processCtx, p, event, et)
			if err != nil {
				errorChan <- fmt.Errorf("processor '%s': %w", et, err)
			}
		}(entityType, processor)
	}

	// Wait for all processors to complete
	go func() {
		wg.Wait()
		close(errorChan)
	}()

	// Collect errors
	for err := range errorChan {
		processingErrors = append(processingErrors, err)
	}

	// Update processing time statistics
	if m.config.EnableStatistics {
		m.updateProcessingStats(time.Since(startTime))
	}

	// Handle errors
	if len(processingErrors) > 0 {
		m.updateFailedEventStats()
	} else {
		m.updateProcessedEventStats()
	}
}

// processWithProcessor processes an event with a specific processor
func (m *ReverseSyncManager) processWithProcessor(
	ctx context.Context,
	processor EntityProcessorInterface,
	event monitor.SyncChangeEvent,
	entityType string,
) error {
	startTime := time.Now()

	err := processor.ProcessChanges(ctx, event)

	// Update entity-specific statistics
	if m.config.EnableStatistics {
		m.updateEntityStats(entityType, err == nil, time.Since(startTime))
	}

	if err != nil {
		return err
	}

	return nil
}

// IsRunning returns true if the manager is running
func (m *ReverseSyncManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetProcessorCount returns the number of registered processors
func (m *ReverseSyncManager) GetProcessorCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.processors)
}

// GetStats returns current synchronization statistics
func (m *ReverseSyncManager) GetStats() ReverseSyncStats {
	if !m.config.EnableStatistics {
		return ReverseSyncStats{}
	}

	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	// Create a deep copy
	stats := ReverseSyncStats{
		StartTime:             m.stats.StartTime,
		TotalEvents:           m.stats.TotalEvents,
		ProcessedEvents:       m.stats.ProcessedEvents,
		FailedEvents:          m.stats.FailedEvents,
		LastEventTimestamp:    m.stats.LastEventTimestamp,
		AverageProcessingTime: m.stats.AverageProcessingTime,
		TotalProcessingTime:   m.stats.TotalProcessingTime,
		EntityCounts:          make(map[string]EntityStats),
	}

	// Copy entity counts
	for k, v := range m.stats.EntityCounts {
		stats.EntityCounts[k] = v
	}

	return stats
}

// runHealthChecks performs periodic health checks
func (m *ReverseSyncManager) runHealthChecks() {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck performs a health check
func (m *ReverseSyncManager) performHealthCheck() {
	m.mu.RLock()
	isRunning := m.isRunning
	_ = len(m.processors)
	m.mu.RUnlock()

	if !isRunning {
		return
	}

}

// updateEventStats updates event statistics
func (m *ReverseSyncManager) updateEventStats(event monitor.SyncChangeEvent) {
	if !m.config.EnableStatistics {
		return
	}

	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalEvents++
	m.stats.LastEventTimestamp = event.Timestamp
}

// updateProcessedEventStats updates processed event statistics
func (m *ReverseSyncManager) updateProcessedEventStats() {
	if !m.config.EnableStatistics {
		return
	}

	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.ProcessedEvents++
}

// updateFailedEventStats updates failed event statistics
func (m *ReverseSyncManager) updateFailedEventStats() {
	if !m.config.EnableStatistics {
		return
	}

	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.FailedEvents++
}

// updateProcessingStats updates processing time statistics
func (m *ReverseSyncManager) updateProcessingStats(duration time.Duration) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalProcessingTime += duration
	if m.stats.ProcessedEvents > 0 {
		m.stats.AverageProcessingTime = m.stats.TotalProcessingTime / time.Duration(m.stats.ProcessedEvents)
	}
}

// updateEntityStats updates entity-specific statistics
func (m *ReverseSyncManager) updateEntityStats(entityType string, success bool, duration time.Duration) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	entityStats := m.stats.EntityCounts[entityType]
	entityStats.TotalRequests++
	entityStats.LastSyncTime = time.Now()

	if success {
		entityStats.SuccessfulSyncs++
	} else {
		entityStats.FailedSyncs++
	}

	// Update success rate
	if entityStats.TotalRequests > 0 {
		entityStats.AverageSuccessRate = float64(entityStats.SuccessfulSyncs) / float64(entityStats.TotalRequests) * 100.0
	}

	m.stats.EntityCounts[entityType] = entityStats
}

// performConnectionSync performs sync for all processors that support OnConnectionSync interface
// This is called on each SGROUP connection/reconnection
func (m *ReverseSyncManager) performConnectionSync() {
	m.mu.RLock()
	processors := make([]EntityProcessorInterface, 0, len(m.processors))
	for _, p := range m.processors {
		processors = append(processors, p)
	}
	ctx := m.ctx
	m.mu.RUnlock()

	if ctx == nil {
		return
	}

	for _, processor := range processors {
		if syncer, ok := processor.(OnConnectionSync); ok {
			syncCtx, cancel := context.WithTimeout(ctx, m.config.ProcessingTimeout)
			_ = syncer.PerformConnectionSync(syncCtx)
			cancel()
		}
	}
}
