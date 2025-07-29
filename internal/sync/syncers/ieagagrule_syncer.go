package syncers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// IEAgAgRuleSyncer implements EntitySyncer for IEAgAgRule entities
type IEAgAgRuleSyncer struct {
	gateway interfaces.SGroupGateway
	logger  logr.Logger
}

// NewIEAgAgRuleSyncer creates a new IEAgAgRule syncer
func NewIEAgAgRuleSyncer(gateway interfaces.SGroupGateway, logger logr.Logger) *IEAgAgRuleSyncer {
	return &IEAgAgRuleSyncer{
		gateway: gateway,
		logger:  logger,
	}
}

// Sync synchronizes a single IEAgAgRule entity
func (s *IEAgAgRuleSyncer) Sync(ctx context.Context, entity interfaces.SyncableEntity, operation types.SyncOperation) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	fmt.Printf("🔧 DEBUG: IEAgAgRuleSyncer.Sync - Starting sync for entity %s (operation: %s)\n", entity.GetSyncKey(), operation)

	// Validate entity type
	if entity.GetSyncSubjectType() != types.SyncSubjectTypeIEAgAgRules {
		return fmt.Errorf("invalid entity type for IEAgAgRuleSyncer: %s", entity.GetSyncSubjectType())
	}

	// Convert entity to sgroups protobuf format
	protoData, err := entity.ToSGroupsProto()
	if err != nil {
		fmt.Printf("❌ ERROR: IEAgAgRuleSyncer.Sync - Failed to convert entity %s to proto: %v\n", entity.GetSyncKey(), err)
		return fmt.Errorf("failed to convert entity to sgroups proto: %w", err)
	}

	fmt.Printf("🔧 DEBUG: IEAgAgRuleSyncer.Sync - Converted entity %s to proto\n", entity.GetSyncKey())

	// Create sync request
	syncReq := &types.SyncRequest{
		Operation:   operation,
		SubjectType: types.SyncSubjectTypeIEAgAgRules,
		Data:        protoData,
	}

	fmt.Printf("🔧 DEBUG: IEAgAgRuleSyncer.Sync - Sending sync request to gateway for entity %s\n", entity.GetSyncKey())

	// Send sync request to sgroups
	if err := s.gateway.Sync(ctx, syncReq); err != nil {
		fmt.Printf("❌ ERROR: IEAgAgRuleSyncer.Sync - Gateway sync failed for entity %s: %v\n", entity.GetSyncKey(), err)
		return fmt.Errorf("failed to sync entity with sgroups: %w", err)
	}

	fmt.Printf("✅ DEBUG: IEAgAgRuleSyncer.Sync - Successfully completed sync for entity %s\n", entity.GetSyncKey())
	s.logger.V(1).Info("Successfully synced IEAgAgRule", "key", entity.GetSyncKey(), "operation", operation)

	return nil
}

// SyncBatch synchronizes multiple IEAgAgRule entities in a batch
func (s *IEAgAgRuleSyncer) SyncBatch(ctx context.Context, entities []interfaces.SyncableEntity, operation types.SyncOperation) error {
	if len(entities) == 0 {
		return nil
	}

	fmt.Printf("🔧 DEBUG: IEAgAgRuleSyncer.SyncBatch - Starting batch sync for %d entities (operation: %s)\n", len(entities), operation)

	// For now, sync entities one by one
	// TODO: Implement true batch sync if sgroups supports it
	for i, entity := range entities {
		fmt.Printf("🔧 DEBUG: IEAgAgRuleSyncer.SyncBatch - Processing entity %d/%d: %s\n", i+1, len(entities), entity.GetSyncKey())
		if err := s.Sync(ctx, entity, operation); err != nil {
			fmt.Printf("❌ ERROR: IEAgAgRuleSyncer.SyncBatch - Failed to sync entity %s: %v\n", entity.GetSyncKey(), err)
			return fmt.Errorf("failed to sync entity %s in batch: %w", entity.GetSyncKey(), err)
		}
	}

	fmt.Printf("✅ DEBUG: IEAgAgRuleSyncer.SyncBatch - Successfully completed batch sync for %d entities\n", len(entities))
	s.logger.V(1).Info("Successfully synced batch of IEAgAgRules", "count", len(entities), "operation", operation)

	return nil
}

// GetSupportedSubjectType returns the subject type this syncer supports
func (s *IEAgAgRuleSyncer) GetSupportedSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeIEAgAgRules
}
