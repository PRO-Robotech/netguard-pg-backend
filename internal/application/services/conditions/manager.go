package conditions

import (
	"context"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/sync/interfaces"
	"sync"
	"time"
)

type IEAgAgRuleGenerator interface {
	GenerateIEAgAgRulesFromRuleS2SWithReader(ctx context.Context, reader ports.Reader, ruleS2S models.RuleS2S) ([]models.IEAgAgRule, error)
}
type IEAgAgRuleManager interface {
	IEAgAgRuleGenerator
	CleanupIEAgAgRulesForRuleS2S(ctx context.Context, ruleS2S models.RuleS2S) error
}
type RuleS2SService interface {
	SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope) error
}
type ConditionManager struct {
	registry        ports.Registry
	outboxRepo      ports.OutboxRepository
	ieAgAgManager   IEAgAgRuleManager
	ruleS2SService  RuleS2SService
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
		ieAgAgManager:   nil,
		ruleS2SService:  nil,
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
func (cm *ConditionManager) SetIEAgAgRuleManager(manager IEAgAgRuleManager) {
	cm.ieAgAgManager = manager
}
func (cm *ConditionManager) SetRuleS2SService(service RuleS2SService) {
	cm.ruleS2SService = service
}
func (cm *ConditionManager) SetSyncManager(syncManager interfaces.SyncManager) {
	cm.syncManager = syncManager
}
