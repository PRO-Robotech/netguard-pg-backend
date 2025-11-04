package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/sync/monitor"
)

// Mock OutboxRepository for testing
type mockOutboxRepo struct {
	findPendingFunc   func(ctx context.Context, limit int) ([]*domain.OutboxEntry, error)
	findPendingCalled bool
	deleteFunc        func(ctx context.Context, id uuid.UUID) error
	deleteCalled      bool
}

func (m *mockOutboxRepo) Create(ctx context.Context, entry *domain.OutboxEntry) error {
	return nil
}

func (m *mockOutboxRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.OutboxEntry, error) {
	return nil, nil
}

func (m *mockOutboxRepo) FindByResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*domain.OutboxEntry, error) {
	return []*domain.OutboxEntry{}, nil
}

func (m *mockOutboxRepo) FindPending(ctx context.Context, limit int) ([]*domain.OutboxEntry, error) {
	m.findPendingCalled = true
	if m.findPendingFunc != nil {
		return m.findPendingFunc(ctx, limit)
	}
	return []*domain.OutboxEntry{}, nil
}

func (m *mockOutboxRepo) CountPending(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockOutboxRepo) CountPendingForResource(ctx context.Context, resourceType, namespace, name string) (int, error) {
	return 0, nil
}

func (m *mockOutboxRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OutboxStatus, processedAt *time.Time, lastError *string, errorCategory *string) error {
	return nil
}

func (m *mockOutboxRepo) IncrementAttempts(ctx context.Context, id uuid.UUID, nextRetryAt *time.Time) error {
	return nil
}

func (m *mockOutboxRepo) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockOutboxRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.deleteCalled = true
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockOutboxRepo) DeleteOldSuccessful(ctx context.Context, olderThan time.Duration) (int64, error) {
	return 0, nil
}

// createTestMonitor создает mock монитор для тестов
func createTestMonitor(t *testing.T) *monitor.SGroupConnectionMonitor {
	logger := zaptest.NewLogger(t)
	mockGateway := &mockSGroupGateway{}
	config := monitor.DefaultConfig()

	return monitor.NewSGroupConnectionMonitor(mockGateway, config, logger)
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 5*time.Second, config.PollInterval)
	assert.Equal(t, 10, config.BatchSize)
	assert.Equal(t, 4*time.Second, config.SGROUPTimeout)
	assert.Equal(t, 1, config.MaxConcurrentBatches)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *WorkerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "zero poll interval",
			config: &WorkerConfig{
				PollInterval:          0,
				BatchSize:             10,
				SGROUPTimeout:         4 * time.Second,
				MaxConcurrentBatches:  1,
				MaxAttemptsValidation: 3,
				MaxAttemptsTemporary:  20,
				MaxAttemptsNetwork:    100,
				HealthCheckInterval:   10 * time.Second,
			},
			wantErr: true,
			errMsg:  "poll_interval must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewOutboxWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockMonitor := createTestMonitor(t)

	worker := NewOutboxWorker(
		nil, // pool
		nil, // registry
		nil, // hostSyncer
		nil, // addressGroupSyncer
		nil, // networkSyncer
		nil, // serviceSyncer
		nil, // svcSvcRuleSyncer
		nil, // svcFqdnRuleSyncer
		nil, // conditionManager
		logger,
		nil, // uses default config
		mockMonitor,
		nil, // portMappingRegenerator
	)

	require.NotNil(t, worker)
	assert.NotNil(t, worker.config)
	assert.Equal(t, 5*time.Second, worker.config.PollInterval)
	assert.NotNil(t, worker.connectionMonitor)
}

func TestNewOutboxWorker_WithCustomConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockMonitor := createTestMonitor(t)
	config := &WorkerConfig{
		PollInterval:          10 * time.Second,
		BatchSize:             20,
		SGROUPTimeout:         5 * time.Second,
		MaxConcurrentBatches:  2,
		MaxAttemptsValidation: 3,
		MaxAttemptsTemporary:  20,
		MaxAttemptsNetwork:    100,
		HealthCheckInterval:   10 * time.Second,
		MetricsEnabled:        true,
	}

	worker := NewOutboxWorker(
		nil, // pool
		nil, // registry
		nil, // hostSyncer
		nil, // addressGroupSyncer
		nil, // networkSyncer
		nil, // serviceSyncer
		nil, // svcSvcRuleSyncer
		nil, // svcFqdnRuleSyncer
		nil, // conditionManager
		logger,
		config,
		mockMonitor,
		nil, // portMappingRegenerator
	)

	require.NotNil(t, worker)
	assert.Equal(t, 10*time.Second, worker.config.PollInterval)
	assert.Equal(t, 20, worker.config.BatchSize)
}

// TestOutboxWorker_Stop_GracefulShutdown tests graceful shutdown
func TestOutboxWorker_Stop_GracefulShutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := &mockOutboxRepo{}

	worker := &OutboxWorker{
		logger:     logger,
		outboxRepo: mockRepo,
		config:     DefaultConfig(),
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}

	// Stop should close stopCh
	worker.Stop()

	// Verify stopCh is closed
	select {
	case <-worker.stopCh:
		// Success - channel closed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stopCh not closed after Stop()")
	}

	// Calling Stop() again should be safe (idempotent)
	worker.Stop()
}

// TestOutboxWorker_ProcessOnce_EmptyBatch tests processing with no entries
func TestOutboxWorker_ProcessOnce_EmptyBatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := &mockOutboxRepo{
		findPendingFunc: func(ctx context.Context, limit int) ([]*domain.OutboxEntry, error) {
			return []*domain.OutboxEntry{}, nil
		},
	}

	worker := &OutboxWorker{
		logger:     logger,
		outboxRepo: mockRepo,
		config:     DefaultConfig(),
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
		isPaused:   false, // not paused
	}

	ctx := context.Background()
	err := worker.ProcessOnce(ctx)

	assert.NoError(t, err)
	assert.True(t, mockRepo.findPendingCalled)
}

// TestOutboxWorker_ProcessOnce_WithEntries tests processing with entries
func TestOutboxWorker_ProcessOnce_WithEntries(t *testing.T) {
	t.Skip("Skipping: test requires full worker initialization with conditionManager and registry. " +
		"Use integration tests or e2e tests for processing verification.")
}

// TestOutboxWorker_ProcessBatch_HandlesErrors tests batch processing with errors
func TestOutboxWorker_ProcessBatch_HandlesErrors(t *testing.T) {
	t.Skip("Skipping: test requires full worker initialization with conditionManager and registry. " +
		"Use integration tests or e2e tests for processing verification.")
}

// TestOutboxWorker_Done_Channel tests Done() channel behavior
func TestOutboxWorker_Done_Channel(t *testing.T) {
	logger := zaptest.NewLogger(t)

	worker := &OutboxWorker{
		logger: logger,
		config: DefaultConfig(),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	// doneCh should not be closed initially
	select {
	case <-worker.Done():
		t.Fatal("doneCh should not be closed initially")
	case <-time.After(10 * time.Millisecond):
		// Expected - channel not closed
	}

	// Close doneCh manually (simulating worker stop)
	close(worker.doneCh)

	// Now Done() should return a closed channel
	select {
	case <-worker.Done():
		// Success - channel closed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done() channel not closed")
	}
}

// TestOutboxWorker_GetSyncerForEntity tests syncer lookup
func TestOutboxWorker_GetSyncerForEntity(t *testing.T) {
	logger := zaptest.NewLogger(t)

	worker := &OutboxWorker{
		logger:             logger,
		hostSyncer:         nil, // nil syncers for test
		addressGroupSyncer: nil,
		networkSyncer:      nil,
		serviceSyncer:      nil,
	}

	tests := []struct {
		name         string
		resourceType string
		expectError  bool
	}{
		{
			name:         "Host",
			resourceType: "Host",
			expectError:  false,
		},
		{
			name:         "AddressGroup",
			resourceType: "AddressGroup",
			expectError:  false,
		},
		{
			name:         "Network",
			resourceType: "Network",
			expectError:  false,
		},
		{
			name:         "Service",
			resourceType: "Service",
			expectError:  false,
		},
		{
			name:         "Unknown type",
			resourceType: "UnknownType",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := worker.getSyncerForEntity(tt.resourceType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no syncer registered")
			} else {
				// Returns nil syncer but no error (syncer lookup works)
				assert.NoError(t, err)
			}
		})
	}
}

// TestOutboxWorker_ProcessBatch_WhenPaused tests that paused worker skips batch
func TestOutboxWorker_ProcessBatch_WhenPaused(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockRepo := &mockOutboxRepo{}

	worker := &OutboxWorker{
		logger:     logger,
		outboxRepo: mockRepo,
		config:     DefaultConfig(),
		isPaused:   true, // PAUSED
	}

	ctx := context.Background()
	err := worker.processBatch(ctx)

	// Should return nil without calling FindPending
	assert.NoError(t, err)
	assert.False(t, mockRepo.findPendingCalled, "FindPending should not be called when paused")
}
