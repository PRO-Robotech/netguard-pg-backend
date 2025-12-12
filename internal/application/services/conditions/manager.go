package conditions

import (
	"sync"
	"time"

	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
)

type ConditionManager struct {
	registry        ports.Registry
	outboxRepo      ports.OutboxRepository
	syncManager     interfaces.SyncManager
	batchMutex      sync.Mutex
	pendingBatch    map[string]interface{}
	batchTimer      *time.Timer
	batchSize       int
	batchTimeout    time.Duration
	sequentialMutex *sync.Mutex
}

func NewConditionManager(registry ports.Registry, outboxRepo ports.OutboxRepository) *ConditionManager {
	cm := &ConditionManager{
		registry:        registry,
		outboxRepo:      outboxRepo,
		syncManager:     nil,
		pendingBatch:    make(map[string]interface{}),
		batchSize:       5,
		batchTimeout:    2 * time.Second,
		sequentialMutex: nil,
	}
	return cm
}
func (cm *ConditionManager) SetSequentialMutex(mutex *sync.Mutex) {
	cm.sequentialMutex = mutex
}
func (cm *ConditionManager) SetSyncManager(syncManager interfaces.SyncManager) {
	cm.syncManager = syncManager
}
