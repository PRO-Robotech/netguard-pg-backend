package processors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/sync/detector"
	"netguard-pg-backend/internal/sync/types"
)

// MockHostSynchronizer implements synchronizer.HostSynchronizer for testing
type MockHostSynchronizer struct {
	mock.Mock
}

func (m *MockHostSynchronizer) SyncHosts(ctx context.Context, namespace string) (*types.HostSyncResult, error) {
	args := m.Called(ctx, namespace)
	return args.Get(0).(*types.HostSyncResult), args.Error(1)
}

func (m *MockHostSynchronizer) SyncHostsByUUIDs(ctx context.Context, uuids []string) (*types.HostSyncResult, error) {
	args := m.Called(ctx, uuids)
	return args.Get(0).(*types.HostSyncResult), args.Error(1)
}

func (m *MockHostSynchronizer) SyncAllHosts(ctx context.Context) (*types.HostSyncResult, error) {
	args := m.Called(ctx)
	return args.Get(0).(*types.HostSyncResult), args.Error(1)
}

func TestNewHostProcessor(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()

	processor := NewHostProcessor(mockSynchronizer, config)

	require.NotNil(t, processor)
	assert.Equal(t, "host", processor.GetEntityType())

	// Verify it implements the interface
	var _ EntityProcessor = processor
}

func TestDefaultHostProcessorConfig(t *testing.T) {
	config := DefaultHostProcessorConfig()

	assert.False(t, config.EnableNamespaceFiltering)
	assert.Empty(t, config.AllowedNamespaces)
	assert.False(t, config.EnableFullSyncOnChange)
	assert.Equal(t, 3, config.MaxRetryAttempts)
}

func TestHostProcessor_ProcessChanges_FullSync(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	config.EnableFullSyncOnChange = true

	processor := NewHostProcessor(mockSynchronizer, config)

	// Create test result
	result := types.NewHostSyncResult()
	result.AddSyncedHost("uuid1")
	result.AddSyncedHost("uuid2")
	result.SetTotalRequested(2)

	// Set up mock expectation
	mockSynchronizer.On("SyncAllHosts", mock.Anything).Return(result, nil)

	// Create test event
	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_ProcessChanges_NamespaceSync_NoFiltering(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	config.EnableFullSyncOnChange = false
	config.EnableNamespaceFiltering = false

	processor := NewHostProcessor(mockSynchronizer, config)

	// Create test result
	result := types.NewHostSyncResult()
	result.AddSyncedHost("uuid1")

	// When no namespace filtering, should call SyncAllHosts
	mockSynchronizer.On("SyncAllHosts", mock.Anything).Return(result, nil)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_ProcessChanges_NamespaceSync_WithFiltering(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	config.EnableFullSyncOnChange = false
	config.EnableNamespaceFiltering = true
	config.AllowedNamespaces = []string{"default", "kube-system"}

	processor := NewHostProcessor(mockSynchronizer, config)

	// Create test results for each namespace
	result1 := types.NewHostSyncResult()
	result1.AddSyncedHost("uuid1")
	result1.SetTotalRequested(1)

	result2 := types.NewHostSyncResult()
	result2.AddSyncedHost("uuid2")
	result2.AddSyncedHost("uuid3")
	result2.SetTotalRequested(2)

	// Set up mock expectations for each namespace
	mockSynchronizer.On("SyncHosts", mock.Anything, "default").Return(result1, nil)
	mockSynchronizer.On("SyncHosts", mock.Anything, "kube-system").Return(result2, nil)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_ProcessChanges_NamespaceSync_WithErrors(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	config.EnableFullSyncOnChange = false
	config.EnableNamespaceFiltering = true
	config.AllowedNamespaces = []string{"default", "failing-namespace", "kube-system"}

	processor := NewHostProcessor(mockSynchronizer, config)

	// Create test results
	result1 := types.NewHostSyncResult()
	result1.AddSyncedHost("uuid1")

	result3 := types.NewHostSyncResult()
	result3.AddSyncedHost("uuid3")

	// Set up mock expectations - one success, one failure, one success
	mockSynchronizer.On("SyncHosts", mock.Anything, "default").Return(result1, nil)
	mockSynchronizer.On("SyncHosts", mock.Anything, "failing-namespace").Return(&types.HostSyncResult{}, errors.New("sync failed"))
	mockSynchronizer.On("SyncHosts", mock.Anything, "kube-system").Return(result3, nil)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	// Should not return error even if some namespaces fail
	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_ProcessChanges_SyncError(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	config.EnableFullSyncOnChange = true

	processor := NewHostProcessor(mockSynchronizer, config)

	// Set up mock to return error
	mockSynchronizer.On("SyncAllHosts", mock.Anything).Return(&types.HostSyncResult{}, errors.New("connection failed"))

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "host sync failed")
	assert.Contains(t, err.Error(), "connection failed")
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_ProcessChanges_EmptyResult(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()

	processor := NewHostProcessor(mockSynchronizer, config)

	// Create empty result
	result := types.NewHostSyncResult()

	mockSynchronizer.On("SyncAllHosts", mock.Anything).Return(result, nil)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_ProcessChanges_ResultWithErrors(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()

	processor := NewHostProcessor(mockSynchronizer, config)

	// Create result with both success and failures
	result := types.NewHostSyncResult()
	result.AddSyncedHost("uuid1")
	result.AddFailedHost("uuid2", "IP validation failed")
	result.SetTotalRequested(2)

	mockSynchronizer.On("SyncAllHosts", mock.Anything).Return(result, nil)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}

func TestHostProcessor_mergeResults(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	processor := NewHostProcessor(mockSynchronizer, config).(*hostProcessor)

	// Create aggregate result
	aggregate := types.NewHostSyncResult()
	aggregate.AddSyncedHost("existing-uuid")
	aggregate.SetTotalRequested(1)

	// Create individual result to merge
	individual := types.NewHostSyncResult()
	individual.AddSyncedHost("new-uuid1")
	individual.AddSyncedHost("new-uuid2")
	individual.AddFailedHost("failed-uuid", "test error")
	individual.SetTotalRequested(3)

	// Merge results
	processor.mergeResults(aggregate, individual)

	// Verify merged results
	assert.Len(t, aggregate.SyncedHostUUIDs, 3)
	assert.Contains(t, aggregate.SyncedHostUUIDs, "existing-uuid")
	assert.Contains(t, aggregate.SyncedHostUUIDs, "new-uuid1")
	assert.Contains(t, aggregate.SyncedHostUUIDs, "new-uuid2")

	assert.Len(t, aggregate.FailedUUIDs, 1)
	assert.Contains(t, aggregate.FailedUUIDs, "failed-uuid")
	assert.Equal(t, "test error", aggregate.GetError("failed-uuid"))

	assert.Equal(t, 4, aggregate.TotalRequested) // 1 + 3
	assert.Equal(t, 3, aggregate.TotalSynced)    // Auto-calculated
	assert.Equal(t, 1, aggregate.TotalFailed)    // Auto-calculated
}

func TestHostProcessor_GetEntityType(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := DefaultHostProcessorConfig()
	processor := NewHostProcessor(mockSynchronizer, config)

	assert.Equal(t, "host", processor.GetEntityType())
}

func TestHostProcessor_CustomConfig(t *testing.T) {
	mockSynchronizer := &MockHostSynchronizer{}
	config := HostProcessorConfig{
		EnableNamespaceFiltering: true,
		AllowedNamespaces:        []string{"custom-namespace"},
		EnableFullSyncOnChange:   true,
		MaxRetryAttempts:         5,
	}

	processor := NewHostProcessor(mockSynchronizer, config)

	require.NotNil(t, processor)
	assert.Equal(t, "host", processor.GetEntityType())

	// Test that custom config is used
	result := types.NewHostSyncResult()
	result.AddSyncedHost("uuid1")

	// With EnableFullSyncOnChange = true, should call SyncAllHosts
	mockSynchronizer.On("SyncAllHosts", mock.Anything).Return(result, nil)

	event := detector.ChangeEvent{
		Source:    "test-sgroup",
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := processor.ProcessChanges(ctx, event)

	require.NoError(t, err)
	mockSynchronizer.AssertExpectations(t)
}
