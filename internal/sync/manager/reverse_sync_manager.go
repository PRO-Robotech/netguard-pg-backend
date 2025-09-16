package manager

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"netguard-pg-backend/internal/sync/detector"
)

// EntityProcessorInterface defines the interface for entity processors
type EntityProcessorInterface interface {
	// GetEntityType returns the entity type that this processor handles
	GetEntityType() string

	// ProcessChanges processes changes for entities of this type
	ProcessChanges(ctx context.Context, event detector.ChangeEvent) error
}

// ReverseSyncManager manages the reverse synchronization system
type ReverseSyncManager struct {
	mu sync.RWMutex

	// Components
	changeDetector detector.ChangeDetector
	processors     map[string]EntityProcessorInterface
	config         ReverseSyncConfig

	// State
	isRunning bool
	ctx       context.Context
	cancel    context.CancelFunc

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
	changeDetector detector.ChangeDetector,
	config ReverseSyncConfig,
) *ReverseSyncManager {
	return &ReverseSyncManager{
		changeDetector: changeDetector,
		processors:     make(map[string]EntityProcessorInterface),
		config:         config,
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
	log.Printf("🔧 DEBUG: ReverseSyncManager.RegisterProcessor - Registered processor for entity type: %s", entityType)

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
	log.Printf("🔧 DEBUG: ReverseSyncManager.UnregisterProcessor - Unregistered processor for entity type: %s", entityType)

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

	// Subscribe to change detector
	err := m.changeDetector.Subscribe(m)
	if err != nil {
		return fmt.Errorf("failed to subscribe to change detector: %w", err)
	}

	// Start change detector
	err = m.changeDetector.Start(m.ctx)
	if err != nil {
		m.changeDetector.Unsubscribe(m)
		return fmt.Errorf("failed to start change detector: %w", err)
	}

	m.isRunning = true
	log.Printf("✅ INFO: ReverseSyncManager.Start - Reverse synchronization system started with %d processors", len(m.processors))

	// Start health checks if configured
	if m.config.HealthCheckInterval > 0 {
		go m.runHealthChecks()
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

	// Stop change detector
	err := m.changeDetector.Stop()
	if err != nil {
		log.Printf("⚠️  WARNING: ReverseSyncManager.Stop - Error stopping change detector: %v", err)
	}

	// Unsubscribe from change detector
	err = m.changeDetector.Unsubscribe(m)
	if err != nil {
		log.Printf("⚠️  WARNING: ReverseSyncManager.Stop - Error unsubscribing from change detector: %v", err)
	}

	m.isRunning = false
	log.Printf("✅ INFO: ReverseSyncManager.Stop - Reverse synchronization system stopped")

	return nil
}

// OnChange implements detector.ChangeHandler interface
func (m *ReverseSyncManager) OnChange(ctx context.Context, event detector.ChangeEvent) error {
	startTime := time.Now()

	m.updateEventStats(event)

	log.Printf("🔧 DEBUG: ReverseSyncManager.OnChange - Processing change event from %s at %v",
		event.Source, event.Timestamp)

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

		var errorMsg string
		for i, err := range processingErrors {
			if i == 0 {
				errorMsg = err.Error()
			} else {
				errorMsg += "; " + err.Error()
			}
		}

		log.Printf("❌ ERROR: ReverseSyncManager.OnChange - Some processors failed: %s", errorMsg)
		return fmt.Errorf("processing failed for some entities: %s", errorMsg)
	}

	m.updateProcessedEventStats()
	log.Printf("✅ SUCCESS: ReverseSyncManager.OnChange - Successfully processed change event with %d processors", processorsCount)

	return nil
}

// processWithProcessor processes an event with a specific processor
func (m *ReverseSyncManager) processWithProcessor(
	ctx context.Context,
	processor EntityProcessorInterface,
	event detector.ChangeEvent,
	entityType string,
) error {
	startTime := time.Now()

	log.Printf("🔧 DEBUG: ReverseSyncManager.processWithProcessor - Processing with %s processor", entityType)

	err := processor.ProcessChanges(ctx, event)

	// Update entity-specific statistics
	if m.config.EnableStatistics {
		m.updateEntityStats(entityType, err == nil, time.Since(startTime))
	}

	if err != nil {
		log.Printf("❌ ERROR: ReverseSyncManager.processWithProcessor - Processor %s failed: %v", entityType, err)
		return err
	}

	log.Printf("✅ SUCCESS: ReverseSyncManager.processWithProcessor - Processor %s completed successfully", entityType)
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
	processorCount := len(m.processors)
	m.mu.RUnlock()

	if !isRunning {
		log.Printf("⚠️  WARNING: ReverseSyncManager.performHealthCheck - Manager is not running")
		return
	}

	log.Printf("✅ INFO: ReverseSyncManager.performHealthCheck - System healthy: %d processors active", processorCount)
}

// updateEventStats updates event statistics
func (m *ReverseSyncManager) updateEventStats(event detector.ChangeEvent) {
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
