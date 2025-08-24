package testutil

import (
	"context"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// MockSyncManager implements a test-friendly sync manager
type MockSyncManager struct {
	syncers map[types.SyncSubjectType]interface{}
}

// NewMockSyncManager creates a new mock sync manager
func NewMockSyncManager() *MockSyncManager {
	return &MockSyncManager{
		syncers: make(map[types.SyncSubjectType]interface{}),
	}
}

// RegisterSyncer registers a syncer for a specific subject type
func (m *MockSyncManager) RegisterSyncer(subjectType types.SyncSubjectType, syncer interface{}) error {
	m.syncers[subjectType] = syncer
	return nil
}

// SyncEntity performs sync operation on a single entity (mock implementation)
func (m *MockSyncManager) SyncEntity(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	// Mock implementation - always succeeds
	return nil
}

// SyncEntityForced performs forced sync operation on a single entity (mock implementation)
func (m *MockSyncManager) SyncEntityForced(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	// Mock implementation - always succeeds
	return nil
}

// SyncBatch performs sync operation on multiple entities (mock implementation)
func (m *MockSyncManager) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	// Mock implementation - always succeeds
	return nil
}

// Start starts the sync manager (mock implementation)
func (m *MockSyncManager) Start(ctx context.Context) error {
	return nil
}

// Stop stops the sync manager (mock implementation)
func (m *MockSyncManager) Stop() error {
	return nil
}

// GetSyncer returns the registered syncer for a subject type
func (m *MockSyncManager) GetSyncer(subjectType types.SyncSubjectType) interface{} {
	return m.syncers[subjectType]
}

// Reset clears all registered syncers (useful for test cleanup)
func (m *MockSyncManager) Reset() {
	m.syncers = make(map[types.SyncSubjectType]interface{})
}
