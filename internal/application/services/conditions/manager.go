package conditions

import (
	"context"
	"sync"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
)

// IEAgAgRuleGenerator defines interface for generating IEAgAg rules from RuleS2S
type IEAgAgRuleGenerator interface {
	GenerateIEAgAgRulesFromRuleS2SWithReader(ctx context.Context, reader ports.Reader, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error)
}

// IEAgAgRuleManager defines interface for both generating and cleaning up IEAgAg rules
type IEAgAgRuleManager interface {
	IEAgAgRuleGenerator
	// CleanupIEAgAgRulesForRuleS2S removes all IEAgAgRules associated with a RuleS2S that became not Ready
	CleanupIEAgAgRulesForRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) error
}

// RuleS2SService interface for avoiding circular dependency with RuleS2SResourceService
type RuleS2SService interface {
	SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope) error
}

// ConditionManager управляет формированием условий для ресурсов ПОСЛЕ commit транзакций
type ConditionManager struct {
	registry       ports.Registry
	ieAgAgManager  IEAgAgRuleManager      // For IEAgAg rule generation and cleanup
	ruleS2SService RuleS2SService         // For proper IEAgAgRule processing with conditions and external sync
	syncManager    interfaces.SyncManager // For external sync operations to SGROUP

	batchMutex   sync.Mutex
	pendingBatch map[string]interface{} // resourceType:resourceKey -> resource with conditions
	batchTimer   *time.Timer
	batchSize    int
	batchTimeout time.Duration

	sequentialMutex *sync.Mutex
}

// NewConditionManager создает новый ConditionManager
func NewConditionManager(registry ports.Registry) *ConditionManager {
	cm := &ConditionManager{
		registry:       registry,
		ieAgAgManager:  nil, // Will be injected later to avoid circular dependency
		ruleS2SService: nil, // Will be injected later to avoid circular dependency
		syncManager:    nil, // Will be injected later to avoid circular dependency

		pendingBatch: make(map[string]interface{}),
		batchSize:    5,               // Reduced from 10 to 5 to minimize lock contention
		batchTimeout: 2 * time.Second, // Flush batch every 2 seconds max

		sequentialMutex: nil,
	}
	return cm
}

// SetSequentialMutex injects the shared sequential processing mutex from NetguardFacade
// This allows condition batching to participate in the same sequential processing that prevents deadlocks
func (cm *ConditionManager) SetSequentialMutex(mutex *sync.Mutex) {
	cm.sequentialMutex = mutex
}

// SetIEAgAgRuleManager injects the IEAgAg rule manager (called after construction to avoid circular dependency)
func (cm *ConditionManager) SetIEAgAgRuleManager(manager IEAgAgRuleManager) {
	cm.ieAgAgManager = manager
}

// SetRuleS2SService injects the RuleS2S service (called after construction to avoid circular dependency)
func (cm *ConditionManager) SetRuleS2SService(service RuleS2SService) {
	cm.ruleS2SService = service
}

// SetSyncManager injects the SyncManager for external sync operations
func (cm *ConditionManager) SetSyncManager(syncManager interfaces.SyncManager) {
	cm.syncManager = syncManager
}
