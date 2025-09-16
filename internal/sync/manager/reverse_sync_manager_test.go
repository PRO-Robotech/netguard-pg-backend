package manager

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/sync/detector"
)

// MockChangeDetector implements detector.ChangeDetector for testing
type MockChangeDetector struct {
	mock.Mock
	mu           sync.RWMutex
	handlers     map[string]detector.ChangeHandler
	isStarted    bool
	handlerCount int
}

func NewMockChangeDetector() *MockChangeDetector {
	return &MockChangeDetector{
		handlers: make(map[string]detector.ChangeHandler),
	}
}

func (m *MockChangeDetector) Start(ctx context.Context) error {
	args := m.Called(ctx)
	m.mu.Lock()
	m.isStarted = true
	m.mu.Unlock()
	return args.Error(0)
}

func (m *MockChangeDetector) Stop() error {
	args := m.Called()
	m.mu.Lock()
	m.isStarted = false
	m.mu.Unlock()
	return args.Error(0)
}

func (m *MockChangeDetector) Subscribe(handler detector.ChangeHandler) error {
	args := m.Called(handler)
	if args.Error(0) == nil {
		m.mu.Lock()
		handlerKey := "handler-" + string(rune(m.handlerCount))
		m.handlers[handlerKey] = handler
		m.handlerCount++
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *MockChangeDetector) Unsubscribe(handler detector.ChangeHandler) error {
	args := m.Called(handler)
	if args.Error(0) == nil {
		m.mu.Lock()
		// Find and remove handler
		for key, h := range m.handlers {
			if h == handler {
				delete(m.handlers, key)
				break
			}
		}
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *MockChangeDetector) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isStarted
}

func (m *MockChangeDetector) GetHandlerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.handlers)
}

// TriggerEvent simulates a change event for testing
func (m *MockChangeDetector) TriggerEvent(event detector.ChangeEvent) {
	m.mu.RLock()
	handlers := make([]detector.ChangeHandler, 0, len(m.handlers))
	for _, h := range m.handlers {
		handlers = append(handlers, h)
	}
	m.mu.RUnlock()

	// Notify all handlers
	for _, handler := range handlers {
		go func(h detector.ChangeHandler) {
			ctx := context.Background()
			h.OnChange(ctx, event)
		}(handler)
	}
}

// MockEntityProcessor implements EntityProcessorInterface for testing
type MockEntityProcessor struct {
	mock.Mock
	entityType string
}

func NewMockEntityProcessor(entityType string) *MockEntityProcessor {
	return &MockEntityProcessor{
		entityType: entityType,
	}
}

func (m *MockEntityProcessor) GetEntityType() string {
	return m.entityType
}

func (m *MockEntityProcessor) ProcessChanges(ctx context.Context, event detector.ChangeEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func TestNewReverseSyncManager(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()

	manager := NewReverseSyncManager(mockDetector, config)

	require.NotNil(t, manager)
	assert.Equal(t, mockDetector, manager.changeDetector)
	assert.Equal(t, config, manager.config)
	assert.False(t, manager.IsRunning())
	assert.Equal(t, 0, manager.GetProcessorCount())
}

func TestDefaultReverseSyncConfig(t *testing.T) {
	config := DefaultReverseSyncConfig()

	assert.False(t, config.AutoStart)
	assert.Equal(t, 30*time.Second, config.ProcessingTimeout)
	assert.True(t, config.EnableStatistics)
	assert.Equal(t, 10, config.MaxConcurrentProcessors)
	assert.Equal(t, 60*time.Second, config.HealthCheckInterval)
}

func TestReverseSyncManager_RegisterProcessor(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	manager := NewReverseSyncManager(mockDetector, DefaultReverseSyncConfig())

	// Test successful registration
	processor1 := NewMockEntityProcessor("host")
	err := manager.RegisterProcessor(processor1)
	require.NoError(t, err)
	assert.Equal(t, 1, manager.GetProcessorCount())

	// Test registering another processor
	processor2 := NewMockEntityProcessor("addressgroup")
	err = manager.RegisterProcessor(processor2)
	require.NoError(t, err)
	assert.Equal(t, 2, manager.GetProcessorCount())

	// Test registering nil processor
	err = manager.RegisterProcessor(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "processor cannot be nil")

	// Test registering duplicate processor
	processor3 := NewMockEntityProcessor("host")
	err = manager.RegisterProcessor(processor3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestReverseSyncManager_UnregisterProcessor(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	manager := NewReverseSyncManager(mockDetector, DefaultReverseSyncConfig())

	// Register a processor
	processor := NewMockEntityProcessor("host")
	err := manager.RegisterProcessor(processor)
	require.NoError(t, err)

	// Test successful unregistration
	err = manager.UnregisterProcessor("host")
	require.NoError(t, err)
	assert.Equal(t, 0, manager.GetProcessorCount())

	// Test unregistering non-existent processor
	err = manager.UnregisterProcessor("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no processor registered")
}

func TestReverseSyncManager_StartStop(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	manager := NewReverseSyncManager(mockDetector, DefaultReverseSyncConfig())

	// Set up mock expectations
	mockDetector.On("Subscribe", manager).Return(nil)
	mockDetector.On("Start", mock.Anything).Return(nil)
	mockDetector.On("Stop").Return(nil)
	mockDetector.On("Unsubscribe", manager).Return(nil)

	ctx := context.Background()

	// Test start
	assert.False(t, manager.IsRunning())
	err := manager.Start(ctx)
	require.NoError(t, err)
	assert.True(t, manager.IsRunning())

	// Test double start
	err = manager.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Test stop
	err = manager.Stop()
	require.NoError(t, err)
	assert.False(t, manager.IsRunning())

	// Test double stop (should not error)
	err = manager.Stop()
	require.NoError(t, err)

	mockDetector.AssertExpectations(t)
}

func TestReverseSyncManager_StartErrors(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	manager := NewReverseSyncManager(mockDetector, DefaultReverseSyncConfig())

	ctx := context.Background()

	// Test subscribe error
	mockDetector.On("Subscribe", manager).Return(errors.New("subscribe failed"))

	err := manager.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to subscribe")
	assert.False(t, manager.IsRunning())

	// Reset mock
	mockDetector.Mock = mock.Mock{}

	// Test start detector error
	mockDetector.On("Subscribe", manager).Return(nil)
	mockDetector.On("Start", mock.Anything).Return(errors.New("start failed"))
	mockDetector.On("Unsubscribe", manager).Return(nil)

	err = manager.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start change detector")
	assert.False(t, manager.IsRunning())

	mockDetector.AssertExpectations(t)
}

func TestReverseSyncManager_OnChange_Success(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.ProcessingTimeout = 5 * time.Second
	config.EnableStatistics = true

	manager := NewReverseSyncManager(mockDetector, config)

	// Register processors
	processor1 := NewMockEntityProcessor("host")
	processor2 := NewMockEntityProcessor("addressgroup")

	err := manager.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = manager.RegisterProcessor(processor2)
	require.NoError(t, err)

	// Set up processor expectations
	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	processor1.On("ProcessChanges", mock.Anything, event).Return(nil)
	processor2.On("ProcessChanges", mock.Anything, event).Return(nil)

	// Process change
	ctx := context.Background()
	err = manager.OnChange(ctx, event)
	require.NoError(t, err)

	// Check statistics
	stats := manager.GetStats()
	assert.Equal(t, int64(1), stats.TotalEvents)
	assert.Equal(t, int64(1), stats.ProcessedEvents)
	assert.Equal(t, int64(0), stats.FailedEvents)

	// Verify processors were called
	processor1.AssertExpectations(t)
	processor2.AssertExpectations(t)
}

func TestReverseSyncManager_OnChange_ProcessorError(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.EnableStatistics = true

	manager := NewReverseSyncManager(mockDetector, config)

	// Register processors
	processor1 := NewMockEntityProcessor("host")
	processor2 := NewMockEntityProcessor("addressgroup")

	err := manager.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = manager.RegisterProcessor(processor2)
	require.NoError(t, err)

	// Set up processor expectations - one succeeds, one fails
	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	processor1.On("ProcessChanges", mock.Anything, event).Return(nil)
	processor2.On("ProcessChanges", mock.Anything, event).Return(errors.New("processing failed"))

	// Process change
	ctx := context.Background()
	err = manager.OnChange(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processing failed for some entities")

	// Check statistics
	stats := manager.GetStats()
	assert.Equal(t, int64(1), stats.TotalEvents)
	assert.Equal(t, int64(0), stats.ProcessedEvents) // Should be 0 because of error
	assert.Equal(t, int64(1), stats.FailedEvents)

	processor1.AssertExpectations(t)
	processor2.AssertExpectations(t)
}

func TestReverseSyncManager_OnChange_NoProcessors(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.EnableStatistics = true

	manager := NewReverseSyncManager(mockDetector, config)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	// Process change with no processors
	ctx := context.Background()
	err := manager.OnChange(ctx, event)
	require.NoError(t, err)

	// Check statistics
	stats := manager.GetStats()
	assert.Equal(t, int64(1), stats.TotalEvents)
	assert.Equal(t, int64(1), stats.ProcessedEvents)
	assert.Equal(t, int64(0), stats.FailedEvents)
}

func TestReverseSyncManager_Statistics(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.EnableStatistics = true

	manager := NewReverseSyncManager(mockDetector, config)

	// Register a processor
	processor := NewMockEntityProcessor("host")
	err := manager.RegisterProcessor(processor)
	require.NoError(t, err)

	// Set up processor expectation
	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}
	processor.On("ProcessChanges", mock.Anything, event).Return(nil)

	// Process change
	ctx := context.Background()
	err = manager.OnChange(ctx, event)
	require.NoError(t, err)

	// Check statistics
	stats := manager.GetStats()
	assert.Equal(t, int64(1), stats.TotalEvents)
	assert.Equal(t, int64(1), stats.ProcessedEvents)
	assert.Equal(t, int64(0), stats.FailedEvents)
	assert.True(t, stats.AverageProcessingTime >= 0) // Can be 0 for very fast operations

	// Check entity statistics
	assert.Contains(t, stats.EntityCounts, "host")
	hostStats := stats.EntityCounts["host"]
	assert.Equal(t, int64(1), hostStats.TotalRequests)
	assert.Equal(t, int64(1), hostStats.SuccessfulSyncs)
	assert.Equal(t, int64(0), hostStats.FailedSyncs)
	assert.Equal(t, 100.0, hostStats.AverageSuccessRate)

	processor.AssertExpectations(t)
}

func TestReverseSyncManager_StatisticsDisabled(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.EnableStatistics = false

	manager := NewReverseSyncManager(mockDetector, config)

	// Process change
	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := manager.OnChange(ctx, event)
	require.NoError(t, err)

	// Statistics should be empty when disabled
	stats := manager.GetStats()
	assert.Equal(t, int64(0), stats.TotalEvents)
	assert.Equal(t, int64(0), stats.ProcessedEvents)
	assert.Equal(t, int64(0), stats.FailedEvents)
}

func TestReverseSyncManager_ConcurrentProcessing(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.MaxConcurrentProcessors = 2
	config.EnableStatistics = true

	manager := NewReverseSyncManager(mockDetector, config)

	// Register multiple processors
	numProcessors := 5
	var processors []*MockEntityProcessor
	for i := 0; i < numProcessors; i++ {
		processor := NewMockEntityProcessor("type" + string(rune(i+'0')))
		processors = append(processors, processor)
		err := manager.RegisterProcessor(processor)
		require.NoError(t, err)
	}

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	// Set up expectations for all processors
	for _, processor := range processors {
		processor.On("ProcessChanges", mock.Anything, event).Return(nil)
	}

	// Process change
	ctx := context.Background()
	err := manager.OnChange(ctx, event)
	require.NoError(t, err)

	// Verify all processors were called
	for _, processor := range processors {
		processor.AssertExpectations(t)
	}

	// Check statistics
	stats := manager.GetStats()
	assert.Equal(t, int64(1), stats.ProcessedEvents)
	assert.Equal(t, numProcessors, len(stats.EntityCounts))
}

func TestReverseSyncManager_HealthChecks(t *testing.T) {
	mockDetector := NewMockChangeDetector()
	config := DefaultReverseSyncConfig()
	config.HealthCheckInterval = 100 * time.Millisecond // Fast for testing
	config.AutoStart = false

	manager := NewReverseSyncManager(mockDetector, config)

	// Set up mock expectations
	mockDetector.On("Subscribe", manager).Return(nil)
	mockDetector.On("Start", mock.Anything).Return(nil)
	mockDetector.On("Stop").Return(nil)
	mockDetector.On("Unsubscribe", manager).Return(nil)

	ctx := context.Background()

	// Start manager
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Let health checks run for a bit
	time.Sleep(300 * time.Millisecond)

	// Stop manager
	err = manager.Stop()
	require.NoError(t, err)

	mockDetector.AssertExpectations(t)
}

func TestEntityStats(t *testing.T) {
	stats := EntityStats{}

	// Test initial state
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.SuccessfulSyncs)
	assert.Equal(t, int64(0), stats.FailedSyncs)
	assert.Equal(t, 0.0, stats.AverageSuccessRate)

	// Test after updates
	stats.TotalRequests = 10
	stats.SuccessfulSyncs = 7
	stats.FailedSyncs = 3
	stats.AverageSuccessRate = 70.0

	assert.Equal(t, int64(10), stats.TotalRequests)
	assert.Equal(t, int64(7), stats.SuccessfulSyncs)
	assert.Equal(t, int64(3), stats.FailedSyncs)
	assert.Equal(t, 70.0, stats.AverageSuccessRate)
}
